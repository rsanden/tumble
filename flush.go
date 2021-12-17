package tumble

import "io"

type FlusherError interface{ Flush() error }
type FlusherVoid interface{ Flush() }

func Flush(wr io.Writer) error {
	if wr == nil {
		return nil
	}
	if flusher, ok := wr.(FlusherError); ok {
		return flusher.Flush()
	}
	if flusher, ok := wr.(FlusherVoid); ok {
		flusher.Flush()
		return nil
	}
	return nil
}

func (me *Logger) Flush() error {
	return Flush(me.file)
}
