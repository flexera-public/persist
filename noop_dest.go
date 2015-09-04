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

func (nd *noopDest) Close() {}

func (nd *noopDest) Write(p []byte) (int, error) {
	return len(p), nil
}

func (nd *noopDest) ReplayReaders() []io.ReadCloser {
	return nil
}

// StartRotate is called by persist in order to start a new log file.
func (nd *noopDest) StartRotate() error {
	return nil
}

func (nd *noopDest) EndRotate() error {
	return nil
}
