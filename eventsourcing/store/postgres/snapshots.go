package es_postgres

import (
	"context"
	"time"

	"github.com/Masterminds/squirrel"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
)

// GetAggregateWithoutSnapshot returns the aggregates that gained at least
// WithSnapshotThreshold events since their last snapshot, least recently
// changed first, at most WithSnapshotBatchSize of them. An aggregate leaves the
// result once its snapshot is saved, so call it again until it returns nothing.
func (e *EventStore) GetAggregateWithoutSnapshot(ctx context.Context) ([]*eventsourcing.BaseAggregate, error) {
	// start tracing
	ctx, span := tracer().Start(ctx, "GetAggregateWithoutSnapshot")
	defer span.End()

	query := psql.Select("aggregates.id", "aggregates.type", "aggregates.version").
		From("aggregates AS aggregates").
		LeftJoin("snapshots AS snapshots ON aggregates.id = snapshots.aggregate_id").
		Where(squirrel.Expr("aggregates.version - COALESCE(snapshots.aggregate_version, 0) >= ?", e.opts.snapshotThreshold)).
		OrderBy("aggregates.updated_at", "aggregates.id").
		Limit(e.opts.snapshotBatchSize)

	q, args, err := query.ToSql()
	if err != nil {
		return nil, recordError(span, err)
	}

	rows, err := e.conn(ctx).Query(ctx, q, args...)
	if err != nil {
		return nil, recordError(span, err)
	}

	defer rows.Close()

	var aggregates []*eventsourcing.BaseAggregate //nolint:prealloc // false positive

	for rows.Next() {
		aggregate := &eventsourcing.BaseAggregate{}

		err = rows.Scan(&aggregate.Id, &aggregate.Type, &aggregate.Version)
		if err != nil {
			return nil, recordError(span, err)
		}

		aggregates = append(aggregates, aggregate)
	}

	if rows.Err() != nil {
		return nil, recordError(span, rows.Err())
	}

	return aggregates, nil
}

// SaveSnapshot saves the snapshot unless a newer one is already stored.
func (e *EventStore) SaveSnapshot(ctx context.Context, snapshot *eventsourcing.Snapshot) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "SaveSnapshot")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(snapshot.GetAggregateId()))

	err := validateUUID(snapshot.GetAggregateId(), ErrInvalidAggregateID)
	if err != nil {
		return recordError(span, err)
	}

	now := time.Now().UTC()

	query := psql.Insert("snapshots").
		Columns("aggregate_id", "aggregate_type", "aggregate_version", "payload", "created_at", "updated_at").
		Values(snapshot.GetAggregateId(), snapshot.GetAggregateType(), snapshot.GetAggregateVersion(), snapshot.GetPayload(), now, now).
		Suffix(`ON CONFLICT (aggregate_id) DO UPDATE
			SET aggregate_version = EXCLUDED.aggregate_version, payload = EXCLUDED.payload, updated_at = EXCLUDED.updated_at
			WHERE snapshots.aggregate_version < EXCLUDED.aggregate_version`)

	q, args, err := query.ToSql()
	if err != nil {
		return recordError(span, err)
	}

	_, err = e.conn(ctx).Exec(ctx, q, args...)
	if err != nil {
		return recordError(span, err)
	}

	return nil
}
