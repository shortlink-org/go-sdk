package eventsourcing

import (
	"encoding/json"
	"fmt"
)

// Aggregate is a consistency boundary rebuilt from its events. Implement it with a
// pointer to a type that embeds Root and defines Apply.
type Aggregate interface {
	// Apply changes the state by the event. It runs both when an event is raised
	// and when it is replayed, so it must not have side effects.
	Apply(event *Event) error

	root() *Root
}

// Snapshotter is implemented by an aggregate that can be restored from a snapshot.
// Without it, Repository.Load replays all the events.
type Snapshotter interface {
	// SnapshotSchemaVersion is the version of the MarshalSnapshot format. Bump it
	// when the format changes: snapshots of another version are ignored and the
	// events are replayed.
	SnapshotSchemaVersion() int
	// MarshalSnapshot returns the state of the aggregate as JSON.
	MarshalSnapshot() (json.RawMessage, error)
	// RestoreSnapshot sets the state of the aggregate from MarshalSnapshot's output.
	RestoreSnapshot(payload json.RawMessage) error
}

// Root holds what every aggregate has: an identity, a version and the events not
// saved yet. Embed it in an aggregate.
type Root struct {
	id            string
	aggregateType string
	version       int64
	uncommitted   []*Event
}

// ID returns the id of the aggregate.
func (r *Root) ID() string {
	return r.id
}

// Type returns the type of the aggregate.
func (r *Root) Type() string {
	return r.aggregateType
}

// Version returns the version of the aggregate, counting the events not saved yet.
func (r *Root) Version() int64 {
	return r.version
}

// Uncommitted returns the events not saved yet.
func (r *Root) Uncommitted() []*Event {
	return r.uncommitted
}

func (r *Root) root() *Root { //nolint:unused // promoted to the aggregates embedding Root, see Aggregate
	return r
}

// Raise applies a new event to the aggregate and records it for Repository.Save.
// When Apply fails, the aggregate takes no new version and records nothing.
func Raise(aggregate Aggregate, event *Event) error {
	if aggregate == nil {
		return ErrNilAggregate
	}

	if event == nil {
		return fmt.Errorf("%w: nil event", ErrInvalidEvent)
	}

	root := aggregate.root()

	event.AggregateID = root.id
	event.AggregateType = root.aggregateType
	event.Version = root.version + 1

	err := aggregate.Apply(event)
	if err != nil {
		return err
	}

	root.version = event.Version
	root.uncommitted = append(root.uncommitted, event)

	return nil
}

// replay applies a stored event and moves the aggregate to its version.
func replay(aggregate Aggregate, event *Event) error {
	root := aggregate.root()

	if event.Version <= root.version {
		return fmt.Errorf("%w: event version %d, aggregate version %d", ErrEventOutOfOrder, event.Version, root.version)
	}

	err := aggregate.Apply(event)
	if err != nil {
		return err
	}

	root.version = event.Version

	return nil
}
