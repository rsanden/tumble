package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rsanden/tumble"
)

const BUF_SIZE = 32 * 1024

var (
	logfile      string
	maxLogSize   uint
	maxTotalSize uint
	isTeeStdout  bool
	isTeeStderr  bool
	isBinary     bool
	timeFormat   string
	formatFn     func(msg []byte, buf []byte) ([]byte, int)
	isDump       bool
)

func init_globals() {
	var dumpfile string

	flag.StringVar(&logfile /*****/, "logfile" /*********/, "" /*****/, "path to logfile (required)")
	flag.UintVar(&maxLogSize /****/, "max-log-size" /****/, 0 /******/, "max log size before rotation (in MB) (required)")
	flag.UintVar(&maxTotalSize /**/, "max-total-size" /**/, 0 /******/, "max total size before deletion (in MB) (required)")
	flag.BoolVar(&isTeeStdout /***/, "tee-stdout" /******/, false /**/, "tee to stdout (default: false)")
	flag.BoolVar(&isTeeStderr /***/, "tee-stderr" /******/, false /**/, "tee to stderr (default: false)")
	flag.BoolVar(&isBinary /******/, "binary" /**********/, false /**/, "raw binary input (default: false)")
	flag.StringVar(&timeFormat /**/, "time-format" /*****/, "" /*****/, "add timestamp with given format (default: no timestamp) (example: '2006-01-02 15:04:05.000')")
	flag.StringVar(&dumpfile /****/, "dump" /************/, "" /*****/, "dump archives for given filepath and exit (default: do not dump)")
	flag.Parse()

	if dumpfile != "" {
		if logfile != "" || maxLogSize != 0 || maxTotalSize != 0 {
			flag.Usage()
			os.Exit(1)
		}
		isDump = true
		logfile = dumpfile
	} else {
		if logfile == "" || maxLogSize == 0 || maxTotalSize == 0 {
			flag.Usage()
			os.Exit(1)
		}
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

func writeLogData(logger *tumble.Logger, buf []byte) error {
	var ERR error
	if _, err := logger.Write(buf); ERR == nil {
		ERR = err
	}
	if isTeeStdout {
		if _, err := os.Stdout.Write(buf); ERR == nil {
			ERR = err
		}
	}
	if isTeeStderr {
		if _, err := os.Stderr.Write(buf); ERR == nil {
			ERR = err
		}
	}
	return ERR
}

func runLogBinaryMode(logger *tumble.Logger) error {
	buf := make([]byte, BUF_SIZE)
	for {
		n, readErr := os.Stdin.Read(buf)
		writeErr := writeLogData(logger, buf[:n])
		if readErr == io.EOF {
			return writeErr
		}
		if readErr != nil {
			return readErr
		}
		if writeErr != nil {
			return writeErr
		}
	}
}

func runLogTextMode(logger *tumble.Logger) error {
	var line []byte
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line = line[:0]
		line = append(line, scanner.Bytes()...)
		line = append(line, '\n')
		if err := writeLogData(logger, line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func runLog() error {
	logger := tumble.NewLogger(
		/* Filepath:       */ logfile,
		/* MaxLogSizeMB:   */ maxLogSize,
		/* MaxTotalSizeMB: */ maxTotalSize,
		/* FormatFn:       */ formatFn,
	)
	defer logger.Close()

	var runFn func(logger *tumble.Logger) error
	if isBinary {
		runFn = runLogBinaryMode
	} else {
		runFn = runLogTextMode
	}
	return runFn(logger)
}

func writeDumpData(buf []byte) error {
	var ERR error
	if _, err := os.Stdout.Write(buf); ERR == nil {
		ERR = err
	}
	if isTeeStderr {
		if _, err := os.Stderr.Write(buf); ERR == nil {
			ERR = err
		}
	}
	return ERR
}

func runDumpBinaryMode(muster *tumble.Muster) error {
	buf := make([]byte, BUF_SIZE)
	reader := bufio.NewReader(muster)
	for {
		n, readErr := reader.Read(buf)
		writeErr := writeDumpData(buf[:n])
		if readErr == io.EOF {
			return writeErr
		}
		if readErr != nil {
			return readErr
		}
		if writeErr != nil {
			return writeErr
		}
	}
}

func runDumpTextMode(muster *tumble.Muster) error {
	var line []byte
	scanner := bufio.NewScanner(muster)
	for scanner.Scan() {
		line = line[:0]
		line = append(line, scanner.Bytes()...)
		line = append(line, '\n')
		if err := writeDumpData(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func runDump() error {
	muster := tumble.NewMuster(
		/* Filepath: */ logfile,
	)
	defer muster.Close()

	var runFn func(muster *tumble.Muster) error
	if isBinary {
		runFn = runDumpBinaryMode
	} else {
		runFn = runDumpTextMode
	}
	return runFn(muster)
}

func main() {
	init_globals()

	var err error
	if isDump {
		err = runDump()
	} else {
		err = runLog()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error in tumble/main:", err)
	}
}
