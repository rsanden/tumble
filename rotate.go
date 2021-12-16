package tumble

import (
	"fmt"
	"os"
	"path/filepath"
)

// backupName creates a new filename from the given name, inserting a timestamp
// between the filename and the extension
func backupName(name string) string {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	t := nowFn().UTC()
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", prefix, t.Unix(), ext))
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

	if info.Size()+int64(writeLen) >= int64(l.MaxLogSizeMB*MB) {
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
