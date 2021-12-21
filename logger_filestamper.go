package tumble

import "time"

func (me *Logger) filepath() string {
	return me.Filepath
}

func (me *Logger) dirpath() string {
	return dirpath(me)
}

func (me *Logger) namePrefix() string {
	return namePrefix(me)
}

func (me *Logger) nameExt() string {
	return nameExt(me)
}

func (me *Logger) timestampToFpath(ts time.Time) string {
	return timestampToFpath(me, ts)
}

func (me *Logger) timestampLength() int {
	return timestampLength(me)
}

func (me *Logger) parseTimestamp(s string) (time.Time, error) {
	return parseTimestamp(me, s)
}

func (me *Logger) fpathToTimestamp(fpath string) (time.Time, error) {
	return fpathToTimestamp(me, fpath)
}
