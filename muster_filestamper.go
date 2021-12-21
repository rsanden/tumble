package tumble

import "time"

func (me *Muster) filepath() string {
	return me.Filepath
}

func (me *Muster) dirpath() string {
	return dirpath(me)
}

func (me *Muster) namePrefix() string {
	return namePrefix(me)
}

func (me *Muster) nameExt() string {
	return nameExt(me)
}

func (me *Muster) timestampToFpath(ts time.Time) string {
	return timestampToFpath(me, ts)
}

func (me *Muster) timestampLength() int {
	return timestampLength(me)
}

func (me *Muster) parseTimestamp(s string) (time.Time, error) {
	return parseTimestamp(me, s)
}

func (me *Muster) fpathToTimestamp(fpath string) (time.Time, error) {
	return fpathToTimestamp(me, fpath)
}
