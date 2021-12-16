package tumble

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
	"time"
)

type logInfo struct {
	os.FileInfo
	timestamp time.Time
}

type byFormatTime []logInfo

func (b byFormatTime) Len() int {
	return len(b)
}
func (b byFormatTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
func (b byFormatTime) Less(i, j int) bool {
	return b[i].timestamp.After(b[j].timestamp)
}

func compressLogFile(src string) (err error) {
	dst := src + compressSuffix

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

func (l *Logger) dir() string {
	return filepath.Dir(l.Filepath)
}

func (l *Logger) prefixAndExt() (prefix, ext string) {
	filename := filepath.Base(l.Filepath)
	ext = filepath.Ext(filename)
	prefix = filename[:len(filename)-len(ext)] + "-"
	return prefix, ext
}

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
		// by us, and therefore it's not a backup file.
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

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
			err := compressLogFile(fn)
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
		if totalSizeBytes > int64((l.MaxTotalSizeMB-l.MaxLogSizeMB)*MB) {
			err := os.Remove(filepath.Join(l.dir(), f.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

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
			fmt.Fprintln(os.Stderr, "error in tumble/millRunOnce:", err)
		}
	}
}

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
