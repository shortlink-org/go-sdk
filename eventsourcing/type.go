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

var _ EventSourcing = (*es_postgres.EventStore)(nil)
