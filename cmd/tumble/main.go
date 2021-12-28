package main

import (
	_ "embed"

	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	timeFormat   string
	formatFn     func(msg []byte, buf []byte) ([]byte, int)
	isDump       bool
	isVersion    bool

	//go:embed VERSION.txt
	VERSION string
)

func init_globals() {
	var dumpfile string

	flag.StringVar(&logfile /*****/, "logfile" /*********/, "" /*****/, "path to logfile (required)")
	flag.UintVar(&maxLogSize /****/, "max-log-size" /****/, 0 /******/, "max log size before rotation (in MB) (required)")
	flag.UintVar(&maxTotalSize /**/, "max-total-size" /**/, 0 /******/, "max total size before deletion (in MB) (required)")
	flag.BoolVar(&isTeeStdout /***/, "tee-stdout" /******/, false /**/, "tee to stdout (default: false)")
	flag.BoolVar(&isTeeStderr /***/, "tee-stderr" /******/, false /**/, "tee to stderr (default: false)")
	flag.StringVar(&timeFormat /**/, "time-format" /*****/, "" /*****/, "add timestamp with given format (default: no timestamp) (example: '2006-01-02 15:04:05.000')")
	flag.StringVar(&dumpfile /****/, "dump" /************/, "" /*****/, "dump archives for given filepath and exit (default: do not dump)")
	flag.BoolVar(&isVersion /*****/, "version" /*********/, false /**/, "print version and exit (default: false)")
	flag.Parse()

	if isVersion {
		fmt.Println(strings.TrimSpace(VERSION))
		os.Exit(0)
	}

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

func runLogBinaryMode(logger *tumble.Logger) error {
	writers := []io.Writer{logger}
	if isTeeStdout {
		writers = append(writers, os.Stdout)
	}
	if isTeeStderr {
		writers = append(writers, os.Stderr)
	}
	_, err := io.Copy(io.MultiWriter(writers...), os.Stdin)
	return err
}

func runLogTextMode(logger *tumble.Logger) error {
	writers := []io.Writer{logger}
	if isTeeStdout {
		writers = append(writers, os.Stdout)
	}
	if isTeeStderr {
		writers = append(writers, os.Stderr)
	}
	multiWriter := io.MultiWriter(writers...)

	var line []byte
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line = line[:0]
		line = append(line, scanner.Bytes()...)
		line = append(line, '\n')
		if _, err := multiWriter.Write(line); err != nil {
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
	if timeFormat != "" {
		runFn = runLogTextMode
	} else {
		runFn = runLogBinaryMode
	}

	// Schedule cleanup on interrupt
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-sigCh
		switch sig {
		case os.Interrupt, syscall.SIGINT, syscall.SIGTERM:
			logger.StopMill()
		case syscall.SIGHUP:
			logger.RotateClose()
			os.Exit(0)
		}
	}()

	return runFn(logger)
}

func runDump() error {
	muster := tumble.NewMuster(
		/* Filepath: */ logfile,
	)
	defer muster.Close()

	writers := []io.Writer{os.Stdout}
	if isTeeStderr {
		writers = append(writers, os.Stderr)
	}
	_, err := io.Copy(io.MultiWriter(writers...), muster)
	return err
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
