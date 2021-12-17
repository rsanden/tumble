package tumble

import (
	"io"
	"sync"
)

// Logger is an io.WriteCloser which writes content to a rotating log archive.
//
// Parameters:
//
//     fpath:          Path to the logfile
//     maxLogSizeMB:   Logfile size before it gets rotation (in MB)
//     maxTotalSizeMB: Total disk space of active log + compressed archives (in MB)
//     formatFn:       Log message formatting function (optional)
//
// FormatFn is a formatting function that processes input before it is written.
// It is typically used to add a timestamp in a configurable format.
// The buf parameter is a buffer to be modified and returned (prevents allocations).
//
// In addition to returning the buffer, it needs to also return the index
// where the msg begins. This is so the caller can calculate the correct
// return value in the case of a write error.
//
// Default formatting example:
//
//     log.SetFlags(log.LstdFlags | log.Lmicroseconds)
//     logger := tumble.NewLogger(
//         /* Filepath:       */ "/path/to/foo.log",
//         /* MaxLogSizeMB:   */ 100,
//         /* MaxTotalSizeMB: */ 500,
//         /* FormatFn:       */ nil,
//     )
//     defer logger.Close()
//     log.SetOutput(logger)
//
// Custom Formatting example:
//
//     log.SetFlags(0)
//     formatFn := func(msg []byte, buf []byte) ([]byte, int) {
//         now := time.Now().UTC().Format("2006-01-02 15:04:05.000")
//         buf = append(buf, []byte(now)...)   // This always has length 23
//         buf = append(buf, []byte(" : ")...) // This always has length 3
//         buf = append(buf, msg...)           // Therefore, this starts at index 26
//         return buf, 26                      // alternatively, len(now)+len(" : ")
//     }
//     logger := tumble.NewLogger(
//         /* Filepath:       */ "/path/to/foo.log",
//         /* MaxLogSizeMB:   */ 100,
//         /* MaxTotalSizeMB: */ 500,
//         /* FormatFn:       */ formatFn,
//     )
//     defer logger.Close()
//     log.SetOutput(logger)
//
// Note: maxTotalSizeMB is not precise. It may be temporarily exceeded
//       during rotation by the amount of MaxLogSizeMB.
//
type Logger struct {
	Filepath       string
	MaxLogSizeMB   uint
	MaxTotalSizeMB uint
	FormatFn       func(msg []byte, buf []byte) ([]byte, int)

	file         io.WriteCloser
	size         int64
	millCh       chan struct{}
	millWG       sync.WaitGroup
	stopMillOnce sync.Once
	fmtbuf       []byte
}

// Muster is an io.ReadCloser which produces the full history of
// the given log file and its archives seamlessly and in order.
type Muster struct {
	Filepath string

	latestTs           Timestamp
	unreadyTs          Timestamp
	openArchives       []io.Closer
	archiveMultireader io.Reader
	lastOpenFile       io.ReadCloser
}
