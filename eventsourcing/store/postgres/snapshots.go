package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/shortlink-org/go-sdk/eventsourcing"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
)

// LoadSnapshot returns the snapshot of the aggregate, or an error matching
// eventsourcing.ErrSnapshotNotFound.
func (s *Store) LoadSnapshot(ctx context.Context, aggregateID string) (*eventsourcing.Snapshot, error) {
	ctx, span := tracer().Start(ctx, "LoadSnapshot")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID))

	err := validateUUID(aggregateID, eventsourcing.ErrInvalidAggregateID)
	if err != nil {
		return nil, recordError(span, err)
	}

	statement, args, err := psql.Select(
		columnAggregateID, columnAggregateType, columnVersion, columnSchemaVersion, "payload", "created_at", "updated_at",
	).
		From(tableSnapshots).
		Where(squirrel.Eq{columnAggregateID: aggregateID}).
		ToSql()
	if err != nil {
		return nil, recordError(span, err)
	}

	var snapshot eventsourcing.Snapshot

	err = s.conn(ctx).QueryRow(ctx, statement, args...).Scan(
		&snapshot.AggregateID, &snapshot.AggregateType, &snapshot.Version, &snapshot.SchemaVersion,
		&snapshot.Payload, &snapshot.CreatedAt, &snapshot.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", eventsourcing.ErrSnapshotNotFound, aggregateID)
	}

	if err != nil {
		return nil, recordError(span, err)
	}

	return &snapshot, nil
}

// SaveSnapshot saves the snapshot unless a newer one is stored: one of a later schema
// version, or of a later version in the same schema version.
func (s *Store) SaveSnapshot(ctx context.Context, snapshot *eventsourcing.Snapshot) error {
	ctx, span := tracer().Start(ctx, "SaveSnapshot")
	defer span.End()

	if snapshot == nil {
		return recordError(span, fmt.Errorf("%w: nil snapshot", eventsourcing.ErrInvalidEvent))
	}

	span.SetAttributes(semconv.AggregateID(snapshot.AggregateID), semconv.AggregateType(snapshot.AggregateType))

	err := validateUUID(snapshot.AggregateID, eventsourcing.ErrInvalidAggregateID)
	if err != nil {
		return recordError(span, err)
	}

	if !json.Valid(snapshot.Payload) {
		return recordError(span, fmt.Errorf("%w: snapshot payload is not JSON", eventsourcing.ErrInvalidEvent))
	}

	schemaVersion := snapshot.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = defaultSchemaVersion
	}

	statement, args, err := psql.Insert(tableSnapshots).
		Columns(columnAggregateID, columnAggregateType, columnVersion, columnSchemaVersion, "payload").
		Values(snapshot.AggregateID, snapshot.AggregateType, snapshot.Version, schemaVersion, snapshot.Payload).
		Suffix(`ON CONFLICT (aggregate_id) DO UPDATE
			SET version = EXCLUDED.version, schema_version = EXCLUDED.schema_version,
				payload = EXCLUDED.payload, updated_at = now()
			WHERE (snapshots.schema_version, snapshots.version) < (EXCLUDED.schema_version, EXCLUDED.version)`).
		ToSql()
	if err != nil {
		return recordError(span, err)
	}

	_, err = s.conn(ctx).Exec(ctx, statement, args...)
	if err != nil {
		return recordError(span, err)
	}

	return nil
}

// AggregatesDueForSnapshot returns up to limit aggregates that gained at least
// minEvents events since their last snapshot, least recently changed first. An
// aggregate leaves the result once its snapshot is saved, so call it again until it
// returns nothing.
func (s *Store) AggregatesDueForSnapshot(ctx context.Context, minEvents int64, limit uint64) ([]eventsourcing.AggregateRef, error) {
	ctx, span := tracer().Start(ctx, "AggregatesDueForSnapshot")
	defer span.End()

	statement, args, err := psql.Select("aggregates.id", "aggregates.type", "aggregates.version").
		From(tableAggregates+" AS aggregates").
		LeftJoin(tableSnapshots+" AS snapshots ON aggregates.id = snapshots.aggregate_id").
		Where(squirrel.Expr("aggregates.version - COALESCE(snapshots.version, 0) >= ?", max(minEvents, 1))).
		OrderBy("aggregates.updated_at", "aggregates.id").
		Limit(limit).
		ToSql()
	if err != nil {
		return nil, recordError(span, err)
	}

	rows, err := s.conn(ctx).Query(ctx, statement, args...)
	if err != nil {
		return nil, recordError(span, err)
	}

	aggregates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (eventsourcing.AggregateRef, error) {
		var aggregate eventsourcing.AggregateRef

		return aggregate, row.Scan(&aggregate.ID, &aggregate.Type, &aggregate.Version)
	})
	if err != nil {
		return nil, recordError(span, err)
	}

	return aggregates, nil
}
