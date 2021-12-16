package tumble

import (
	"fmt"
	"os"
	"path/filepath"
)

func backupName(fpath string) string {
	dir := filepath.Dir(fpath)
	filename := filepath.Base(fpath)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	t := nowFn().UTC()
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", prefix, t.Unix(), ext))
}

func (l *Logger) openNew() error {
	name := l.Filepath
	_, err := os.Stat(name)
	if err == nil {
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

func (l *Logger) openExistingOrNew(writeLen int) error {
	l.mill()

	fpath := l.Filepath
	info, err := os.Stat(fpath)
	if os.IsNotExist(err) {
		return l.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %s", err)
	}

	if info.Size()+int64(writeLen) >= int64(l.MaxLogSizeMB*MB) {
		return l.rotate()
	}

	file, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, fileMode)
	if err != nil {
		// if we fail to open the old log file for some reason, just ignore
		// it and open a new log file.
		return l.openNew()
	}
	l.file = file
	l.size = info.Size()
	return nil
}

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
