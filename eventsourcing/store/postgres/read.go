package es_postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
)

const (
	defaultReadAllLimit = 1000

	cursorBase = 10
	cursorBits = 64

	columnAggregateID = "aggregate_id"
	columnVersion     = "version"
)

//nolint:gochecknoglobals // the column list of scanEvents
var eventColumns = []string{
	columnAggregateID, "aggregate_type", "id", "type", "payload", columnVersion, "created_at",
	"schema_version", "correlation_id", "causation_id", "metadata", "transaction_id::text", "global_position",
}

// ReadStream returns the events of the aggregate after the given version, in
// version order.
func (e *EventStore) ReadStream(ctx context.Context, aggregateID string, afterVersion int32) ([]*eventsourcing.Event, error) {
	// start tracing
	ctx, span := tracer().Start(ctx, "ReadStream")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID))

	err := validateUUID(aggregateID, ErrInvalidAggregateID)
	if err != nil {
		return nil, recordError(span, err)
	}

	events, err := loadEvents(ctx, e.conn(ctx), aggregateID, afterVersion)
	if err != nil {
		return nil, recordError(span, err)
	}

	return events, nil
}

// ReadAll returns up to limit events of all aggregates after the cursor, 1000
// when limit is 0. Start with an empty cursor, and continue with the Cursor of
// the last event handled.
//
// An event becomes visible once every transaction older than its own has ended.
// That is what keeps a slow commit from landing behind the cursor, so one
// long-running transaction anywhere in the database delays the stream.
func (e *EventStore) ReadAll(ctx context.Context, cursor string, limit uint64) ([]*eventsourcing.Event, error) {
	// start tracing
	ctx, span := tracer().Start(ctx, "ReadAll")
	defer span.End()

	transactionID, position, err := parseCursor(cursor)
	if err != nil {
		return nil, recordError(span, err)
	}

	if limit == 0 {
		limit = defaultReadAllLimit
	}

	statement, args, err := psql.Select(eventColumns...).
		From("events").
		Where("transaction_id < pg_snapshot_xmin(pg_current_snapshot())").
		Where("(transaction_id, global_position) > (?::text::xid8, ?::bigint)", transactionID, position).
		OrderBy("transaction_id", "global_position").
		Limit(limit).
		ToSql()
	if err != nil {
		return nil, recordError(span, err)
	}

	rows, err := e.db.Query(ctx, statement, args...)
	if err != nil {
		return nil, recordError(span, err)
	}

	events, err := scanEvents(rows)
	if err != nil {
		return nil, recordError(span, err)
	}

	return events, nil
}

// LoadSnapshot returns the snapshot of the aggregate. An aggregate without one
// yields an empty snapshot, at version 0.
func (e *EventStore) LoadSnapshot(ctx context.Context, aggregateID string) (*eventsourcing.Snapshot, error) {
	// start tracing
	ctx, span := tracer().Start(ctx, "LoadSnapshot")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID))

	err := validateUUID(aggregateID, ErrInvalidAggregateID)
	if err != nil {
		return nil, recordError(span, err)
	}

	snapshot, err := loadSnapshot(ctx, e.conn(ctx), aggregateID)
	if err != nil {
		return nil, recordError(span, err)
	}

	return snapshot, nil
}

func loadSnapshot(ctx context.Context, conn querier, aggregateID string) (*eventsourcing.Snapshot, error) {
	statement, args, err := psql.Select(columnAggregateID, "aggregate_type", "aggregate_version", "payload", "created_at", "updated_at", "schema_version").
		From("snapshots").
		Where(squirrel.Eq{columnAggregateID: aggregateID}).
		ToSql()
	if err != nil {
		return nil, err
	}

	var (
		snapshot             eventsourcing.Snapshot
		createdAt, updatedAt time.Time
	)

	err = conn.QueryRow(ctx, statement, args...).Scan(
		&snapshot.AggregateId, &snapshot.AggregateType, &snapshot.AggregateVersion, &snapshot.Payload,
		&createdAt, &updatedAt, &snapshot.SchemaVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return &snapshot, nil
	}

	if err != nil {
		return nil, err
	}

	snapshot.CreatedAt = timestamppb.New(createdAt)
	snapshot.UpdatedAt = timestamppb.New(updatedAt)

	return &snapshot, nil
}

func loadEvents(ctx context.Context, conn querier, aggregateID string, afterVersion int32) ([]*eventsourcing.Event, error) {
	statement, args, err := psql.Select(eventColumns...).
		From("events").
		Where(squirrel.And{
			squirrel.Eq{columnAggregateID: aggregateID},
			squirrel.Gt{columnVersion: afterVersion},
		}).
		OrderBy(columnVersion).
		ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}

	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]*eventsourcing.Event, error) {
	defer rows.Close()

	var events []*eventsourcing.Event //nolint:prealloc // false positive

	for rows.Next() {
		var (
			event         eventsourcing.Event
			createdAt     time.Time
			transactionID string
			position      int64
		)

		err := rows.Scan(
			&event.AggregateId, &event.AggregateType, &event.Id, &event.Type, &event.Payload, &event.Version, &createdAt,
			&event.SchemaVersion, &event.CorrelationId, &event.CausationId, &event.Metadata, &transactionID, &position,
		)
		if err != nil {
			return nil, err
		}

		event.CreatedAt = timestamppb.New(createdAt)
		event.Cursor = transactionID + "-" + strconv.FormatInt(position, cursorBase)
		events = append(events, &event)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return events, nil
}

// parseCursor splits a cursor into the transaction id, as the decimal text
// PostgreSQL casts to xid8, and the global position.
func parseCursor(cursor string) (string, int64, error) {
	if cursor == "" {
		return "0", 0, nil
	}

	transactionID, rawPosition, found := strings.Cut(cursor, "-")
	if !found {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidCursor, cursor)
	}

	_, err := strconv.ParseUint(transactionID, cursorBase, cursorBits)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidCursor, cursor)
	}

	position, err := strconv.ParseInt(rawPosition, cursorBase, cursorBits)
	if err != nil || position < 0 {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidCursor, cursor)
	}

	return transactionID, position, nil
}
