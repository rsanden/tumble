package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rsanden/tumble"
)

var (
	logfile      string
	maxLogSize   uint
	maxTotalSize uint
	isTeeStdout  bool
	isTeeStderr  bool
	timeFormat   string
	formatFn     func(msg []byte, buf []byte) ([]byte, int)
)

func init_globals() {
	flag.StringVar(&logfile /*****/, "logfile" /*********/, "" /*****/, "path to logfile (required)")
	flag.UintVar(&maxLogSize /****/, "max-log-size" /****/, 0 /******/, "max log size before rotation (in MB) (required)")
	flag.UintVar(&maxTotalSize /**/, "max-total-size" /**/, 0 /******/, "max total size before deletion (in MB) (required)")
	flag.BoolVar(&isTeeStdout /***/, "tee-stdout" /******/, false /**/, "tee to stdout")
	flag.BoolVar(&isTeeStderr /***/, "tee-stderr" /******/, false /**/, "tee to stderr")
	flag.StringVar(&timeFormat /**/, "time-format" /*****/, "" /*****/, "add timestamp with given format (default: no timestamp) (example: '2006-01-02 15:04:05.000')")
	flag.Parse()

	if logfile == "" || maxLogSize == 0 || maxTotalSize == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if timeFormat != "" {
		formatFn = func(msg []byte, buf []byte) ([]byte, int) {
			now := time.Now().UTC().Format(timeFormat)
			buf = append(buf, []byte(now)...)
			buf = append(buf, []byte(" : ")...)
			buf = append(buf, msg...)
			return buf, len(now) + len(" : ")
		}
	}
}

func main() {
	init_globals()

	logger := tumble.Logger{
		Filepath:       logfile,
		MaxLogSizeMB:   maxLogSize,
		MaxTotalSizeMB: maxTotalSize,
		FormatFn:       formatFn,
	}
	defer logger.Close()

	var line []byte
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line = []byte(scanner.Text() + "\n")
		logger.Write(line)
		if isTeeStdout {
			os.Stdout.Write(line)
		}
		if isTeeStderr {
			os.Stderr.Write(line)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "error reading stdin:", err)
	}
}
