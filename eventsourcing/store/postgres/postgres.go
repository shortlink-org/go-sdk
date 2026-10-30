package es_postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"uuid"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

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

// Append writes the events as the next versions of the aggregate, in one
// transaction with one UPDATE of the aggregate and batched INSERTs. It returns an
// error matching ErrConcurrencyConflict unless the aggregate is at expectedVersion,
// 0 for an aggregate that does not exist yet.
//
// Events get the aggregate id and their version from this call. Setting a different
// one is an ErrInvalidEvent. Like Save, it fills in what the events do not set.
func (e *EventStore) Append(ctx context.Context, aggregateID string, expectedVersion int32, events []*eventsourcing.Event) error {
	// start tracing
	ctx, span := tracer().Start(ctx, "Append")
	defer span.End()

	span.SetAttributes(semconv.AggregateID(aggregateID))

	err := stampEvents(aggregateID, expectedVersion, events)
	if err != nil {
		return recordError(span, err)
	}

	return e.save(ctx, span, events)
}

// stampEvents gives the events the aggregate id and the versions Append expects.
func stampEvents(aggregateID string, expectedVersion int32, events []*eventsourcing.Event) error {
	if expectedVersion < 0 {
		return fmt.Errorf("%w: expected version %d", ErrInvalidEvent, expectedVersion)
	}

	version := expectedVersion

	for i, event := range events {
		version++

		if event == nil {
			return fmt.Errorf("%w: event %d is nil", ErrInvalidEvent, i)
		}

		if event.GetAggregateId() != "" && event.GetAggregateId() != aggregateID {
			return fmt.Errorf("%w: event %d belongs to aggregate %s", ErrInvalidEvent, i, event.GetAggregateId())
		}

		if event.GetVersion() != 0 && event.GetVersion() != version {
			return fmt.Errorf("%w: event %d has version %d, expected %d", ErrInvalidEvent, i, event.GetVersion(), version)
		}

		event.AggregateId = aggregateID
		event.Version = version
	}

	return nil
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
		err = e.appendRuns(ctx, tx, events)
	} else {
		err = pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
			return e.appendRuns(ctx, tx, events)
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

// appendRuns writes each run of consecutive versions of one aggregate with one
// UPDATE. A gap starts a new run, whose expected version then fails to match.
func (e *EventStore) appendRuns(ctx context.Context, conn querier, events []*eventsourcing.Event) error {
	for start := 0; start < len(events); {
		end := start + 1
		for end < len(events) &&
			events[end].GetAggregateId() == events[start].GetAggregateId() &&
			events[end].GetVersion() == events[end-1].GetVersion()+1 {
			end++
		}

		err := e.appendRun(ctx, conn, events[start:end])
		if err != nil {
			return err
		}

		start = end
	}

	return nil
}

func (e *EventStore) appendRun(ctx context.Context, conn querier, run []*eventsourcing.Event) error {
	first, last := run[0], run[len(run)-1]

	// Either insert a new aggregate or append to an existing.
	var err error
	if first.GetVersion() == 1 {
		err = e.addAggregate(ctx, conn, first, last.GetVersion())
	} else {
		err = e.updateAggregate(ctx, conn, first, last.GetVersion())
	}

	if err != nil {
		return err
	}

	return e.addEvents(ctx, conn, run)
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
