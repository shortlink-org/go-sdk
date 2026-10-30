package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	"uuid"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/eventsourcing"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
	"github.com/shortlink-org/go-sdk/uow"
)

const (
	// One INSERT stays well below the limit of 65535 parameters.
	insertBatchSize = 1000

	defaultSchemaVersion = 1

	traceparentKey = "traceparent"

	constraintEventsPrimaryKey = "events_pkey"
)

// Append writes the events as the versions after expectedVersion, 0 for an aggregate
// that does not exist yet, with one write of the aggregate and batched INSERTs. It
// works in the transaction ctx carries, see the uow package, or else in its own.
//
// It sets AggregateID and Version of the events, and fills in ID, CreatedAt,
// SchemaVersion and the trace context in Metadata when they are not set.
func (s *Store) Append(ctx context.Context, aggregateID string, expectedVersion int64, events []*eventsourcing.Event) error {
	ctx, span := tracer().Start(ctx, "Append")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID))

	if len(events) == 0 {
		return nil
	}

	err := prepareEvents(aggregateID, expectedVersion, events)
	if err != nil {
		return recordError(span, err)
	}

	aggregateType := events[0].AggregateType
	span.SetAttributes(semconv.AggregateType(aggregateType))

	write := func(conn querier) error {
		return s.write(ctx, conn, expectedVersion, events)
	}

	if tx := uow.FromContext(ctx); tx != nil {
		err = write(tx)
	} else {
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			return write(tx)
		})
	}

	if errors.Is(err, eventsourcing.ErrConcurrencyConflict) {
		s.conflicts.Add(ctx, 1, aggregateType)
	}

	if err != nil {
		return recordError(span, err)
	}

	s.eventsAppended.Add(ctx, int64(len(events)), aggregateType)

	return nil
}

func prepareEvents(aggregateID string, expectedVersion int64, events []*eventsourcing.Event) error {
	err := validateUUID(aggregateID, eventsourcing.ErrInvalidAggregateID)
	if err != nil {
		return err
	}

	if expectedVersion < 0 {
		return fmt.Errorf("%w: expected version %d", eventsourcing.ErrInvalidEvent, expectedVersion)
	}

	version := expectedVersion

	for i, event := range events {
		version++

		err = validateEvent(i, event, aggregateID, version)
		if err != nil {
			return err
		}

		event.AggregateID = aggregateID
		event.Version = version
	}

	return nil
}

func validateEvent(index int, event *eventsourcing.Event, aggregateID string, version int64) error {
	if event == nil {
		return fmt.Errorf("%w: event %d is nil", eventsourcing.ErrInvalidEvent, index)
	}

	if event.AggregateID != "" && event.AggregateID != aggregateID {
		return fmt.Errorf("%w: event %d belongs to aggregate %s", eventsourcing.ErrInvalidEvent, index, event.AggregateID)
	}

	if event.Version != 0 && event.Version != version {
		return fmt.Errorf("%w: event %d has version %d, expected %d", eventsourcing.ErrInvalidEvent, index, event.Version, version)
	}

	if event.AggregateType == "" || event.Type == "" {
		return fmt.Errorf("%w: event %d has no aggregate type or no type", eventsourcing.ErrInvalidEvent, index)
	}

	if !json.Valid(event.Payload) {
		return fmt.Errorf("%w: event %d has a payload that is not JSON", eventsourcing.ErrInvalidEvent, index)
	}

	if event.ID != "" {
		return validateUUID(event.ID, eventsourcing.ErrInvalidEventID)
	}

	return nil
}

func (s *Store) write(ctx context.Context, conn querier, expectedVersion int64, events []*eventsourcing.Event) error {
	first, last := events[0], events[len(events)-1]

	var err error
	if expectedVersion == 0 {
		err = s.createAggregate(ctx, conn, first, last.Version)
	} else {
		err = s.advanceAggregate(ctx, conn, first, expectedVersion, last.Version)
	}

	if err != nil {
		return err
	}

	return s.insertEvents(ctx, conn, events)
}

func (s *Store) createAggregate(ctx context.Context, conn querier, first *eventsourcing.Event, version int64) error {
	ctx, span := tracer().Start(ctx, "createAggregate")
	defer span.End()

	statement, args, err := psql.Insert(tableAggregates).
		Columns("id", "type", columnVersion).
		Values(first.AggregateID, first.AggregateType, version).
		ToSql()
	if err != nil {
		return recordError(span, err)
	}

	_, err = conn.Exec(ctx, statement, args...)
	if err != nil {
		return recordError(span, classifyWriteError(err, first))
	}

	return nil
}

// advanceAggregate moves the aggregate from expectedVersion to version. The row lock
// it takes serializes the writers of one aggregate.
func (s *Store) advanceAggregate(ctx context.Context, conn querier, first *eventsourcing.Event, expectedVersion, version int64) error {
	ctx, span := tracer().Start(ctx, "advanceAggregate")
	defer span.End()

	statement, args, err := psql.Update(tableAggregates).
		Set(columnVersion, version).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{columnVersion: expectedVersion, "id": first.AggregateID}).
		ToSql()
	if err != nil {
		return recordError(span, err)
	}

	tag, err := conn.Exec(ctx, statement, args...)
	if err != nil {
		return recordError(span, err)
	}

	if tag.RowsAffected() != 1 {
		return recordError(span, fmt.Errorf("%w: aggregate %s is not at version %d",
			eventsourcing.ErrConcurrencyConflict, first.AggregateID, expectedVersion))
	}

	return nil
}

func (s *Store) insertEvents(ctx context.Context, conn querier, events []*eventsourcing.Event) error {
	ctx, span := tracer().Start(ctx, "insertEvents")
	defer span.End()

	for batch := range slices.Chunk(events, insertBatchSize) {
		query := psql.Insert(tableEvents).Columns(
			"id", columnAggregateID, columnAggregateType, columnVersion, "type", columnSchemaVersion,
			"payload", "correlation_id", "causation_id", "metadata", "created_at",
		)

		for _, event := range batch {
			fillEvent(ctx, event)
			span.AddEvent("event", trace.WithAttributes(semconv.EventID(event.ID)))

			query = query.Values(
				event.ID, event.AggregateID, event.AggregateType, event.Version, event.Type, event.SchemaVersion,
				event.Payload, event.CorrelationID, event.CausationID, event.Metadata, event.CreatedAt,
			)
		}

		statement, args, err := query.ToSql()
		if err != nil {
			return recordError(span, err)
		}

		_, err = conn.Exec(ctx, statement, args...)
		if err != nil {
			return recordError(span, classifyWriteError(err, batch[0]))
		}
	}

	return nil
}

func fillEvent(ctx context.Context, event *eventsourcing.Event) {
	if event.ID == "" {
		event.ID = uuid.NewV7().String()
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	if event.SchemaVersion == 0 {
		event.SchemaVersion = defaultSchemaVersion
	}

	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}

	if _, ok := event.Metadata[traceparentKey]; !ok {
		propagation.TraceContext{}.Inject(ctx, propagation.MapCarrier(event.Metadata))
	}
}

// classifyWriteError names a unique violation: an event id already in use, or else
// two writers racing for the same version of the aggregate.
func classifyWriteError(err error, event *eventsourcing.Event) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.UniqueViolation {
		return err
	}

	if pgErr.ConstraintName == constraintEventsPrimaryKey {
		return fmt.Errorf("%w: %w", eventsourcing.ErrDuplicateEventID, err)
	}

	return fmt.Errorf("%w: aggregate %s at version %d: %w", eventsourcing.ErrConcurrencyConflict, event.AggregateID, event.Version, err)
}
