package es_postgres

import (
	"context"
	"errors"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
)

type Aggregates interface {
	addAggregate(ctx context.Context, event *eventsourcing.Event) error
	updateAggregate(ctx context.Context, event *eventsourcing.Event) error
}

func (e *EventStore) addAggregate(ctx context.Context, event *eventsourcing.Event) error {
	// start tracing
	_, span := otel.Tracer("aggregate").Start(ctx, "addAggregate")
	defer span.End()

	entities := psql.Insert("aggregates").
		Columns("id", "type", "version").
		Values(event.GetAggregateId(), event.GetAggregateType(), event.GetVersion())

	q, args, err := entities.ToSql()
	if err != nil {
		return err
	}

	row := e.db.QueryRow(ctx, q, args...)
	err = row.Scan()
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err.Error() != "" {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return err
	}

	return nil
}

func (e *EventStore) updateAggregate(ctx context.Context, event *eventsourcing.Event) error {
	// start tracing
	_, span := otel.Tracer("aggregate").Start(ctx, "updateAggregate")
	defer span.End()

	entities := psql.Update("aggregates").
		Set("version", event.GetVersion()).
		Set("updated_at", time.Now()).
		Where(squirrel.Eq{
			"version": event.GetVersion() - 1,
			"id":      event.GetAggregateId(),
		})

	q, args, err := entities.ToSql()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return err
	}

	row, err := e.db.Exec(ctx, q, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return err
	}

	if row.RowsAffected() != 1 {
		return &IncorrectUpdatedBillingError{
			Updated: row.RowsAffected(),
		}
	}

	return nil
}
