package tumble

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Filestamper interface {
	filepath() string
	dirpath() string
	namePrefix() string
	nameExt() string
	timestampToFpath(ts time.Time) string
	timestampLength() int
	parseTimestamp(s string) (time.Time, error)
	fpathToTimestamp(fpath string) (time.Time, error)
}

// This is:
//           ""  in          "foo.log",
//        "tmp/  in      "tmp/foo.log"
//   "/path/to/" in "/path/to/foo.log"
//
func dirpath(this Filestamper) string {
	if filepath.Dir(this.filepath()) == "." {
		return ""
	}
	return filepath.Dir(this.filepath()) + "/"
}

// This is "foo" in "/path/to/foo.log"
func namePrefix(this Filestamper) string {
	return this.filepath()[len(this.dirpath()) : len(this.filepath())-len(this.nameExt())]
}

// This is ".log" in "/path/to/foo.log"
func nameExt(this Filestamper) string {
	return filepath.Ext(this.filepath())
}

func timestampToFpath(this Filestamper, ts time.Time) string {
	return fmt.Sprintf("%s%s-%d%s%s", this.dirpath(), this.namePrefix(), ts.Unix(), this.nameExt(), compressSuffix)
}

func timestampLength(this Filestamper) int {
	// Today we use a length-10 dateint (but this could be changed)
	return 10
}

func parseTimestamp(this Filestamper, s string) (time.Time, error) {
	if len(s) != this.timestampLength() {
		return time.Time{}, errors.New("invalid timestamp")
	}

	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, errors.New("invalid timestamp")
	}

	return time.Unix(secs, 0).UTC(), nil
}

func fpathToTimestamp(this Filestamper, fpath string) (time.Time, error) {
	dirpath := this.dirpath()
	namePrefix := this.namePrefix()
	nameExt := this.nameExt()

	// fpath must be exactly this long to possibly match
	if len(fpath) != len(dirpath)+len(namePrefix)+len("-")+this.timestampLength()+len(nameExt)+len(compressSuffix) {
		return time.Time{}, errors.New("mismatch")
	}

	middle := fpath[len(dirpath)+len(namePrefix) : len(fpath)-len(nameExt)-len(compressSuffix)]

	// middle should be a hyphen followed by a timestamp
	if !strings.HasPrefix(middle, "-") {
		return time.Time{}, errors.New("mismatch")
	}

	// middle (after hyphen) should be exactly a timestamp
	ts, err := this.parseTimestamp(middle[1:])
	if err != nil {
		return time.Time{}, errors.New("mismatch")
	}

	// finally, check that this is our log (rather than another with the same length)
	if fpath != this.timestampToFpath(ts) {
		return time.Time{}, errors.New("mismatch")
	}

	return ts, nil
}
