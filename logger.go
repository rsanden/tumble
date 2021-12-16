package tumble

import (
	"io"
	"time"
)

const (
	compressSuffix = ".gz"
	fileMode       = 0644
)

// Ensure we always implement io.WriteCloser
var _ io.WriteCloser = (*Logger)(nil)

var (
	// These constants are mocked out by tests
	nowFn = time.Now
	MB    = uint(1024 * 1024)
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
	} else if l.size+writeLen > int64(l.MaxLogSizeMB*MB) {
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
