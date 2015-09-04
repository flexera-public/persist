// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/inconshreveable/log15.v2"
)

type fileDest struct {
	basepath       string
	replayReaders  []io.ReadCloser
	outputFile     *os.File
	outputFilename string
	oldFilename    string // name of previous file (used at end of rotation)
	snapOK         bool   // true when the initial snapshot is completed
	log            log15.Logger
}

const (
	newExt  = "-new.plog"        // new log with incomplete initial snapshot
	currExt = "-curr.plog"       // current log with complete initial snapshot
	oldExt  = "-old.plog"        // old log no longer needed
	dateFmt = "-20060102-150405" // format for timestamp added for log files
)

// NewFileDest creates or opens a file for logging. The basepath must not contain any character
// in the set '*', '?', '[', '\', or '.'. The individual log file names will have a -<timestamp>
// and possibly a <-new>, <-curr>, and '.plog' extension appended.
// The create argument determines whether it's OK to create a new set of log files or whether
// an existing set is expected to be found.
func NewFileDest(basepath string, create bool, log log15.Logger) (LogDestination, error) {
	if log == nil {
		log = log15.Root()
	}
	log = log.New("basepath", basepath)

	if strings.ContainsAny(basepath, "*?[\\.") {
		return nil, fmt.Errorf("basepath cannot contain '*', '?', '[', '\\' or '.'")
	}
	m, err := filepath.Glob(basepath + "*.plog")
	if err != nil {
		return nil, fmt.Errorf("basepath invalid: %s", err.Error())
	}

	fd := &fileDest{basepath: basepath, log: log}

	if len(m) > 0 {
		sort.Strings(m)
		lm := len(m) - 1
		if strings.HasSuffix(m[lm], currExt) {
			// the most recent log file is current, i.e. it's all we need
			f0, err := os.Open(m[lm])
			if err != nil {
				return nil, fmt.Errorf("error opening %s: %s", m[lm], err.Error())
			}
			fd.replayReaders = []io.ReadCloser{f0}
			fd.oldFilename = m[lm]
			stat, _ := f0.Stat()
			log.Info("Opening existing log, replaying one file",
				"file1", m[lm], "len1", stat.Size())
		} else if strings.HasSuffix(m[lm], newExt) && lm > 0 &&
			strings.HasSuffix(m[lm-1], currExt) {
			// the most recent log is not a complete snapshot, we need it and
			// the prior log file (and we have both)
			// we create a multi-reader that reads from the prior log file and then
			// from the new one
			f0, err := os.Open(m[lm-1])
			if err != nil {
				return nil, fmt.Errorf("error opening %s: %s", m[lm-1], err.Error())
			}
			f1, err := os.Open(m[lm])
			if err != nil {
				f0.Close()
				return nil, fmt.Errorf("error opening %s: %s", m[lm], err.Error())
			}
			fd.replayReaders = []io.ReadCloser{f0, f1}
			fd.oldFilename = m[lm]
			log.Info("Opening existing log, replaying two files", "file1", m[lm-1],
				"file2", m[lm])
		} else {
			return nil, fmt.Errorf(
				"Cannot determine current (&new) logs from basepath %s", basepath)
		}
	} else if !create {
		return nil, fmt.Errorf("No existing log file found at %s", basepath)
	} else {
		log.Info("No existing log found, creating a new one")
	}

	// Open new destination
	err = fd.startNew(len(m) > 0)
	if err != nil {
		if fd.replayReaders != nil {
			for _, rr := range fd.replayReaders {
				rr.Close()
			}
			fd.replayReaders = nil
		}
		return nil, err
	}
	return fd, nil
}

// createNewFile attempts to create a new file and keeps adding from 'a' to 'z' to ensure it
// doesn't open an existing file
// TODO: can't create foo-new.plog if foo-curr.plog exists!
func createNewFile(name, ext string) (*os.File, string, error) {
	for i := '`'; i <= 'z'; i++ {
		n := name
		if i != '`' { // '`' is just before 'a', signifies no suffix
			n += string(i)
		}
		// check that we have no file with this suffix
		if m, _ := filepath.Glob(n + "*"); len(m) > 0 {
			continue
		}

		// try to create, making sure it does not exist
		n += ext
		fd, err := os.OpenFile(n, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0660)
		if err == nil {
			// success, return this one
			return fd, n, nil
		}
		if err != os.ErrExist {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("Too many log files in the same second")
}

// start a new log file and use either the newExt (normal case) or the currExt (when creating the
// very first log file since there's no preceding currExt file)
func (fd *fileDest) startNew(useNewExt bool) error {
	// work out filename
	date := time.Now().UTC().Format(dateFmt)
	name := fd.basepath + date
	ext := currExt
	if useNewExt {
		ext = newExt
	}

	// create it and deal with errors
	outF, outFn, err := createNewFile(name, ext)
	if err != nil {
		return fmt.Errorf("Cannot create new log file: %s", err.Error())
	}
	fd.log.Info("Starting new log file", "file", outF.Name())
	fd.outputFile = outF
	fd.outputFilename = outFn
	fd.snapOK = false
	return nil
}

func (fd *fileDest) Close() {
	if fd.replayReaders != nil {
		for _, rr := range fd.replayReaders {
			rr.Close()
		}
		fd.replayReaders = nil
	}
	if fd.outputFile != nil {
		fd.outputFile.Close()
		fd.outputFile = nil
		fd.outputFilename = ""
	}
	fd.basepath = ""
}

func (fd *fileDest) Write(p []byte) (int, error) {
	return fd.outputFile.Write(p)
}

func (fd *fileDest) ReplayReaders() []io.ReadCloser {
	return fd.replayReaders
}

// StartRotate is called by persist in order to start a new log file.
func (fd *fileDest) StartRotate() error {
	if !fd.snapOK {
		return fmt.Errorf("Cannot rotate: initial snapshot incomplete")
	}
	fd.outputFile.Close()
	fd.outputFile = nil
	fd.oldFilename = fd.outputFilename
	fd.outputFilename = ""
	return fd.startNew(true)
}

// EndRotate is called by persist in order to signal the completion of the initial snapshot
// on the new log file. It is called after StartRotate() and after NewFileDest(), i.e., there's
// an implicit StartRotate() when the destination is initially created.
func (fd *fileDest) EndRotate() error {
	if fd.snapOK {
		return fmt.Errorf("internal error: StartRotate not called")
	}

	// if we started a new log and there's no replay, then the first file has
	// currExt and there's nothing to do. If we opened an existing log, then the
	// current file has newExt and we need some renaming to make it currExt
	if fd.oldFilename == "" {
		if !strings.HasSuffix(fd.outputFilename, currExt) {
			return fmt.Errorf(
				"internal error: first log file (%s) should have %s suffix",
				fd.outputFilename, currExt)
		}
		fd.snapOK = true
		fd.log.Info("New log file now initialized")
		return nil
	}
	if !strings.HasSuffix(fd.outputFilename, newExt) {
		return fmt.Errorf("internal error: new log file (%s) does not have %s suffix !?",
			fd.outputFilename, newExt)
	}

	// Rename new log file
	newName := strings.TrimSuffix(fd.outputFilename, newExt) + currExt
	err := os.Rename(fd.outputFilename, newName)
	if err != nil {
		return err
	}
	fd.outputFilename = newName
	fd.log.Info("New log file now initialized & renamed", "file", newName)

	// Rename old file
	var oldName string // new name for old file...
	if strings.HasSuffix(fd.oldFilename, currExt) {
		oldName = strings.TrimSuffix(fd.oldFilename, currExt) + oldExt
	} else if strings.HasSuffix(fd.oldFilename, newExt) {
		oldName = strings.TrimSuffix(fd.oldFilename, newExt) + oldExt
		// TODO: should really also rename the log file prior to that, which must
		// have a currExt
	} else {
		return fmt.Errorf("internal error: old log file (%s) doesn't have %s or %s suffix",
			fd.oldFilename, currExt, newExt)
	}
	fd.log.Info("Old log file now superceded", "file", oldName)
	err = os.Rename(fd.oldFilename, oldName)
	if err != nil {
		return err
	}
	fd.oldFilename = ""
	fd.snapOK = true

	return nil
}
