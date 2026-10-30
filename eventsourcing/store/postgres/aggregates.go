package es_postgres

import (
	"context"
	"errors"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
)

// addAggregate creates the aggregate of the first event, at the given version.
func (e *EventStore) addAggregate(ctx context.Context, conn querier, first *eventsourcing.Event, version int32) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "addAggregate")
	defer span.End()

	now := time.Now().UTC()

	statement, args, err := psql.Insert("aggregates").
		Columns("id", "type", columnVersion, "created_at", "updated_at").
		Values(first.GetAggregateId(), first.GetAggregateType(), version, now, now).
		ToSql()
	if err != nil {
		return recordError(span, err)
	}

	_, err = conn.Exec(ctx, statement, args...)
	if err != nil {
		return recordError(span, asConflict(err, first))
	}

	return nil
}

// updateAggregate moves the aggregate from the version before first to the given
// version. The row lock it takes serializes the writers of one aggregate.
func (e *EventStore) updateAggregate(ctx context.Context, conn querier, first *eventsourcing.Event, version int32) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "updateAggregate")
	defer span.End()

	statement, args, err := psql.Update("aggregates").
		Set(columnVersion, version).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{
			columnVersion: first.GetVersion() - 1,
			"id":          first.GetAggregateId(),
		}).
		ToSql()
	if err != nil {
		return recordError(span, err)
	}

	row, err := conn.Exec(ctx, statement, args...)
	if err != nil {
		return recordError(span, err)
	}

	if row.RowsAffected() != 1 {
		return recordError(span, &ConcurrencyConflictError{
			AggregateID: first.GetAggregateId(),
			Version:     first.GetVersion(),
			Updated:     row.RowsAffected(),
		})
	}

	return nil
}

// asConflict turns a unique violation into a ConcurrencyConflictError: two
// writers raced for the same version of the aggregate.
func asConflict(err error, event *eventsourcing.Event) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return &ConcurrencyConflictError{
			Err:         err,
			AggregateID: event.GetAggregateId(),
			Version:     event.GetVersion(),
		}
	}

	return err
}
