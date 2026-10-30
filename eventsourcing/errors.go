package eventsourcing

import "errors"

var (
	// ErrConcurrencyConflict means the aggregate is not at the expected version:
	// another writer got there first. Load the aggregate and retry the command.
	ErrConcurrencyConflict = errors.New("eventsourcing: concurrency conflict")

	// ErrInvalidEvent means an event cannot be stored as it is.
	ErrInvalidEvent = errors.New("eventsourcing: invalid event")

	// ErrInvalidAggregateID means an aggregate id is not a UUID.
	ErrInvalidAggregateID = errors.New("eventsourcing: aggregate id is not a UUID")

	// ErrInvalidEventID means an event id is set and is not a UUID.
	ErrInvalidEventID = errors.New("eventsourcing: event id is not a UUID")

	// ErrDuplicateEventID means an event with the same id is already stored.
	ErrDuplicateEventID = errors.New("eventsourcing: duplicate event id")

	// ErrInvalidCursor means a cursor was not taken from an event read by ReadAll.
	ErrInvalidCursor = errors.New("eventsourcing: invalid cursor")

	// ErrSnapshotNotFound means the aggregate has no snapshot.
	ErrSnapshotNotFound = errors.New("eventsourcing: snapshot not found")

	// ErrAggregateNotFound means the aggregate has no snapshot and no events.
	ErrAggregateNotFound = errors.New("eventsourcing: aggregate not found")

	// ErrAggregateTypeMismatch means the stored aggregate is of another type than
	// the repository handles.
	ErrAggregateTypeMismatch = errors.New("eventsourcing: aggregate type mismatch")

	// ErrEventOutOfOrder means a replayed event is not newer than the aggregate.
	ErrEventOutOfOrder = errors.New("eventsourcing: event is out of order")

	// ErrNotSnapshotter means the aggregate does not implement Snapshotter.
	ErrNotSnapshotter = errors.New("eventsourcing: aggregate does not implement Snapshotter")

	// ErrNilAggregate is returned by Raise without an aggregate.
	ErrNilAggregate = errors.New("eventsourcing: nil aggregate")

	// ErrNilStore is returned by NewRepository without a store.
	ErrNilStore = errors.New("eventsourcing: repository requires a store")

	// ErrNilFactory is returned by NewRepository without a factory.
	ErrNilFactory = errors.New("eventsourcing: repository requires a factory")

	// ErrEmptyAggregateType is returned by NewRepository without an aggregate type.
	ErrEmptyAggregateType = errors.New("eventsourcing: repository requires an aggregate type")
)
