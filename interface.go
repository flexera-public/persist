// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

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

	// Enumerate is called by the persistence layer in order to enumerate all live resources
	// by making calls to Log.Write(). (If enumerate encounters an erorr it's time to panic.)
	// Enumerate can run in parallel with new updates to resources however the application
	// must ensure that calls to Log.Write() are in the same order as Enumerate's reads
	// and other update's writes.
	Enumerate()
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
	SetSizeLimit(bytes int64)

	// AddDestination adds additional destinations to the Log (not yet implemented)
	AddDestination(dest LogDestination) error

	// HealthCheck returns any persistent error encountered in persist that prevents it
	// from logging. If HealthCheck() returns an error then all Write() calls will return
	// the same error. If the problem is fixed the error will eventually go away again and
	// the log will be "repaired" by doing a rotation. The intent of the HealthCheck call
	// is for the application to be able to reject requests early if the logging is broken.
	HealthCheck() error
}

// CreateLog creates a new log and errors if a pre-existing log is found.
func CreateLog(dest LogDestination, client LogClient) (Log, error) {
}

// OpenLog reopens an existing log, replays all log entries, and then prepares to append
// to it. The call to OpenLog completes once any necessary replay has completed. The
// okToCreate flag indicates whether it's ok to start a completely new log (or whether an
// error shold be produced if no pre-existing log is found).
func OpenLog(dest LogDestination, client LogClient, okToCreate bool) (Log, error) {
}

type LogDestination interface {
}
