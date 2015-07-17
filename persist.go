// Copyright (c) 2015 RightScale, Inc. - see LICENSE

// issues:
// - need to rotate() in NewLog? not clear
// - ordering of replay and persist-all in NewLog not clear

package persist

import (
	"encoding/gob"
	"fmt"
	"io"
	"sync"
	"time"

	"gopkg.in/inconshreveable/log15.v2"
)

type pLog struct {
	client    LogClient // client which we make callbacks
	size      int       // size used to decide when to rotate
	sizeLimit int       // size limit when to rotate
	encoder   *gob.Encoder
	decoder   *gob.Decoder   // becomes nil when done replaying
	priDest   LogDestination // primary dest, where we initially replay from
	secDest   LogDestination // secondary dest, no replay and OK if "down"
	errState  error
	log       log15.Logger
	sync.Mutex
}

// Close the log for test purposes
func (pl *pLog) Close() {
	pl.priDest.Close()
	if pl.secDest != nil {
		pl.secDest.Close()
	}
}

// SetSizeLimit sets the log size limit at which a rotation occurs, the size value is in
// addition to the initial size produced by the initial snapshot, i.e., it doesn't count that
func (pl *pLog) SetSizeLimit(bytes int) { pl.sizeLimit = bytes }

// HealthCheck returns nil if everything is OK and an error if the log is in an error state
func (pl *pLog) HealthCheck() error { return pl.errState }

// hack...
var pLogError bool

// Output a log entry
func (pl *pLog) Output(logEvent interface{}) error {
	pl.Lock()
	defer pl.Unlock()

	//pl.log.Debug("persist.Output", "ev", logEvent)

	if pl.errState != nil {
		if !pLogError {
			pl.log.Crit("Persistence log in error state: " +
				pl.errState.Error())
			pLogError = true
		}
		return pl.errState
	}
	pLogError = false
	if pl.encoder == nil {
		return fmt.Errorf("uninitialized persistence log (nil encoder)")
	}
	// perverse stuff: we need to slap the event into an interface{} so gob later allows
	// us to decode into an interface{}
	var t interface{} = logEvent
	err := pl.encoder.Encode(&t)
	if err != nil {
		pl.errState = err
	} else if pl.size > pl.sizeLimit {
		err = pl.rotate()
	}
	return err
}

func (pl *pLog) SetSecondaryDestination(dest LogDestination) error {
	pl.Lock()
	defer pl.Unlock()

	return fmt.Errorf("not implemented yet!")

	/*
		if pl.secDest != nil {
			return fmt.Errorf("secondary destination is already set")
		}
		pl.secDest = dest

		return pl.rotate()
	*/
}

// perform a log rotation
func (pl *pLog) rotate() error {
	// tell all log destinations to start a rotation
	err := pl.priDest.StartRotate()
	if pl.secDest != nil {
		pl.secDest.StartRotate() // TODO: record error
	}
	if err != nil {
		pl.errState = err
		return err
	}

	// now create a full snapshot
	pl.client.PersistAll(pl)

	// tell all log destinations that we're done with the rotation
	err = pl.priDest.EndRotate()
	if pl.secDest != nil {
		pl.secDest.EndRotate() // TODO: record error
	}
	if err != nil {
		pl.errState = err
		return err
	}
	return nil
}

// replay a log file
func (pl *pLog) replay() (err error) {
	// iterate reading one log entry after another until EOF is reached
	for {
		var ev interface{}
		err := pl.decoder.Decode(&ev)
		if err == io.EOF {
			return nil // done replaying
		}
		if err != nil {
			pl.log.Debug("replay decode failed", "err", err)
			return fmt.Errorf("replay decode failed: %s", err.Error())
		}
		//pl.log.Debug("replay decoded", "ev", ev)
		err = pl.client.Replay(ev)
		if err != nil {
			return fmt.Errorf("replay failed: %s", err.Error())
		}
	}
}

// Write is called by the gob encoder and needs to write the bytes to all destinations
func (pl *pLog) Write(p []byte) (int, error) {
	if pl.errState != nil {
		return 0, pl.errState // in error state don't move!
	}

	// track size for log rotation, initial snapshot doesn't count towards limit
	l := len(p)
	if pl.decoder == nil { // nil means we're done replaying
		pl.size += len(p)
	}

	// write to primary destination
	n, err := pl.priDest.Write(p)
	if n != l || err != nil {
		pl.errState = err
		return n, err
	}

	// write to secondary destination
	if pl.secDest != nil {
		pl.secDest.Write(p) // TODO: record error
	}

	return n, nil
}

// Read is called by the gob decoder and needs to read the replay from the primary dest only
func (pl *pLog) Read(p []byte) (int, error) {
	if pl.errState != nil {
		return 0, pl.errState // in error state don't move!
	}

	n, err := pl.priDest.Read(p)
	if err != nil && err != io.EOF {
		pl.errState = err
	}
	return n, err
}

// NewLog reopens an existing log, replays all log entries, and then prepares to append
// to it. The call to NewLog completes once any necessary replay has completed.
func NewLog(priDest LogDestination, client LogClient, logger log15.Logger) (Log, error) {
	pl := &pLog{
		client:    client,
		sizeLimit: 1024 * 1024, // 1MB default
		priDest:   priDest,
		log:       logger.New("start", time.Now()),
	}
	pl.encoder = gob.NewEncoder(pl)
	pl.decoder = gob.NewDecoder(pl)

	pl.log.Info("Starting replay")
	err := pl.replay()
	if err != nil {
		pl.errState = err
		return nil, err
	}
	pl.log.Info("Replay done")

	// now create a full snapshot
	pl.log.Info("Starting snapshot")
	pl.client.PersistAll(pl)
	pl.log.Info("Snapshot done")

	// tell all log destinations that we're done with the rotation
	err = pl.priDest.EndRotate()
	if pl.secDest != nil {
		pl.secDest.EndRotate() // TODO: record error
	}
	if err != nil {
		pl.errState = err
		return nil, err
	}
	return pl, err
}
