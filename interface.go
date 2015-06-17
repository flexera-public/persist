// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

import "io"

// LogClient is the interface the application needs to implement so the persist can call it back
type LogClient interface {
	// Replay is called by the persistence layer during log replay in order to replay
	// an individual log event. If Replay returns an error the replay is aborted and
	// produces the error.
	// Beware that it it possible that log events are replayed that contain mutations to
	// resources that have not been created yet, i.e. for which the event produced by
	// Enumerate has not been replayed yet. In those cases, Replay must ignore the event
	// because a subsequent create will be Replayed with the correct value for the
	// resource. Note that if events contain updates to multiple resources then some
	// of them may have been created and need updating while others may not have been created
	// and shouldn't be created/updated.
	Replay(logEvent interface{}) error

	// PersistAll is called by the persistence layer in order to enumerate all live resources
	// and persist them by making calls to Log.Write().
	// (If PersistAll encounters an erorr it's time to panic.)
	// PersistAll can run in parallel with new updates to resources however the application
	// must ensure that calls to Log.Write() are in the same order as PersistAll's reads
	// and other update's writes.
	PersistAll()
}

type Log interface {
	// Write an event to the log, this uses gob serialization internally. If an error
	// occurs there is a serious problem with the log, for example, disk full or socket
	// disconnected from the log destination. If the application can unroll the mutations
	// it has performed it should do so and return its client and error. If, however, the
	// application cannot unroll it is recommended not to check for error here and continue
	// to operate optimistically. Once the log problem gets repaired the persist layer will
	// do a log rotation to ensure all live data is captured.
	Write(logEvent interface{}) error

	// SetSizeLimit determines when the persist layer should rotate logs. The default is
	// 10MB
	SetSizeLimit(bytes int)

	// AddDestination adds additional destinations to the Log (not yet implemented)
	AddDestination(dest LogDestination) error

	// HealthCheck returns any persistent error encountered in persist that prevents it
	// from logging. If HealthCheck() returns an error then all Write() calls will return
	// the same error. If the problem is fixed the error will eventually go away again and
	// the log will be "repaired" by doing a rotation. The intent of the HealthCheck call
	// is for the application to be able to reject requests early if the logging is broken.
	HealthCheck() error

	// Register a type being written to the log, this must be called for each type passed
	// to Write and for any type expected in an interface type inside an event. This calls
	// gob.Register() internally, please see the gob docs
	Register(value interface{})
}

type LogDestination interface {
	StartRotate() error
	EndRotate() error
	io.ReadWriter
}

// ===== Stub implementation for do-nothing persistence log destination

type NoopDest struct {
}
