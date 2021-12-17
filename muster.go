package tumble

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Timestamp = int64

const BIG_TIMESTAMP = Timestamp(999999999999)
const SLEEP_TIME = 100 * time.Millisecond

// Ensure we always implement io.ReadCloser
var _ io.ReadCloser = (*Muster)(nil)

func NewMuster(fpath string) *Muster {
	muster := &Muster{
		/* Filepath: */ filepath.Clean(fpath),

		/* latestTs           */ Timestamp(0),
		/* unreadyTs          */ BIG_TIMESTAMP,
		/* openArchives       */ nil,
		/* archiveMultireader */ nil,
		/* lastOpenFile       */ nil,
	}
	return muster
}

// This is:
//           ""  in          "foo.log",
//        "tmp/  in      "tmp/foo.log"
//   "/path/to/" in "/path/to/foo.log"
//
func (me *Muster) dirpath() string {
	if filepath.Dir(me.Filepath) == "." {
		return ""
	}
	return filepath.Dir(me.Filepath) + "/"
}

// This is "foo" in "/path/to/foo.log"
func (me *Muster) namePrefix() string {
	return me.Filepath[len(me.dirpath()) : len(me.Filepath)-len(me.nameExt())]
}

// This is ".log" in "/path/to/foo.log"
func (me *Muster) nameExt() string {
	return filepath.Ext(me.Filepath)
}

func (me *Muster) timestampToFpath(ts Timestamp) string {
	return fmt.Sprintf("%s%s-%d%s%s", me.dirpath(), me.namePrefix(), ts, me.nameExt(), compressSuffix)
}

func (me *Muster) timestampLength() int {
	// Today we use a length-10 dateint (but this could be changed)
	return 10
}

func (me *Muster) parseTimestamp(s string) (Timestamp, error) {
	if len(s) != me.timestampLength() {
		return 0, errors.New("invalid timestamp")
	}

	ts, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.New("invalid timestamp")
	}

	return ts, nil
}

func (me *Muster) fpathToTimestamp(fpath string) (Timestamp, error) {
	dirpath := me.dirpath()
	namePrefix := me.namePrefix()
	nameExt := me.nameExt()

	// fpath must be exactly this long to possibly match
	if len(fpath) != len(dirpath)+len(namePrefix)+len("-")+me.timestampLength()+len(nameExt)+len(compressSuffix) {
		return 0, errors.New("mismatch")
	}

	middle := fpath[len(dirpath)+len(namePrefix) : len(fpath)-len(nameExt)-len(compressSuffix)]

	// middle should be a hyphen followed by a timestamp
	if !strings.HasPrefix(middle, "-") {
		return 0, errors.New("mismatch")
	}

	// middle (after hyphen) should be exactly a timestamp
	ts, err := me.parseTimestamp(middle[1:])
	if err != nil {
		return 0, errors.New("mismatch")
	}

	// finally, check that this is our log (rather than another with the same length)
	if fpath != me.timestampToFpath(ts) {
		return 0, errors.New("mismatch")
	}

	return ts, nil
}

func getOpenFilesLimit() uint64 {
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		// This isn't worth crashing over. Just use a default.
		return 1024
	}
	return rlimit.Cur
}

func (me *Muster) getNewTimestamps() ([]Timestamp, error) {
	files, err := os.ReadDir(filepath.Dir(me.Filepath))
	if err != nil {
		return nil, fmt.Errorf("error listing timestamps: %w", err)
	}

	// Reset the unready timestamp each time
	me.unreadyTs = BIG_TIMESTAMP

	// potentialTimestamps are archive timestamps greater than me.latestTs
	dirpath := me.dirpath()
	potentialTimestamps := []Timestamp{}
	for _, f := range files {
		// Check for a currently-compressing file.
		ts, err := me.fpathToTimestamp(dirpath + f.Name() + compressSuffix)
		if err == nil {
			if ts < me.unreadyTs {
				me.unreadyTs = ts
			}
			continue
		}

		// Check for a compressed archive
		ts, err = me.fpathToTimestamp(dirpath + f.Name())
		if err != nil {
			continue
		}

		// Add any timestamp greater than the latest one.
		// We will filter unready ones later once we know the unready ceiling.
		if ts > me.latestTs {
			potentialTimestamps = append(potentialTimestamps, ts)
		}
	}

	// Reduce to ready timestamps
	readyTimestamps := make([]Timestamp, 0, len(potentialTimestamps))
	for _, ts := range potentialTimestamps {
		if ts < me.unreadyTs {
			readyTimestamps = append(readyTimestamps, ts)
		}
	}

	// Sort ready timestamps in descending order, limited to MaxArchiveLookback()
	sort.Slice(readyTimestamps, func(i, j int) bool { return readyTimestamps[i] > readyTimestamps[j] })
	if len(readyTimestamps) > me.MaxArchiveLookback() {
		readyTimestamps = readyTimestamps[:me.MaxArchiveLookback()]
	}

	if len(readyTimestamps) > 0 {
		me.latestTs = readyTimestamps[0]
	}
	return readyTimestamps, nil
}

func (me *Muster) loadArchives() error {
	timestamps, err := me.getNewTimestamps()
	if err != nil {
		return fmt.Errorf("error processing archives: %w", err)
	}

	// Open all the files in one go from newest to oldest, stopping at
	// a NotExist error (the file was probably deleted by rotation).
	me.openArchives = make([]io.Closer, 0, len(timestamps))
	readers := make([]io.Reader, 0, len(timestamps))
	for _, ts := range timestamps {
		// Open the file, adding it to me.openArchives
		fpath := me.timestampToFpath(ts)
		f, err := os.Open(fpath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break
			} else {
				return fmt.Errorf("error opening %s: %w", fpath, err)
			}
		}
		me.openArchives = append(me.openArchives, f)

		// Create a decompression reader to be used in a MultiReader below
		gzReader, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return fmt.Errorf("error creating decompression reader for %s: %w", fpath, err)
		}
		readers = append(readers, gzReader)
	}

	if len(readers) > 0 {
		// Read from oldest to newest
		reverseSliceReader(readers)

		me.archiveMultireader = io.MultiReader(readers...)
	}
	return nil
}

func (me *Muster) closeAllOpenArchives() error {
	var ERR error
	for _, f := range me.openArchives {
		if err := f.Close(); ERR == nil {
			ERR = err
		}
	}
	return ERR
}

func (me *Muster) MaxArchiveLookback() int {
	// We will use 75% of the open files soft limit as our archive lookback
	return int(0.75 * float64(getOpenFilesLimit()))
}

func (me *Muster) Read(p []byte) (int, error) {
	for me.lastOpenFile == nil {
		// Here, me.archiveMultireader is nil in two sitations:
		//
		//  - On the first Read().
		//
		//  - On the next Read() immediately after we close all open
		//    archive files. This is to check for any new archives
		//    before moving on to the final (current) logfile.
		//
		if me.archiveMultireader == nil {
			if err := me.loadArchives(); err != nil {
				return 0, fmt.Errorf("error in read: %w", err)
			}
		}

		// Here, me.archiveMultireader may have just been
		// created by me.loadArchives(), above.
		//
		// Most of the time, it was not just created, and instead
		// we are continuing an ongoing Read() from the existing
		// multireader.
		//
		// Either way, we will not leave this section without
		// returning. In either a nil error or io.EOF from
		// the multireader, we expect to be called again.
		//
		if me.archiveMultireader != nil {
			n, readErr := me.archiveMultireader.Read(p)
			if readErr == nil {
				return n, nil
			}
			closeErr := me.closeAllOpenArchives()
			me.archiveMultireader = nil

			if readErr != io.EOF {
				return n, fmt.Errorf("error in read: %w", readErr)
			}
			if closeErr != nil {
				return n, fmt.Errorf("error in close: %w", closeErr)
			}

			return n, nil
		}

		// When we make it to here, we have processed all ready archives,
		// but there could still be an unready archive. In that case,
		// we must wait for everything to be ready before proceeding.
		if me.unreadyTs < BIG_TIMESTAMP {
			time.Sleep(SLEEP_TIME)
			continue
		}

		// When we make it to here, we have just checked and confirmed that
		// there are no more unprocessed archives. However, we don't yet
		// have a read handle on the final (current) logfile.
		f, err := os.Open(me.Filepath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// The file may have just been rotated. Check for new files:
				me.loadArchives()

				// If we now have work to do, then all is well.
				if me.archiveMultireader != nil {
					continue
				}
				if me.unreadyTs < BIG_TIMESTAMP {
					time.Sleep(SLEEP_TIME)
					continue
				}

				// The logfile is really gone.
			}
			return 0, fmt.Errorf("error opening %s: %w", me.Filepath, err)
		}

		// We are now ready for the final phase
		me.lastOpenFile = f
	}

	// When we make it to here, we are processing the final logfile.
	n, readErr := me.lastOpenFile.Read(p)
	if readErr == nil {
		return n, nil
	}
	closeErr := me.lastOpenFile.Close()

	if readErr != io.EOF {
		return n, fmt.Errorf("error in read: %w", readErr)
	}
	if closeErr != nil {
		return n, fmt.Errorf("error in close: %w", closeErr)
	}
	return n, io.EOF
}

func (me *Muster) Close() error {
	me.archiveMultireader = nil
	me.lastOpenFile = nil
	return nil
}
