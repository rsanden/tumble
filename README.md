# Tumble #

Tumble is a rolling log library and command-line utility in Go. \
It is a derivative of [Lumberjack](https://github.com/natefinch/lumberjack) and available under the same license.

The main operational changes are as follows:

 - Logs are retained based on their total compressed size only.
 - There is no longer a maximum size for a single log message.
 - Rotated logs use a unix timestamp (seconds since epoch).
 - Logfiles/Archives use 644 permissions.
 - Logfiles/Archives are not chown'ed.
 - No locking. Asynchronous Rotate() support removed.
 - Allows a formatting callback to be provided to set the timestamp format.
 - Includes a -dump option to print a log along with any archives

Many other configuration options are removed (no maximum archive age, compression is always enabled, etc).

Parameters:

    fpath:          Path to the logfile
    maxLogSizeMB:   Logfile size before it gets rotation (in MB)
    maxTotalSizeMB: Total disk space of active log + compressed archives (in MB)
    formatFn:       Log message formatting function (optional)

**Default formatting example:**

```go
import "github.com/rsanden/tumble"

log.SetFlags(log.LstdFlags | log.Lmicroseconds)
logger := tumble.NewLogger(
    /* Filepath:       */ "/path/to/foo.log",
    /* MaxLogSizeMB:   */ 100,
    /* MaxTotalSizeMB: */ 500,
    /* FormatFn:       */ nil,
)
defer logger.Close()
log.SetOutput(logger)
```

**Custom Formatting example:**

```go
import "github.com/rsanden/tumble"

log.SetFlags(0)
formatFn := func(msg []byte, buf []byte) ([]byte, int) {
    now := time.Now().UTC().Format("2006-01-02 15:04:05.000")
    buf = append(buf, []byte(now)...)   // This always has length 23
    buf = append(buf, []byte(" : ")...) // This always has length 3
    buf = append(buf, msg...)           // Therefore, this starts at index 26
    return buf, 26                      // alternatively, len(now)+len(" : ")
}
logger := tumble.NewLogger(
    /* Filepath:       */ "/path/to/foo.log",
    /* MaxLogSizeMB:   */ 100,
    /* MaxTotalSizeMB: */ 500,
    /* FormatFn:       */ formatFn,
)
defer logger.Close()
log.SetOutput(logger)
```

Note: **maxTotalSizeMB** is not precise. It may be temporarily exceeded during rotation by the amount of **MaxLogSizeMB**.
