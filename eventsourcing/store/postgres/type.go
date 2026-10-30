package es_postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
	"github.com/shortlink-org/go-sdk/uow"
)

// Aggregates describes methods no other package can call or implement.
//
// Deprecated: EventStore no longer embeds it.
type Aggregates interface { //nolint:iface // public API, kept for compatibility
	addAggregate(ctx context.Context, event *eventsourcing.Event) error
	updateAggregate(ctx context.Context, event *eventsourcing.Event) error
}

// Events describes methods no other package can call or implement.
//
// Deprecated: EventStore no longer embeds it.
type Events interface { //nolint:iface // public API, kept for compatibility
	addEvent(ctx context.Context, event *eventsourcing.Event) error
}

// EventStore keeps events, aggregates and snapshots in PostgreSQL.
type EventStore struct {
	db   *pgxpool.Pool
	opts options

	eventsAppended semconv.EventsAppendedTotal
	conflicts      semconv.ConcurrencyConflictsTotal
}

// querier is what pgxpool.Pool and pgx.Tx have in common.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// conn returns the transaction carried by ctx, or the pool.
//
//nolint:ireturn // the caller must not care which of the two it got
func (e *EventStore) conn(ctx context.Context) querier {
	if tx := uow.FromContext(ctx); tx != nil {
		return tx
	}

	return e.db
}
