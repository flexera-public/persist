// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type fileDest struct {
	basepath     string
	replayReader io.Reader
	outputFile   *os.File
	oldFilename  string // captured outputFile.Name() when doing StartRotation()
}

func NewFileDest(basepath string, create bool) (LogDestination, error) {
	if strings.ContainsAny(basepath, "*?[\\.") {
		return nil, fmt.Errorf("basepath cannot contain *, ?, [, or \\")
	}
	m, err := filepath.Glob(basepath + "*.plog")
	if err != nil {
		return nil, fmt.Errorf("basepath invalid: %s", err.Error())
	}

	fd := &fileDest{basepath: basepath}

	if len(m) > 0 {
		sort.Strings(m)
		lm := len(m) - 1
		if strings.HasSuffix(m[lm], "-curr.plog") {
			// the most recent log file is current, i.e. all we need
			f0, err := os.Open(m[lm])
			if err != nil {
				return nil, fmt.Errorf("error opening %s: %s", m[lm], err.Error())
			}
			fd.replayReader = bufio.NewReader(f0)
		} else if strings.HasSuffix(m[lm], "-new.plog") && lm > 0 &&
			strings.HasSuffix(m[lm-1], "-curr.plog") {
			// the most recent log is not a complete snapshot, we need it and
			// the prior log file
			f0, err := os.Open(m[lm-1])
			if err != nil {
				return nil, fmt.Errorf("error opening %s: %s", m[lm-1], err.Error())
			}
			f1, err := os.Open(m[lm])
			if err != nil {
				return nil, fmt.Errorf("error opening %s: %s", m[lm], err.Error())
			}
			fd.replayReader = bufio.NewReader(io.MultiReader(f0, f1))
		} else {
			return nil, fmt.Errorf(
				"Cannot determine current (&new) logs from basepath %s", basepath)
		}
	}

	// Open new destination
	err = fd.startNew()
	return fd, err
}

func (fd *fileDest) startNew() error {
	date := time.Now().UTC().Format("20060102-150405")
	name := fd.basepath + date + "-new.plog"
	outF, err := os.Create(name)
	if err != nil {
		return fmt.Errorf(
			"Cannot create new log file %s: %s", name, err.Error())
	}
	fd.outputFile = outF
	return nil
}

func (fd *fileDest) Read(p []byte) (int, error) {
	return fd.replayReader.Read(p)
}

func (fd *fileDest) Write(p []byte) (int, error) {
	return fd.outputFile.Write(p)
}

func (fd *fileDest) StartRotate() error {
	fd.oldFilename = fd.outputFile.Name()
	return fd.startNew()
}

func (fd *fileDest) EndRotate() error {
	if fd.oldFilename == "" {
		return fmt.Errorf("internal error: StartRotate not called")
	}
	if !strings.HasSuffix(fd.oldFilename, "-curr.plog") {
		return fmt.Errorf("internal error: current log file doesn't have -curr suffix")
	}

	name := fd.outputFile.Name()
	if !strings.HasSuffix(name, "-new.plog") {
		return fmt.Errorf("log file does not have -new.plog suffix !?")
	}
	newName := strings.TrimSuffix(name, "-new.plog") + "-curr.plog"
	err := os.Rename(name, newName)
	if err != nil {
		return err
	}

	oldName := strings.TrimSuffix(fd.oldFilename, "-curr.plog") + "-old.plog"
	err = os.Rename(fd.oldFilename, oldName)
	if err != nil {
		return err
	}
	fd.oldFilename = ""

	return nil
}
