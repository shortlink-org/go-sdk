package es_postgres

import (
	"context"
	"time"
	"uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
)

// addEvent writes the event, filling in Id and CreatedAt when they are not set.
func (e *EventStore) addEvent(ctx context.Context, conn querier, event *eventsourcing.Event) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "addEvent")
	defer span.End()

	if event.GetId() == "" {
		event.Id = uuid.NewV7().String()
	}

	if event.GetCreatedAt() == nil {
		event.CreatedAt = timestamppb.New(time.Now())
	}

	span.SetAttributes(semconv.EventID(event.GetId()))

	statement, args, err := psql.Insert("events").
		Columns("aggregate_id", "aggregate_type", "id", "type", "payload", "version", "created_at").
		Values(
			event.GetAggregateId(), event.GetAggregateType(), event.GetId(), event.GetType(),
			event.GetPayload(), event.GetVersion(), event.GetCreatedAt().AsTime(),
		).
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
