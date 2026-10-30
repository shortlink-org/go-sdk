package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/shortlink-org/go-sdk/eventsourcing"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
)

const (
	defaultReadAllLimit = 1000

	cursorSeparator = "-"
	cursorBase      = 10
	cursorBits      = 64
)

// ReadStream returns the events of the aggregate after the given version, in version
// order. It works in the transaction ctx carries, see the uow package.
func (s *Store) ReadStream(ctx context.Context, aggregateID string, afterVersion int64) ([]*eventsourcing.Event, error) {
	ctx, span := tracer().Start(ctx, "ReadStream")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID))

	err := validateUUID(aggregateID, eventsourcing.ErrInvalidAggregateID)
	if err != nil {
		return nil, recordError(span, err)
	}

	statement, args, err := selectEvents().
		Where(squirrel.And{
			squirrel.Eq{columnAggregateID: aggregateID},
			squirrel.Gt{columnVersion: afterVersion},
		}).
		OrderBy(columnVersion).
		ToSql()
	if err != nil {
		return nil, recordError(span, err)
	}

	rows, err := s.conn(ctx).Query(ctx, statement, args...)
	if err != nil {
		return nil, recordError(span, err)
	}

	events, err := scanEvents(rows)
	if err != nil {
		return nil, recordError(span, err)
	}

	return events, nil
}

// ReadAll returns up to limit events of all aggregates after the cursor, 1000 when
// limit is 0. Start with an empty cursor, and continue with the Cursor of the last
// event handled.
//
// An event becomes visible once every transaction older than its own has ended. That
// is what keeps a slow commit from landing behind the cursor, so one long-running
// transaction anywhere in the database delays the stream.
func (s *Store) ReadAll(ctx context.Context, cursor string, limit uint64) ([]*eventsourcing.Event, error) {
	ctx, span := tracer().Start(ctx, "ReadAll")
	defer span.End()

	transactionID, position, err := parseCursor(cursor)
	if err != nil {
		return nil, recordError(span, err)
	}

	if limit == 0 {
		limit = defaultReadAllLimit
	}

	statement, args, err := selectEvents().
		Where("transaction_id < pg_snapshot_xmin(pg_current_snapshot())").
		Where("(transaction_id, global_position) > (?::text::xid8, ?::bigint)", transactionID, position).
		OrderBy("transaction_id", "global_position").
		Limit(limit).
		ToSql()
	if err != nil {
		return nil, recordError(span, err)
	}

	rows, err := s.db.Query(ctx, statement, args...)
	if err != nil {
		return nil, recordError(span, err)
	}

	events, err := scanEvents(rows)
	if err != nil {
		return nil, recordError(span, err)
	}

	return events, nil
}

func selectEvents() squirrel.SelectBuilder {
	return psql.Select(
		"id", columnAggregateID, columnAggregateType, columnVersion, "type", columnSchemaVersion,
		"payload", "correlation_id", "causation_id", "metadata", "created_at", "transaction_id::text", "global_position",
	).From(tableEvents)
}

func scanEvents(rows pgx.Rows) ([]*eventsourcing.Event, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*eventsourcing.Event, error) {
		var (
			event         eventsourcing.Event
			transactionID string
			position      int64
		)

		err := row.Scan(
			&event.ID, &event.AggregateID, &event.AggregateType, &event.Version, &event.Type, &event.SchemaVersion,
			&event.Payload, &event.CorrelationID, &event.CausationID, &event.Metadata, &event.CreatedAt, &transactionID, &position,
		)
		if err != nil {
			return nil, err
		}

		event.Cursor = transactionID + cursorSeparator + strconv.FormatInt(position, cursorBase)

		return &event, nil
	})
}

// parseCursor splits a cursor into the transaction id, as the decimal text PostgreSQL
// casts to xid8, and the global position.
func parseCursor(cursor string) (string, int64, error) {
	if cursor == "" {
		return "0", 0, nil
	}

	transactionID, rawPosition, found := strings.Cut(cursor, cursorSeparator)
	if !found {
		return "", 0, fmt.Errorf("%w: %q", eventsourcing.ErrInvalidCursor, cursor)
	}

	_, err := strconv.ParseUint(transactionID, cursorBase, cursorBits)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %q", eventsourcing.ErrInvalidCursor, cursor)
	}

	position, err := strconv.ParseInt(rawPosition, cursorBase, cursorBits)
	if err != nil || position < 0 {
		return "", 0, fmt.Errorf("%w: %q", eventsourcing.ErrInvalidCursor, cursor)
	}

	return transactionID, position, nil
}
