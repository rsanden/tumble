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

func (me *Logger) openNew() error {
	name := me.Filepath
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
	me.file = f
	me.size = 0
	return nil
}

func (me *Logger) openExistingOrNew(writeLen int) error {
	me.mill()

	fpath := me.Filepath
	info, err := os.Stat(fpath)
	if os.IsNotExist(err) {
		return me.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %s", err)
	}

	if info.Size()+int64(writeLen) >= int64(me.MaxLogSizeMB*MB) {
		return me.rotate()
	}

	file, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, fileMode)
	if err != nil {
		// if we fail to open the old log file for some reason, just ignore
		// it and open a new log file.
		return me.openNew()
	}
	me.file = file
	me.size = info.Size()
	return nil
}

func (me *Logger) rotate() error {
	if err := me.closeFile(); err != nil {
		return err
	}
	if err := me.openNew(); err != nil {
		return err
	}
	me.mill()
	return nil
}
