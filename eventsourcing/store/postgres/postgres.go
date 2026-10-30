package es_postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"
	"uuid"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shortlink-org/go-sdk/db"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/migrate"
	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
	"github.com/shortlink-org/go-sdk/uow"
)

const instrumentationName = "github.com/shortlink-org/go-sdk/eventsourcing"

var (
	//go:embed migrations/*.sql
	migrations embed.FS

	psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
)

// Migrations returns the schema of the store, a "migrations" directory, for
// applications that apply it themselves. See WithoutMigrations.
func Migrations() fs.FS {
	return migrations
}

// New returns an EventStore on the primary pool of store. Aggregates are
// rebuilt from what Load returns, so reads must see the latest write.
func New(ctx context.Context, store db.DB, opts ...Option) (*EventStore, error) {
	conn, err := db.Conn[*pgxpool.Pool](store)
	if err != nil {
		return nil, err
	}

	config := options{
		snapshotBatchSize: defaultSnapshotBatchSize,
		snapshotThreshold: defaultSnapshotThreshold,
	}

	for _, opt := range opts {
		opt(&config)
	}

	// Migration ---------------------------------------------------------------------------------------------------
	if !config.skipMigrations {
		err = migrate.Migration(ctx, store, migrations, "eventsourcing")
		if err != nil {
			return nil, err
		}
	}

	meter := otel.Meter(instrumentationName)

	eventsAppended, err := semconv.NewEventsAppendedTotal(meter)
	if err != nil {
		return nil, err
	}

	conflicts, err := semconv.NewConcurrencyConflictsTotal(meter)
	if err != nil {
		return nil, err
	}

	return &EventStore{
		db:             conn,
		opts:           config,
		eventsAppended: eventsAppended,
		conflicts:      conflicts,
	}, nil
}

// save writes the events atomically: in the transaction ctx carries, see the
// uow package, or else in its own.
func (e *EventStore) save(ctx context.Context, span trace.Span, events []*eventsourcing.Event) error {
	if len(events) == 0 {
		return nil
	}

	err := validateEvents(events)
	if err != nil {
		return recordError(span, err)
	}

	if tx := uow.FromContext(ctx); tx != nil {
		err = e.append(ctx, tx, events)
	} else {
		err = pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
			return e.append(ctx, tx, events)
		})
	}

	if err != nil {
		if _, ok := errors.AsType[*ConcurrencyConflictError](err); ok {
			e.conflicts.Add(ctx, 1, events[0].GetAggregateType())
		}

		return recordError(span, err)
	}

	for _, event := range events {
		e.eventsAppended.Add(ctx, 1, event.GetAggregateType())
	}

	return nil
}

func (e *EventStore) append(ctx context.Context, conn querier, events []*eventsourcing.Event) error {
	for _, event := range events {
		// Either insert a new aggregate or append to an existing.
		var err error
		if event.GetVersion() == 1 {
			err = e.addAggregate(ctx, conn, event)
		} else {
			err = e.updateAggregate(ctx, conn, event)
		}

		if err != nil {
			return err
		}

		err = e.addEvent(ctx, conn, event)
		if err != nil {
			return err
		}
	}

	return nil
}

// Save appends the events atomically. It returns an error matching
// ErrConcurrencyConflict when the aggregate moved past the expected version,
// and fills in Id and CreatedAt of the events that do not have them.
func (e *EventStore) Save(ctx context.Context, events []*eventsourcing.Event) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "Save")
	defer span.End()

	return e.save(ctx, span, events)
}

// SafeSave is the same as Save, which already checks the expected version.
//
// Deprecated: use Save.
func (e *EventStore) SafeSave(ctx context.Context, events []*eventsourcing.Event) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "SafeSave")
	defer span.End()

	return e.save(ctx, span, events)
}

// Load returns the latest snapshot and the events after it, in version order.
// An aggregate that does not exist yields an empty snapshot and no events.
func (e *EventStore) Load(ctx context.Context, aggregateID string) (*eventsourcing.Snapshot, []*eventsourcing.Event, error) {
	// start tracing
	ctx, span := tracer().Start(ctx, "Load")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID))

	err := validateUUID(aggregateID, ErrInvalidAggregateID)
	if err != nil {
		return nil, nil, recordError(span, err)
	}

	conn := e.conn(ctx)

	snapshot, err := loadSnapshot(ctx, conn, aggregateID)
	if err != nil {
		return nil, nil, recordError(span, err)
	}

	events, err := loadEvents(ctx, conn, aggregateID, snapshot.GetAggregateVersion())
	if err != nil {
		return nil, nil, recordError(span, err)
	}

	return snapshot, events, nil
}

func loadSnapshot(ctx context.Context, conn querier, aggregateID string) (*eventsourcing.Snapshot, error) {
	statement, args, err := psql.Select("aggregate_id", "aggregate_type", "aggregate_version", "payload", "created_at", "updated_at").
		From("snapshots").
		Where(squirrel.Eq{"aggregate_id": aggregateID}).
		ToSql()
	if err != nil {
		return nil, err
	}

	var (
		snapshot             eventsourcing.Snapshot
		createdAt, updatedAt time.Time
	)

	err = conn.QueryRow(ctx, statement, args...).
		Scan(&snapshot.AggregateId, &snapshot.AggregateType, &snapshot.AggregateVersion, &snapshot.Payload, &createdAt, &updatedAt)
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
	statement, args, err := psql.Select("aggregate_type", "id", "type", "payload", "version", "created_at").
		From("events").
		Where(squirrel.And{
			squirrel.Eq{"aggregate_id": aggregateID},
			squirrel.Gt{"version": afterVersion},
		}).
		OrderBy("version").
		ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var events []*eventsourcing.Event //nolint:prealloc // false positive

	for rows.Next() {
		var createdAt time.Time

		event := &eventsourcing.Event{AggregateId: aggregateID}

		err = rows.Scan(&event.AggregateType, &event.Id, &event.Type, &event.Payload, &event.Version, &createdAt)
		if err != nil {
			return nil, err
		}

		event.CreatedAt = timestamppb.New(createdAt)
		events = append(events, event)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return events, nil
}

func validateEvents(events []*eventsourcing.Event) error {
	for i, event := range events {
		if event == nil {
			return fmt.Errorf("%w: event %d is nil", ErrInvalidEvent, i)
		}

		if event.GetVersion() < 1 {
			return fmt.Errorf("%w: event %d has version %d", ErrInvalidEvent, i, event.GetVersion())
		}

		err := validateUUID(event.GetAggregateId(), ErrInvalidAggregateID)
		if err != nil {
			return err
		}

		if event.GetId() != "" {
			err = validateUUID(event.GetId(), ErrInvalidEventID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func validateUUID(value string, sentinel error) error {
	_, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("%w: %q", sentinel, value)
	}

	return nil
}

func tracer() trace.Tracer { //nolint:ireturn // the OpenTelemetry API returns the interface
	return otel.Tracer(instrumentationName)
}

func recordError(span trace.Span, err error) error {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	return err
}
