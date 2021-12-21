package tumble

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
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
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	_, err = os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat log file: %w", err)
	}

	// If this file already exists, we presume it was created by
	// a previous attempt to compress the log file.
	gzf, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(fileMode))
	if err != nil {
		return fmt.Errorf("failed to open compressed log file: %w", err)
	}
	defer gzf.Close()

	gz := gzip.NewWriter(gzf)

	defer func() {
		if err != nil {
			os.Remove(dst)
			err = fmt.Errorf("failed to compress log file: %w", err)
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

func (me *Logger) oldLogFiles() ([]logInfo, error) {
	files, err := ioutil.ReadDir(filepath.Dir(me.Filepath))
	if err != nil {
		return nil, fmt.Errorf("can't read log file directory: %w", err)
	}
	logFiles := []logInfo{}

	dirpath := me.dirpath()
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if ts, err := me.fpathToTimestamp(dirpath + f.Name()); err == nil {
			logFiles = append(logFiles, logInfo{f, ts})
			continue
		}
		if ts, err := me.fpathToTimestamp(dirpath + f.Name() + compressSuffix); err == nil {
			logFiles = append(logFiles, logInfo{f, ts})
			continue
		}
		// error parsing means that the suffix at the end was not generated
		// by us, and therefore it's not a backup file.
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

func (me *Logger) millRunOnce() error {
	oldFiles, err := me.oldLogFiles()
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
			fn := filepath.Join(filepath.Dir(me.Filepath), f.Name())
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
	// within the remaining space (me.MaxTotalSizeMB - me.MaxLogSizeMB).
	compressedFiles := make([]logInfo, 0, len(compressedMap))
	for _, v := range compressedMap {
		compressedFiles = append(compressedFiles, v)
	}
	sort.Sort(byFormatTime(compressedFiles))

	totalSizeBytes := int64(0)
	for _, f := range compressedFiles {
		totalSizeBytes += f.Size()
		if totalSizeBytes > int64((me.MaxTotalSizeMB-me.MaxLogSizeMB)*MB) {
			err := os.Remove(filepath.Join(filepath.Dir(me.Filepath), f.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (me *Logger) drainMillCh() {
	for {
		select {
		case _, ok := <-me.millCh:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func (me *Logger) millRun() {
	defer me.millWG.Done()

	for range me.millCh {
		me.drainMillCh()

		if err := me.millRunOnce(); err != nil {
			fmt.Fprintln(os.Stderr, "error in tumble/millRunOnce:", err)
		}
	}
}

func (me *Logger) mill() {
	select {
	case me.millCh <- struct{}{}:
	default:
	}
}

func (me *Logger) StopMill() {
	me.stopMillOnce.Do(func() {
		close(me.millCh)
	})
	me.millWG.Wait()
}
