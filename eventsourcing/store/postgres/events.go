package es_postgres

import (
	"context"
	"slices"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
)

const (
	// One INSERT stays well below the limit of 65535 parameters.
	insertBatchSize = 1000

	defaultSchemaVersion = 1

	traceparentKey = "traceparent"
)

// addEvents writes the events, filling in what they do not set: Id, CreatedAt,
// SchemaVersion and the trace context in Metadata.
func (e *EventStore) addEvents(ctx context.Context, conn querier, events []*eventsourcing.Event) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "addEvents")
	defer span.End()

	for batch := range slices.Chunk(events, insertBatchSize) {
		query := psql.Insert("events").Columns(
			columnAggregateID, "aggregate_type", "id", "type", "payload", columnVersion, "created_at",
			"schema_version", "correlation_id", "causation_id", "metadata",
		)

		for _, event := range batch {
			fillEvent(ctx, event)
			span.AddEvent("event", trace.WithAttributes(semconv.EventID(event.GetId())))

			query = query.Values(
				event.GetAggregateId(), event.GetAggregateType(), event.GetId(), event.GetType(),
				event.GetPayload(), event.GetVersion(), event.GetCreatedAt().AsTime(),
				event.GetSchemaVersion(), event.GetCorrelationId(), event.GetCausationId(), event.GetMetadata(),
			)
		}

		statement, args, err := query.ToSql()
		if err != nil {
			return recordError(span, err)
		}

		_, err = conn.Exec(ctx, statement, args...)
		if err != nil {
			return recordError(span, asConflict(err, batch[0]))
		}
	}

	return nil
}

func fillEvent(ctx context.Context, event *eventsourcing.Event) {
	if event.GetId() == "" {
		event.Id = uuid.NewV7().String()
	}

	if event.GetCreatedAt() == nil {
		event.CreatedAt = timestamppb.New(time.Now())
	}

	if event.GetSchemaVersion() == 0 {
		event.SchemaVersion = defaultSchemaVersion
	}

	if event.GetMetadata() == nil {
		event.Metadata = map[string]string{}
	}

	if _, ok := event.GetMetadata()[traceparentKey]; !ok {
		propagation.TraceContext{}.Inject(ctx, propagation.MapCarrier(event.GetMetadata()))
	}
}
