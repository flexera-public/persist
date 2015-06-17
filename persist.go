// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

import (
	"encoding/gob"
	"fmt"
	"io"
	"sync"
)

type pLog struct {
	client    LogClient
	size      int
	sizeLimit int
	encoder   *gob.Encoder
	decoder   *gob.Decoder
	dest      []LogDestination
	errState  error
	sync.Mutex
}

func (pl *pLog) SetSizeLimit(bytes int) { pl.sizeLimit = bytes }

func (pl *pLog) HealthCheck() error { return pl.errState }

func (pl *pLog) Register(value interface{}) { gob.Register(value) }

func (pl *pLog) Write(logEvent interface{}) error {
	pl.Lock()
	defer pl.Unlock()

	if pl.errState != nil {
		return pl.errState
	}
	if pl.encoder == nil {
		return fmt.Errorf("uninitialized persistence log (nil encoder)")
	}
	err := pl.encoder.Encode(logEvent)
	if err != nil {
		pl.errState = err
	}
	if pl.size > pl.sizeLimit {
		err = pl.rotate()
	}
	return err
}

func (pl *pLog) AddDestination(dest LogDestination) error {
	pl.Lock()
	defer pl.Unlock()

	pl.dest = append(pl.dest, dest)

	return pl.rotate()
}

func (pl *pLog) rotate() error {
	for i, d := range pl.dest {
		err := d.StartRotate()
		if i == 0 && err != nil {
			pl.errState = err
			return err
		}
	}
	pl.client.PersistAll()
	for i, d := range pl.dest {
		err := d.EndRotate()
		if i == 0 && err != nil {
			pl.errState = err
			return err
		}
	}
	return nil
}

func (pl *pLog) replay() error {
	for {
		var ev interface{}
		err := pl.decoder.Decode(&ev)
		if err == io.EOF {
			return nil // done replaying
		}
		if err != nil {
			return fmt.Errorf("replay failed: %s", err.Error())
		}
		err = pl.client.Replay(ev)
		if err != nil {
			return fmt.Errorf("replay failed: %s", err.Error())
		}
	}
}

type plReadWriter struct {
	pl *pLog
}

func (plrw *plReadWriter) Write(p []byte) (n int, err error) {
	l := len(p)
	plrw.pl.size += len(p)

	for i, d := range plrw.pl.dest {
		n, err := d.Write(p)
		if i == 0 && (n != l || err != nil) {
			plrw.pl.errState = err
			return 0, err
		}
	}
	return l, nil
}

func (plrw *plReadWriter) Read(p []byte) (n int, err error) {
	return plrw.pl.dest[0].Read(p)
}

// NewLog reopens an existing log, replays all log entries, and then prepares to append
// to it. The call to NewLog completes once any necessary replay has completed.
func NewLog(dest LogDestination, client LogClient) (Log, error) {
	pl := &pLog{
		client:    client,
		sizeLimit: 1024 * 1024, // 1MB default
		dest:      []LogDestination{dest},
	}
	plrw := &plReadWriter{pl: pl}
	pl.encoder = gob.NewEncoder(plrw)
	pl.decoder = gob.NewDecoder(plrw)

	err := pl.replay()
	return pl, err
}
