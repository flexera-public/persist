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
	client     LogClient // client which we make callbacks
	size       int       // size used to decide when to rotate
	sizeLimit  int       // size limit when to rotate
	sizeReplay int       // size of the initial replay
	objects    uint64    // number of objects output, purely for stats
	encoder    *gob.Encoder
	priDest    LogDestination // primary dest, where we initially replay from
	secDest    LogDestination // secondary dest, no replay and OK if "down"
	rotating   bool           // avoid concurrent rotations
	errState   error
	log        log15.Logger
	sync.Mutex
}

// Return some statistics about the logging
func (pl *pLog) Stats() map[string]float64 {
	pl.Lock()
	defer pl.Unlock()

	stats := make(map[string]float64)
	stats["LogSizeReplay"] = float64(pl.sizeReplay)
	stats["LogSize"] = float64(pl.size + pl.sizeReplay)
	stats["LogSizeLimit"] = float64(pl.sizeLimit)
	stats["ObjectOutputRate"] = float64(pl.objects)
	stats["ErrorState"] = 0.0
	if pl.errState != nil {
		stats["ErrorState"] = 1.0
	}
	return stats
}

// Close the log for test purposes
func (pl *pLog) Close() {
	for {
		pl.Lock()
		if !pl.rotating {
			break
		}
		pl.Unlock()
		time.Sleep(1 * time.Millisecond)
	}

	pl.priDest.Close()
	if pl.secDest != nil {
		pl.secDest.Close()
	}
	pl.Unlock()
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
	pl.objects += 1
	var t interface{} = logEvent
	err := pl.encoder.Encode(&t)
	if err != nil {
		pl.errState = err
	} else if !pl.rotating && pl.size > pl.sizeLimit {
		pl.rotate()
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

// perform a log rotation, must be called while holding the pl.Lock()
func (pl *pLog) rotate() {
	if pl.rotating {
		return
	}
	pl.rotating = true
	pl.log.Info("Persist: starting rotation")
	go pl.finishRotate()
}

func (pl *pLog) finishRotate() {
	// tell all log destinations to start a rotation
	pl.Lock()
	defer pl.Unlock()
	pl.size = 0
	pl.sizeReplay = 0
	err := pl.priDest.StartRotate()
	if pl.secDest != nil {
		pl.secDest.StartRotate() // TODO: record error
	}
	if err != nil {
		pl.errState = err
		return
	}
	// we need a new encoder 'cause we start a fresh stream
	pl.encoder = gob.NewEncoder(pl)

	// now create a full snapshot, relinquish the lock while doing that 'cause otherwise
	// we end up with deadlocks since PersistAll will end up calling pl.Output()
	pl.Unlock()
	pl.client.PersistAll(pl)
	pl.Lock()

	// tell all log destinations that we're done with the rotation
	err = pl.priDest.EndRotate()
	if pl.secDest != nil {
		pl.secDest.EndRotate() // TODO: record error
	}
	pl.rotating = false
	if err != nil {
		pl.log.Crit("Finished rotation with error",
			"replay_size", pl.sizeReplay, "err", err)
		pl.errState = err
	} else {
		pl.log.Info("Finished rotation", "replay_size", pl.sizeReplay)
	}
	return
}

// replay a log file
func (pl *pLog) replay() (err error) {
	for i, rr := range pl.priDest.ReplayReaders() {
		pl.log.Info("Starting replay", "log_num", i+1)
		dec := gob.NewDecoder(rr)
		// iterate reading one log entry after another until EOF is reached
		count := 0
		for {
			var ev interface{}
			err := dec.Decode(&ev)
			if err == io.EOF {
				break // done replaying
			}
			if err != nil {
				pl.log.Debug("replay decode failed", "err", err, "log_num", i+1,
					"count", count)
				return fmt.Errorf("replay decode failed in log %d after %d entries: %s",
					i+1, count, err.Error())
			}
			//pl.log.Debug("replay decoded", "ev", ev)
			count += 1
			err = pl.client.Replay(ev)
			if err != nil {
				return fmt.Errorf("replay failed on entry %d: %s", count, err.Error())
			}
		}
		rr.Close()
	}
	pl.log.Debug("Ending replay", "logs", len(pl.priDest.ReplayReaders()))
	return nil
}

// Write is called by the gob encoder and needs to write the bytes to all destinations
func (pl *pLog) Write(p []byte) (int, error) {
	if pl.errState != nil {
		return 0, pl.errState // in error state don't move!
	}

	// track size for log rotation, initial snapshot doesn't count towards limit
	l := len(p)
	if !pl.rotating {
		pl.size += len(p)
	} else {
		pl.sizeReplay += len(p)
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

	pl.log.Debug("Starting replay")
	err := pl.replay()
	if err != nil {
		pl.errState = err
		return nil, err
	}
	pl.log.Info("Replay done")

	// now create a full snapshot
	pl.log.Debug("Starting snapshot")
	pl.rotating = true
	pl.client.PersistAll(pl)
	pl.rotating = false
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
