package tumble

// Note: Run tests sequentially (go test -parallel 1)

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

const sleepTime = 100 * time.Millisecond

func TestNewFile(t *testing.T) {
	nowFn = fakeTime

	dir := makeTempDir("TestNewFile", t)
	defer os.RemoveAll(dir)
	l := NewLogger(
		/* Filepath:       */ logFile(dir),
		/* MaxLogSizeMB:   */ 100,
		/* MaxTotalSizeMB: */ 150,
		/* FormatFn:       */ nil,
	)
	defer l.Close()
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)
	existsWithContent(logFile(dir), b, t)
	fileCount(dir, 1, t)
}

func TestOpenExisting(t *testing.T) {
	nowFn = fakeTime
	dir := makeTempDir("TestOpenExisting", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	data := []byte("foo!")
	err := ioutil.WriteFile(filename, data, fileMode)
	isNil(err, t)
	existsWithContent(filename, data, t)

	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 100,
		/* MaxTotalSizeMB: */ 150,
		/* FormatFn:       */ nil,
	)
	defer l.Close()
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithContent(filename, append(data, b...), t)

	fileCount(dir, 1, t)
}

func TestFirstWriteRotate(t *testing.T) {
	nowFn = fakeTime
	MB = 1
	dir := makeTempDir("TestFirstWriteRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 6,
		/* MaxTotalSizeMB: */ 50,
		/* FormatFn:       */ nil,
	)
	defer l.Close()

	// this won't rotate
	start := []byte("data")
	err := ioutil.WriteFile(filename, start, fileMode)
	isNil(err, t)
	existsWithContent(filename, start, t)

	newFakeTime()

	// this would rotate
	b := []byte("foooooo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	time.Sleep(sleepTime)

	existsWithContent(filename, b, t)

	bc := new(bytes.Buffer)
	gz := gzip.NewWriter(bc)
	_, err = gz.Write(start)
	isNil(err, t)
	err = gz.Close()
	isNil(err, t)
	existsWithContent(backupFile(dir)+compressSuffix, bc.Bytes(), t)

	fileCount(dir, 2, t)
}

func TestCleanupExistingBackups(t *testing.T) {
	// test that if we start with more backup files than we're supposed to have
	// in total, that extra ones get cleaned up when we rotate.

	nowFn = fakeTime
	MB = 1

	dir := makeTempDir("TestCleanupExistingBackups", t)
	defer os.RemoveAll(dir)

	// make 3 backup files

	data := []byte("data")
	backup := backupFile(dir)
	err := ioutil.WriteFile(backup+compressSuffix, data, fileMode)
	isNil(err, t)

	newFakeTime()

	backup = backupFile(dir)
	err = ioutil.WriteFile(backup+compressSuffix, data, fileMode)
	isNil(err, t)

	newFakeTime()

	backup = backupFile(dir)
	err = ioutil.WriteFile(backup+compressSuffix, data, fileMode)
	isNil(err, t)

	// now create a primary log file with some data
	filename := logFile(dir)
	err = ioutil.WriteFile(filename, data, fileMode)
	isNil(err, t)
	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 10,
		/* MaxTotalSizeMB: */ 40, /* The first rotation will create a 28-byte gzipped file */
		/* FormatFn:       */ nil,
	)
	defer l.Close()

	newFakeTime()

	b2 := []byte("foooooo!")
	n, err := l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	time.Sleep(sleepTime)

	// now we should only have 2 files left - the primary and one backup
	fileCount(dir, 2, t)
}

func TestOldLogFiles(t *testing.T) {
	nowFn = fakeTime
	MB = 1

	dir := makeTempDir("TestOldLogFiles", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	data := []byte("data")
	err := ioutil.WriteFile(filename, data, 07)
	isNil(err, t)

	// This gives us a time with the same precision as the time we get from the
	// timestamp in the name.
	t1 := time.Unix(fakeTime().Unix(), 0).UTC()

	backup := backupFile(dir)
	err = ioutil.WriteFile(backup, data, 07)
	isNil(err, t)

	newFakeTime()

	t2 := time.Unix(fakeTime().Unix(), 0).UTC()

	backup2 := backupFile(dir)
	err = ioutil.WriteFile(backup2, data, 07)
	isNil(err, t)

	l := &Logger{Filepath: filename}
	files, err := l.oldLogFiles()
	isNil(err, t)
	equals(2, len(files), t)

	// should be sorted by newest file first, which would be t2
	equals(t2, files[0].timestamp, t)
	equals(t1, files[1].timestamp, t)
}

func TestFpathToTimestamp(t *testing.T) {
	l := &Logger{Filepath: "/var/log/myfoo/foo.log"}

	tests := []struct {
		filename string
		want     time.Time
		wantErr  bool
	}{
		{"foo-1399214673.log" + compressSuffix, time.Date(2014, 5, 4, 14, 44, 33, 000000000, time.UTC), false},
		{"foo-1399214673.log", time.Time{}, true},
		{"foo-1399214673", time.Time{}, true},
		{"1399214673.log", time.Time{}, true},
		{"foo.log", time.Time{}, true},
	}

	for _, test := range tests {
		got, err := l.fpathToTimestamp(l.dirpath() + test.filename)
		equals(got, test.want, t)
		equals(err != nil, test.wantErr, t)
	}
}

func TestRotate(t *testing.T) {
	MB = 1

	nowFn = fakeTime
	dir := makeTempDir("TestRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)

	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 12,
		/* MaxTotalSizeMB: */ 77, /* gz files are between 23 and 29 bytes */
		/* FormatFn:       */ nil,
	)
	defer l.Close()
	b := []byte("data")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithContent(filename, b, t)
	fileCount(dir, 1, t)

	newFakeTime()

	err = l.rotate()
	isNil(err, t)

	time.Sleep(sleepTime)

	filename2 := backupFile(dir)

	bc := new(bytes.Buffer)
	gz := gzip.NewWriter(bc)
	_, err = gz.Write(b)
	isNil(err, t)
	err = gz.Close()
	isNil(err, t)
	existsWithContent(filename2+compressSuffix, bc.Bytes(), t)

	existsWithContent(filename, []byte{}, t)
	fileCount(dir, 2, t)
	newFakeTime()

	err = l.rotate()
	isNil(err, t)

	time.Sleep(sleepTime)

	filename3 := backupFile(dir)

	bc = new(bytes.Buffer)
	gz = gzip.NewWriter(bc)
	_, err = gz.Write([]byte(""))
	isNil(err, t)
	err = gz.Close()
	isNil(err, t)
	existsWithContent(filename3+compressSuffix, bc.Bytes(), t)

	existsWithContent(filename, []byte{}, t)
	fileCount(dir, 3, t)
	newFakeTime()

	b2 := []byte("foooooo!") /* This does not trigger a rotate */
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	time.Sleep(sleepTime)

	fileCount(dir, 3, t)
	newFakeTime()

	b3 := []byte("foooooo!") /* This triggers a rotate */
	n, err = l.Write(b3)
	isNil(err, t)
	equals(len(b3), n, t)

	time.Sleep(sleepTime)

	fileCount(dir, 3, t)

	// this will use the new fake time
	existsWithContent(filename, b3, t)
}

func TestCompressOnRotate(t *testing.T) {
	nowFn = fakeTime
	MB = 1

	dir := makeTempDir("TestCompressOnRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 10,
		/* MaxTotalSizeMB: */ 50,
		/* FormatFn:       */ nil,
	)
	defer l.Close()
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithContent(filename, b, t)
	fileCount(dir, 1, t)

	newFakeTime()

	err = l.rotate()
	isNil(err, t)

	time.Sleep(sleepTime)

	// the old logfile should be moved aside and the main logfile should have nothing in it.
	existsWithContent(filename, []byte{}, t)

	// a compressed version of the log file should now exist and the original should have been removed.
	bc := new(bytes.Buffer)
	gz := gzip.NewWriter(bc)
	_, err = gz.Write(b)
	isNil(err, t)
	err = gz.Close()
	isNil(err, t)

	existsWithContent(backupFile(dir)+compressSuffix, bc.Bytes(), t)
	notExist(backupFile(dir), t)

	fileCount(dir, 2, t)
}

func TestCompressOnResume(t *testing.T) {
	nowFn = fakeTime
	MB = 1

	dir := makeTempDir("TestCompressOnResume", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 6,
		/* MaxTotalSizeMB: */ 40, /* The first rotation will create a 28-byte gzipped file */
		/* FormatFn:       */ nil,
	)
	defer l.Close()

	// Create a backup file and empty "compressed" file.
	filename2 := backupFile(dir)
	b := []byte("foo!")
	err := ioutil.WriteFile(filename2, b, fileMode)
	isNil(err, t)
	err = ioutil.WriteFile(filename2+compressSuffix, []byte{}, fileMode)
	isNil(err, t)

	newFakeTime()
	b2 := []byte("boo!")
	n, err := l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)
	existsWithContent(filename, b2, t)

	time.Sleep(sleepTime)

	// The write should have started the compression - a compressed version of
	// the log file should now exist and the original should have been removed.
	bc := new(bytes.Buffer)
	gz := gzip.NewWriter(bc)
	_, err = gz.Write(b)
	isNil(err, t)
	err = gz.Close()
	isNil(err, t)
	existsWithContent(filename2+compressSuffix, bc.Bytes(), t)
	notExist(filename2, t)

	fileCount(dir, 2, t)
}

func TestRotateClose(t *testing.T) {
	nowFn = fakeTime
	MB = 1
	dir := makeTempDir("TestRotateClose", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 100,
		/* MaxTotalSizeMB: */ 150,
		/* FormatFn:       */ nil,
	)
	defer l.Close()

	b := []byte("data")
	err := ioutil.WriteFile(filename, b, fileMode)
	isNil(err, t)
	existsWithContent(filename, b, t)

	newFakeTime()

	l.RotateClose()

	backupname := backupFile(dir)
	bc := new(bytes.Buffer)
	gz := gzip.NewWriter(bc)
	_, err = gz.Write(b)
	isNil(err, t)
	err = gz.Close()
	isNil(err, t)
	existsWithContent(backupname+compressSuffix, bc.Bytes(), t)
	notExist(backupname, t)

	existsWithContent(filename, []byte(""), t)
}

func TestTimestampFormatFn(t *testing.T) {
	dir := makeTempDir("TestTimestampFormatFn", t)
	defer os.RemoveAll(dir)

	timeFormat := "2006-01-02 15:04:05.000"
	formatFn := func(msg []byte, buf []byte) ([]byte, int) {
		now := fakeTime().Format(timeFormat)
		buf = append(buf, []byte(now)...)
		buf = append(buf, []byte(" : ")...)
		buf = append(buf, msg...)
		return buf, len(now) + len(" : ")
	}

	filename := logFile(dir)
	l := NewLogger(
		/* Filepath:       */ filename,
		/* MaxLogSizeMB:   */ 100,
		/* MaxTotalSizeMB: */ 150,
		/* FormatFn:       */ formatFn,
	)
	defer l.Close()

	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	var expectedContent []byte
	fakeTimestamp := fakeTime().Format(timeFormat)
	expectedContent = append(expectedContent, []byte(fakeTimestamp)...)
	expectedContent = append(expectedContent, []byte(" : ")...)
	expectedContent = append(expectedContent, b...)
	existsWithContent(filename, expectedContent, t)
}

func TestDumpPaths(t *testing.T) {
	var muster *Muster
	var err error
	var ts time.Time
	var LOG_TIME = time.Unix(1500000000, 0).UTC()

	muster = NewMuster("foo.log")
	equals("", muster.dirpath(), t)
	equals("foo", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("foo-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("foo-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("bad-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("./foo.log")
	equals("", muster.dirpath(), t)
	equals("foo", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("foo-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("foo-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("bad-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("tmp/foo.log")
	equals("tmp/", muster.dirpath(), t)
	equals("foo", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("tmp/foo-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("tmp/foo-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("tmp/bad-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("./tmp/foo.log")
	equals("tmp/", muster.dirpath(), t)
	equals("foo", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("tmp/foo-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("tmp/foo-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("tmp/bad-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("/path/to/foo.log")
	equals("/path/to/", muster.dirpath(), t)
	equals("foo", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("/path/to/foo-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("/path/to/foo-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("/path/to/bad-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("foolog")
	equals("", muster.dirpath(), t)
	equals("foolog", muster.namePrefix(), t)
	equals("", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("foolog-1500000000"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("foolog-1500000000" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("badlog-1500000000" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("tmp/foolog")
	equals("tmp/", muster.dirpath(), t)
	equals("foolog", muster.namePrefix(), t)
	equals("", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("tmp/foolog-1500000000"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("tmp/foolog-1500000000" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("tmp/badlog-1500000000" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("/path/to/foolog")
	equals("/path/to/", muster.dirpath(), t)
	equals("foolog", muster.namePrefix(), t)
	equals("", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("/path/to/foolog-1500000000"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("/path/to/foolog-1500000000" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("/path/to/badlog-1500000000" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("foo.bar.log")
	equals("", muster.dirpath(), t)
	equals("foo.bar", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("foo.bar-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("foo.bar-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("bad.bar-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("tmp/foo.bar.log")
	equals("tmp/", muster.dirpath(), t)
	equals("foo.bar", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("tmp/foo.bar-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("tmp/foo.bar-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("tmp/bad.bar-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("/path/to/foo.bar.log")
	equals("/path/to/", muster.dirpath(), t)
	equals("foo.bar", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("/path/to/foo.bar-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("/path/to/foo.bar-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("/path/to/bad.bar-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("foo-bar.baz.log")
	equals("", muster.dirpath(), t)
	equals("foo-bar.baz", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("foo-bar.baz-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("foo-bar.baz-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("bad-bar.baz-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("tmp/foo-bar.baz.log")
	equals("tmp/", muster.dirpath(), t)
	equals("foo-bar.baz", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("tmp/foo-bar.baz-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("tmp/foo-bar.baz-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("tmp/bad-bar.baz-1500000000.log" + compressSuffix)
	notNil(err, t)

	muster = NewMuster("/path/to/foo-bar.baz.log")
	equals("/path/to/", muster.dirpath(), t)
	equals("foo-bar.baz", muster.namePrefix(), t)
	equals(".log", muster.nameExt(), t)
	equals(muster.Filepath, muster.dirpath()+muster.namePrefix()+muster.nameExt(), t)
	equals("/path/to/foo-bar.baz-1500000000.log"+compressSuffix, muster.timestampToFpath(LOG_TIME), t)
	ts, err = muster.fpathToTimestamp("/path/to/foo-bar.baz-1500000000.log" + compressSuffix)
	isNil(err, t)
	equals(LOG_TIME, ts, t)
	_, err = muster.fpathToTimestamp("/path/to/bad-bar.baz-1500000000.log" + compressSuffix)
	notNil(err, t)
}

func createDumpTestData() {
	count := 2001
	start := 1500000055
	for i := 1; i <= count-1; i++ {
		fname := fmt.Sprintf("foo-%d.log.gz", start+100*i)
		fpath := "tmp/" + fname
		content := fmt.Sprintf("This is file number %d\n", i)
		gzContentBuf := new(bytes.Buffer)
		gz := gzip.NewWriter(gzContentBuf)
		if _, err := gz.Write([]byte(content)); err != nil {
			panic(err)
		}
		if err := gz.Close(); err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(fpath, gzContentBuf.Bytes(), 0644); err != nil {
			panic(err)
		}
	}
	content := fmt.Sprintf("This is file number %d\n", count)
	if err := ioutil.WriteFile("tmp/foo.log", []byte(content), 0644); err != nil {
		panic(err)
	}
}

func TestDump(t *testing.T) {
	// Set open files limit to 1024 for test
	setOpenFilesLimit(1024)

	os.Mkdir("tmp", 0700)
	defer os.RemoveAll("tmp")
	createDumpTestData()

	muster := NewMuster("tmp/foo.log")
	defer muster.Close()

	idx := 2001 - muster.MaxArchiveLookback()
	scanner := bufio.NewScanner(muster)
	for scanner.Scan() {
		equals(fmt.Sprintf("This is file number %d", idx), scanner.Text(), t)
		idx += 1
	}
	isNil(scanner.Err(), t)
	equals(2001+1, idx, t)
}
