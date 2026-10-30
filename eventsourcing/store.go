package eventsourcing

import "context"

// Store is the stream API of an event store, what Repository works on.
type Store interface {
	// Append writes the events atomically as the versions after expectedVersion, 0
	// for an aggregate that does not exist yet. It returns an error matching
	// ErrConcurrencyConflict when the aggregate is at another version.
	Append(ctx context.Context, aggregateID string, expectedVersion int64, events []*Event) error
	// ReadStream returns the events of the aggregate after the given version, in
	// version order.
	ReadStream(ctx context.Context, aggregateID string, afterVersion int64) ([]*Event, error)
	// LoadSnapshot returns the snapshot of the aggregate, or an error matching
	// ErrSnapshotNotFound.
	LoadSnapshot(ctx context.Context, aggregateID string) (*Snapshot, error)
	// SaveSnapshot saves the snapshot unless a newer one is stored: one of a later
	// schema version, or of a later version in the same schema version.
	SaveSnapshot(ctx context.Context, snapshot *Snapshot) error
}

// GlobalReader reads the events of all aggregates, what a projection works on.
type GlobalReader interface { //nolint:iface // a port, implemented in store/postgres
	// ReadAll returns up to limit events after the cursor, in a stable order. Start
	// with an empty cursor, and continue with the Cursor of the last event handled.
	ReadAll(ctx context.Context, cursor string, limit uint64) ([]*Event, error)
}

// SnapshotScheduler finds the aggregates a background job should snapshot.
type SnapshotScheduler interface { //nolint:iface // a port, implemented in store/postgres
	// AggregatesDueForSnapshot returns up to limit aggregates that gained at least
	// minEvents events since their last snapshot, least recently changed first. An
	// aggregate leaves the result once its snapshot is saved.
	AggregatesDueForSnapshot(ctx context.Context, minEvents int64, limit uint64) ([]AggregateRef, error)
}
