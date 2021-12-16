// Package lumberjack provides a rolling logger.
//
// Note that this is v2.0 of lumberjack, and should be imported using gopkg.in
// thusly:
//
//   import "gopkg.in/natefinch/lumberjack.v2"
//
// The package name remains simply lumberjack, and the code resides at
// https://github.com/natefinch/lumberjack under the v2.0 branch.
//
// Lumberjack is intended to be one part of a logging infrastructure.
// It is not an all-in-one solution, but instead is a pluggable
// component at the bottom of the logging stack that simply controls the files
// to which logs are written.
//
// Lumberjack plays well with any logging package that can write to an
// io.Writer, including the standard library's log package.
//
// Lumberjack assumes that only one process is writing to the output files.
// Using the same lumberjack configuration from multiple processes on the same
// machine will result in improper behavior.
package lumberjack

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	compressSuffix = ".gz"
	fileMode       = 0644
)

// ensure we always implement io.WriteCloser
var _ io.WriteCloser = (*Logger)(nil)

// Logger is an io.WriteCloser that writes to the specified filename.
//
// Logger opens or creates the logfile on first Write.  If the file exists and
// is less than MaxLogSizeMB megabytes, lumberjack will open and append to that file.
// If the file exists and its size is >= MaxLogSizeMB megabytes, the file is renamed
// by putting the current time in a timestamp in the name immediately before the
// file's extension (or the end of the filename if there's no extension). A new
// log file is then created using original filename.
//
// Whenever a write would cause the current log file exceed MaxLogSizeMB megabytes,
// the current file is closed, renamed, and a new log file created with the
// original name. Thus, the filename you give Logger is always the "current" log
// file.
//
// Backups use the log file name given to Logger, in the form
// `name-timestamp.ext` where name is the filename without the extension,
// timestamp is the time at which the log was rotated formatted with the
// time.Time format of `2006-01-02T15-04-05.000` and the extension is the
// original extension.  For example, if your Logger.Filename is
// `/var/log/foo/server.log`, a backup created at 6:30pm on Nov 11 2016 would
// use the filename `/var/log/foo/server-2016-11-04T18-30-00.000.log`
//
// Cleaning Up Old Log Files
//
// Whenever a new logfile gets created, old log files may be deleted.  The most
// recent files according to the encoded timestamp will be retained, up to a
// maximum size determined by MaxTotalSizeMB. Note that the time encoded in the
// timestamp is the rotation time, which may differ from the last time
// that file was written to.
type Logger struct {
	// Filename is the file to write logs to.  Backup log files will be retained
	// in the same directory.  It uses <processname>-lumberjack.log in
	// os.TempDir() if empty.
	Filename string

	// MaxLogSizeMB is the maximum size in megabytes of the log file before it gets
	// rotated.
	MaxLogSizeMB uint

	// MaxTotalSizeMB is the maximum size in megabytes of all log files, including
	// rotated and compressed ones.
	MaxTotalSizeMB uint

	// FormatFn is a formatting function that processes input before it is written.
	// It is typically used to add a timestamp in a configurable format.
	// The buf parameter is a buffer to be modified and returned (prevents allocations).
	//
	// In addition to returning the buffer, it needs to also return the index
	// where the msg begins. This is so the caller can calculate the correct
	// return value in the case of a write error.
	//
	// For example:
	//
	//   formatFn = func(msg []byte, buf []byte) ([]byte, int) {
	//       now := time.Now().UTC().Format("2006-01-02 15:04:05.000")
	//       buf = append(buf, []byte(now)...)      // This always has length 23
	//       buf = append(buf, []byte(" : ")...)    // This always has length 3
	//       buf = append(buf, msg...)              // Therefore, this starts at index 26
	//       return buf, 26                         // alternatively, len(now)+len(" : ")
	//   }
	//
	FormatFn func(msg []byte, buf []byte) ([]byte, int)
	fmtbuf   []byte

	size int64
	file *os.File

	millCh    chan bool
	startMill sync.Once
}

var (
	// currentTime exists so it can be mocked out by tests.
	currentTime = time.Now

	// megabyte is the conversion factor between MaxLogSizeMB and bytes.  It is a
	// variable so tests can mock it out and not need to write megabytes of data
	// to disk.
	megabyte = uint(1024 * 1024)
)

// Write implements io.Writer.  If a write would cause the log file to be larger
// than MaxLogSizeMB, the file is closed, renamed to include a timestamp of the
// current time, and a new log file is created using the original log file name.
func (l *Logger) Write(p []byte) (n int, err error) {
	writeLen := int64(len(p))

	if l.file == nil {
		if err = l.openExistingOrNew(len(p)); err != nil {
			return 0, err
		}
	} else if l.size+writeLen > l.max() {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	var msg []byte
	var msgIdx int
	if l.FormatFn != nil {
		l.fmtbuf = l.fmtbuf[:0]
		l.fmtbuf, msgIdx = l.FormatFn(p, l.fmtbuf)
		msg = l.fmtbuf
	} else {
		msg = p
	}

	n, err = l.file.Write(msg)
	l.size += int64(n)
	if l.FormatFn != nil {
		// Return length of p consumed
		if n < msgIdx {
			return 0, err
		}
		if n-msgIdx > len(p) {
			return len(p), err
		}
		return n - msgIdx, err
	}
	return n, err
}

// Close implements io.Closer, and closes the current logfile.
func (l *Logger) Close() error {
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// rotate closes the current file, moves it aside with a timestamp in the name,
// (if it exists), opens a new file with the original filename, and then runs
// post-rotation processing and removal.
func (l *Logger) rotate() error {
	if err := l.Close(); err != nil {
		return err
	}
	if err := l.openNew(); err != nil {
		return err
	}
	l.mill()
	return nil
}

// openNew opens a new log file for writing, moving any old log file out of the
// way.  This methods assumes the file has already been closed.
func (l *Logger) openNew() error {
	name := l.Filename
	_, err := os.Stat(name)
	if err == nil {
		// move the existing file
		newname := backupName(name)
		if err := os.Rename(name, newname); err != nil {
			return fmt.Errorf("can't rename log file: %s", err)
		}
	}

	// we use truncate here because this should only get called when we've moved
	// the file ourselves. if someone else creates the file in the meantime,
	// just wipe out the contents.
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(fileMode))
	if err != nil {
		return fmt.Errorf("can't open new logfile: %s", err)
	}
	l.file = f
	l.size = 0
	return nil
}

// backupName creates a new filename from the given name, inserting a timestamp
// between the filename and the extension
func backupName(name string) string {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	t := currentTime().UTC()
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", prefix, t.Unix(), ext))
}

// openExistingOrNew opens the logfile if it exists and if the current write
// would not put it over MaxLogSizeMB.  If there is no such file or the write would
// put it over the MaxLogSizeMB, a new file is created.
func (l *Logger) openExistingOrNew(writeLen int) error {
	l.mill()

	filename := l.Filename
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return l.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %s", err)
	}

	if info.Size()+int64(writeLen) >= l.max() {
		return l.rotate()
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, fileMode)
	if err != nil {
		// if we fail to open the old log file for some reason, just ignore
		// it and open a new log file.
		return l.openNew()
	}
	l.file = file
	l.size = info.Size()
	return nil
}

// millRunOnce performs compression and removal of stale log files.
// Log files are compressed if enabled via configuration and old log
// files are removed
func (l *Logger) millRunOnce() error {
	oldFiles, err := l.oldLogFiles()
	if err != nil {
		return err
	}

	// It is possible to have both an uncompressed and (partially) compressed file for the same log
	// In this case, we overwrite the compressed file with a new one in compressLogFile().
	// We overwrite keys over two passes on a map to ensure that logInfo entries are the current ones.
	compressedMap := make(map[time.Time]logInfo)
	for _, f := range oldFiles {
		if strings.HasSuffix(f.Name(), compressSuffix) {
			compressedMap[f.timestamp] = f
		}
	}
	for _, f := range oldFiles {
		if !strings.HasSuffix(f.Name(), compressSuffix) {
			fn := filepath.Join(l.dir(), f.Name())
			err := compressLogFile(fn, fn+compressSuffix)
			if err != nil {
				return err
			}
			fi, err := os.Stat(fn + compressSuffix)
			if err != nil {
				return err
			}
			compressedMap[f.timestamp] = logInfo{fi, f.timestamp}
		}
	}

	// Sort logInfo entries and discard the oldest once the maximum storage size has been exhausted.
	// Note that we subtract the current log's maximum size, requiring compressed logs to fit
	// within the remaining space (l.MaxTotalSizeMB - l.MaxLogSizeMB).
	compressedFiles := make([]logInfo, 0, len(compressedMap))
	for _, v := range compressedMap {
		compressedFiles = append(compressedFiles, v)
	}
	sort.Sort(byFormatTime(compressedFiles))

	totalSizeBytes := int64(0)
	for _, f := range compressedFiles {
		totalSizeBytes += f.Size()
		if totalSizeBytes > int64((l.MaxTotalSizeMB-l.MaxLogSizeMB)*megabyte) {
			err := os.Remove(filepath.Join(l.dir(), f.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// millRun runs in a goroutine to manage post-rotation compression and removal
// of old log files. The nested loop structure drains the channel on each run.
func (l *Logger) millRun() {
	for {
		<-l.millCh
	outer:
		for {
			select {
			case <-l.millCh:
				continue
			default:
				break outer
			}
		}
		if err := l.millRunOnce(); err != nil {
			fmt.Fprintln(os.Stderr, "error in millRunOnce:", err)
		}
	}
}

// mill performs post-rotation compression and removal of stale log files,
// starting the mill goroutine if necessary.
func (l *Logger) mill() {
	l.startMill.Do(func() {
		l.millCh = make(chan bool, 2)
		go l.millRun()
	})
	select {
	case l.millCh <- true:
	default:
	}
}

// oldLogFiles returns the list of backup log files stored in the same
// directory as the current log file, sorted by ModTime
func (l *Logger) oldLogFiles() ([]logInfo, error) {
	files, err := ioutil.ReadDir(l.dir())
	if err != nil {
		return nil, fmt.Errorf("can't read log file directory: %s", err)
	}
	logFiles := []logInfo{}

	prefix, ext := l.prefixAndExt()

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if t, err := l.timeFromName(f.Name(), prefix, ext); err == nil {
			logFiles = append(logFiles, logInfo{f, t})
			continue
		}
		if t, err := l.timeFromName(f.Name(), prefix, ext+compressSuffix); err == nil {
			logFiles = append(logFiles, logInfo{f, t})
			continue
		}
		// error parsing means that the suffix at the end was not generated
		// by lumberjack, and therefore it's not a backup file.
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

// timeFromName extracts the formatted time from the filename by stripping off
// the filename's prefix and extension. This prevents someone's filename from
// confusing time.parse.
func (l *Logger) timeFromName(filename, prefix, ext string) (time.Time, error) {
	if !strings.HasPrefix(filename, prefix) {
		return time.Time{}, errors.New("mismatched prefix")
	}
	if !strings.HasSuffix(filename, ext) {
		return time.Time{}, errors.New("mismatched extension")
	}
	ts := filename[len(prefix) : len(filename)-len(ext)]
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Time{}, errors.New("invalid timestamp")
	}
	return time.Unix(secs, 0).UTC(), nil
}

// max returns the maximum size in bytes of log files before rolling.
func (l *Logger) max() int64 {
	return int64(l.MaxLogSizeMB) * int64(megabyte)
}

// dir returns the directory for the current filename.
func (l *Logger) dir() string {
	return filepath.Dir(l.Filename)
}

// prefixAndExt returns the filename part and extension part from the Logger's
// filename.
func (l *Logger) prefixAndExt() (prefix, ext string) {
	filename := filepath.Base(l.Filename)
	ext = filepath.Ext(filename)
	prefix = filename[:len(filename)-len(ext)] + "-"
	return prefix, ext
}

// compressLogFile compresses the given log file, removing the
// uncompressed log file if successful.
func compressLogFile(src, dst string) (err error) {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	defer f.Close()

	_, err = os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat log file: %v", err)
	}

	// If this file already exists, we presume it was created by
	// a previous attempt to compress the log file.
	gzf, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(fileMode))
	if err != nil {
		return fmt.Errorf("failed to open compressed log file: %v", err)
	}
	defer gzf.Close()

	gz := gzip.NewWriter(gzf)

	defer func() {
		if err != nil {
			os.Remove(dst)
			err = fmt.Errorf("failed to compress log file: %v", err)
		}
	}()

	if _, err := io.Copy(gz, f); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := gzf.Close(); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return err
	}

	return nil
}

// logInfo is a convenience struct to return the filename and its embedded
// timestamp.
type logInfo struct {
	os.FileInfo
	timestamp time.Time
}

// byFormatTime sorts by newest time formatted in the name.
type byFormatTime []logInfo

func (b byFormatTime) Less(i, j int) bool {
	return b[i].timestamp.After(b[j].timestamp)
}

func (b byFormatTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFormatTime) Len() int {
	return len(b)
}
