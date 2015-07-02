Persistence Log
===============
[![Build Status](https://magnum.travis-ci.com/rightscale/persist.svg?token=4Q13wQTY4zqXgU7Edw3B&branch=master)](https://magnum.travis-ci.com/rightscale/persist)
![Code Coverage](https://s3.amazonaws.com/rs-code-coverage/persist/cc_badge_master.svg)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/rightscale/persist/blob/master/LICENSE)
[![Godoc](https://godoc.org/github.com/rightscale/persist?status.svg)](http://godoc.org/github.com/rightscale/persist)

This Golang package implements a persistence log very similar to a database replay log
or write-ahead-log (WAL): before committing a change to a resource (arbitrary data structure)
the app writes the new state of the resource to a log. If the app crashes, the log can be replayed
in order to recreate all the resources. The trick is to rotate the log periodically
so it doesn't grow indefinitely. This is what this persistence package implements.

One of the special features of this persistence log is that it does not define the set of
operations that can be persisted to the log nor does it require storage beyond the typical
streaming encoder/decoder buffers. In particular, it does not create a copy of the log or
of the in-memory data structure in order to start a new log or create a snapshot. Instead
it makes a callback into the app to enumerate the set of live objects.

Goals
-----
- persist changes that are committed to in-memory data structures/databases to a log
- replay all the chanages in order to re-create the in-memory data structure
- start a fresh stream periodically in order not to grow the log indefinitely
- use the log in order to keep a replica server up-to-date
- not be tied to disk and instead allow the log to go to a remote server/service

Some things this persistence layer does not do:
- multi-resource updates (transactions) where multiple resources are written atomically
- attempt to guarantee durability in the sense that once the log write completes the data
  is guaranteed to be on stable storage, such as disk

Model of operation
------------------

The way the persistence log functions is as follows. After opening the log, the application
calls Update() for every change and passes it an opaque desrciption of the change.
The Update function serializes the change and appends it to the log.

When the time to rotate the log comes, the persistence layer stops writing to the
current log and starts a fresh log. The fresh log begins with a mixture of additional
updates and an enumeration of all current resources.
To generate this enumeration, the persistence layer makes a callback into the application
which must traverse all live resources and call Update() with a "create" descriptor.

When/if the application asks the persistence layer to replay a log, the latter locates
the last log for which the initialization callback has completed, sends all updates in
that log to the application, and then opens any new incomplete log and replays the updates
in that log as well.

One critical question is how to handle concurrency. The persistence layer is designed to
allow concurrency while keeping the model as simple as possible. The primary requirement
on the application is as follows:
 - When an application calls Update() for a resource it must guarantee that no concurrent
   call to Update() can happen for the same resource. This guarantees strict ordering on
   updates to a resource, i.e., it prevents updates from getting out of order. The mutual
   exclusion guarantee made by the application must apply to the initialization callback
   during which the application enumerates all resources as well.

Typically an application acquires a lock on a resource before mutating it. This makes it
easy to satisfy this requirement by calling Update() while holding the lock on the resource.

An additional requirement arises if an application makes resource mutations and calls
Update() concurrently with enumerating all resources in a log initialization callback.
The persistence layer writes the updates to the new log as they come in from the application
which means that on a replay is is likely that the application will receive an update
to a resource before it has received the initialization descriptor written in the
init callback for that resource. There are several ways to deal with this issue:
 - the application can acquire a global lock for the duration of the initialization
   callback, thereby preventing concurrent updates
 - during replay, the application can ignore updates to resources that have not yet been
   created knowing that eventually it will encounter a full create descriptor as written
   during the initialization callback
 - if the application creates log entries that involve multiple resources, for example,
   a log entry to increment resource A and decrement resource B, then during a replay
   it must cope with the situation where one of the resources has been created and the
   other one hasn't. If this example, It may be that A has been created by a prior replayed
   log entry and the app has to increment A, but B may not yet have been created and thus
   the decrement B has to be skipped without producing an error (the correct value of B will
   be created later during the replay).

Note that the persistence layer does not prescribe what the application passes it in an
Update() call. One option is to pass a full copy of the resource being updated or a delete
marker. In this case replaying the log consists of re-creating or updating each resource as
it is fed back to the application and deleting existing resources when encountering a
delete marker.

Concepts
--------

### Resources

A resource is an object that can be serialized to the log and that is
the unit of atomic update in the application.

### Logs

A log is the object to which changes to resources are persisted, and it has
one or multiple destinations to which data is written. Each log has a primary destination,
which is the destination from which a restore can be initiated.
A log has the following operations:
- create: creates a log object and names a primary destination
- restore: reads the last log at the destination and replays all events, making
  callbacks into the application in order to recreate the state
- update: records a change to a resource, i.e., writes the serialized version to the log
- addDestination: adds a secondary destination, this will cause a log rotation

Sample code
-----------

```go
// A sample resource type
type resourceType struct {
	Id    int64  // unique resource ID
	Field string // sample data field
}
// The set of resources in the application
var resources map[int64]resourceType  // all resources indexed by ID
var resourcesMutex mutex.Lock         // exclusive access to the resources map
var pLog persistence.Log              // persistence log for the resources
const (
	ResourceUpsert = iota // insert or update
	ResourceDelete
)
type ResourceLogEntry struct {
	Op  int          // ResourceUpsert or ResourceDelete
	Res resourceType
}

// Create function used within the application to create a new resource. It assumes
// that a unique Id has already been generated.
func createResource(data resourceType) error {
	// lock all resources
	resourcesMutex.Lock()
	defer resourcesMutex.Unlock()
	// check non-existence
	if _, ok := resources[data.Id]; ok {
		return fmt.Errorf("duplicate resource ID")
	}
	// write to log
	pLog.Update(ResourceLogEntry{Op: ResourceUpsert, Res: data})
	// insert into map
	resources[data.Id] = data
}

// Delete function used within the application to delete an existing resource.
func deleteResource(id int64) {
	// lock all resources
	resourcesMutex.Lock()
	defer resourcesMutex.Unlock()
	// check existence
	if _, ok := resources[id]; !ok {
		return fmt.Errorf("deleting non-existing resource")
	}
	// write to log
	pLog.Update(ResourceLogEntry{Op: ResourceDelete, Res: ResourceTpe{Id: id}})
	// delete from map
	delete(resources, id)
}

// Update function used within the application to update an existing resource
func updateResource(data resourceType) error {
	// lock all resources
	resourcesMutex.Lock()
	defer resourcesMutex.Unlock()
	// check existence
	if _, ok := resources[id]; !ok {
		return fmt.Errorf("updating non-existing resource")
	}
	// write to log
	pLog.Update(ResourceLogEntry{Op: ResourceUpsert, Res: data})
	// update map
	resources[data.Id] = data
}

// Callback from persistence log to enumerate all resources in order to start a fresh log
func enumerateResources() {
	// lock all resources
	resourcesMutex.Lock()
	defer resourcesMutex.Unlock()
	// iterate through the entire map
	for _, v := range resources {
		pLog.Update(v)
	}
}

// Callback from persistence log to replay a resource operation
func replayResource(logEntry interface{}) error {
	// lock all resources
	resourcesMutex.Lock()
	defer resourcesMutex.Unlock()
	// type cast log operation
	op, ok := logEntry.(ResourceLogEntry)
	if !ok {
		return fmt.Errorf("invalid replay record type")
	}
	// perform operation
	switch op.Op {
	case ResourceUpsert:
		resources[op.Res.Id] = op.Res
	case ResourceDelete:
		delete(resources, op.Res.Id)
	}
}
```
