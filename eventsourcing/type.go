package eventsourcing

import (
	"context"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	es_postgres "github.com/shortlink-org/go-sdk/eventsourcing/store/postgres"
)

// ErrConcurrencyConflict is matched by the error Save returns when the aggregate
// moved past the version the events were built on.
var ErrConcurrencyConflict = es_postgres.ErrConcurrencyConflict

// EventSourcing - interface for event store
type EventSourcing interface {
	// Save appends the events atomically, in the transaction ctx carries (see
	// the uow package) or else in its own.
	Save(ctx context.Context, events []*eventsourcing.Event) error
	// SafeSave is the same as Save.
	//
	// Deprecated: use Save.
	SafeSave(ctx context.Context, events []*eventsourcing.Event) error
	// Load returns the latest snapshot and the events after it, in version order.
	Load(ctx context.Context, aggregateID string) (*eventsourcing.Snapshot, []*eventsourcing.Event, error)

	GetAggregateWithoutSnapshot(ctx context.Context) ([]*eventsourcing.BaseAggregate, error)
	SaveSnapshot(ctx context.Context, snapshot *eventsourcing.Snapshot) error
}

// Store is the stream API of an event store, what Repository works on.
// It is implemented by es_postgres.EventStore.
type Store interface {
	// Append writes the events as the next versions of the aggregate, and returns
	// an error matching ErrConcurrencyConflict unless it is at expectedVersion, 0
	// for an aggregate that does not exist yet.
	Append(ctx context.Context, aggregateID string, expectedVersion int32, events []*eventsourcing.Event) error
	// ReadStream returns the events of the aggregate after the given version.
	ReadStream(ctx context.Context, aggregateID string, afterVersion int32) ([]*eventsourcing.Event, error)
	// LoadSnapshot returns the snapshot of the aggregate, an empty one at version 0
	// when it has none.
	LoadSnapshot(ctx context.Context, aggregateID string) (*eventsourcing.Snapshot, error)
	SaveSnapshot(ctx context.Context, snapshot *eventsourcing.Snapshot) error
}

// GlobalReader reads the events of all aggregates, what a projection works on.
// It is implemented by es_postgres.EventStore.
type GlobalReader interface {
	// ReadAll returns up to limit events after the cursor. Start with an empty
	// cursor, and continue with the Cursor of the last event handled.
	ReadAll(ctx context.Context, cursor string, limit uint64) ([]*eventsourcing.Event, error)
}

var (
	_ EventSourcing = (*es_postgres.EventStore)(nil)
	_ Store         = (*es_postgres.EventStore)(nil)
	_ GlobalReader  = (*es_postgres.EventStore)(nil)
)
