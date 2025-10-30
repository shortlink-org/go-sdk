package es_postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
)

// GetAggregateWithoutSnapshot - get aggregates without a snapshot
func (e *EventStore) GetAggregateWithoutSnapshot(ctx context.Context) ([]*eventsourcing.BaseAggregate, error) {
	query := psql.Select("aggregates.id", "aggregates.type", "aggregates.version").
		From("aggregates AS aggregates").
		LeftJoin("snapshots AS snapshots ON aggregates.id = snapshots.aggregate_id").
		Where("aggregates.version > snapshots.aggregate_version OR snapshots.aggregate_version IS NULL")

	q, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(ctx, q, args...)
	if err != nil || rows.Err() != nil {
		return nil, err
	}

	defer rows.Close()

	var aggregates []*eventsourcing.BaseAggregate //nolint:prealloc // false positive

	for rows.Next() {
		var (
			id            sql.NullString
			typeAggregate sql.NullString
			version       sql.NullInt32
		)
		err = rows.Scan(&id, &typeAggregate, &version)
		if err != nil {
			return nil, err
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}

		aggregates = append(aggregates, &eventsourcing.BaseAggregate{
			Id:      id.String,
			Type:    typeAggregate.String,
			Version: version.Int32,
		})
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return aggregates, nil
}

// SaveSnapshot - save snapshot
func (e *EventStore) SaveSnapshot(ctx context.Context, snapshot *eventsourcing.Snapshot) error {
	// TODO: use worker pool
	//nolint:wsl // TODO: fix this

	// start tracing
	_, span := otel.Tracer("snapshot").Start(ctx, "SaveSnapshot")
	defer span.End()

	span.SetAttributes(attribute.String("aggregate id", snapshot.GetAggregateId()))

	query := psql.Insert("snapshots").
		Columns("aggregate_id", "aggregate_type", "aggregate_version", "payload").
		Values(snapshot.GetAggregateId(), snapshot.GetAggregateType(), snapshot.GetAggregateVersion(), snapshot.GetPayload()).
		Suffix("ON CONFLICT (aggregate_id) DO UPDATE SET aggregate_version = ?, payload = ?, updated_at = ?", snapshot.GetAggregateVersion(), snapshot.GetPayload(), time.Now())

	q, args, err := query.ToSql()
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
