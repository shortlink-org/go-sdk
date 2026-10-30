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

func (e *EventStore) addAggregate(ctx context.Context, conn querier, event *eventsourcing.Event) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "addAggregate")
	defer span.End()

	now := time.Now().UTC()

	statement, args, err := psql.Insert("aggregates").
		Columns("id", "type", "version", "created_at", "updated_at").
		Values(event.GetAggregateId(), event.GetAggregateType(), event.GetVersion(), now, now).
		ToSql()
	if err != nil {
		return recordError(span, err)
	}

	_, err = conn.Exec(ctx, statement, args...)
	if err != nil {
		return recordError(span, asConflict(err, event))
	}

	return nil
}

func (e *EventStore) updateAggregate(ctx context.Context, conn querier, event *eventsourcing.Event) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "updateAggregate")
	defer span.End()

	statement, args, err := psql.Update("aggregates").
		Set("version", event.GetVersion()).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{
			"version": event.GetVersion() - 1,
			"id":      event.GetAggregateId(),
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
			AggregateID: event.GetAggregateId(),
			Version:     event.GetVersion(),
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
