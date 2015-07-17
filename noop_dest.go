// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

import (
	"io"

	"gopkg.in/inconshreveable/log15.v2"
)

type noopDest struct {
	log log15.Logger
}

func NewNoopDest(log log15.Logger) (LogDestination, error) {
	nd := noopDest{log: log}
	return &nd, nil
}

func (nd *noopDest) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (nd *noopDest) Close() {}

func (nd *noopDest) Write(p []byte) (int, error) {
	return len(p), nil
}

// StartRotate is called by persist in order to start a new log file.
func (nd *noopDest) StartRotate() error {
	return nil
}

func (nd *noopDest) EndRotate() error {
	return nil
}
