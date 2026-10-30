// Package postgres is the PostgreSQL implementation of the eventsourcing ports.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"uuid"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/db"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/migrate"
	"github.com/shortlink-org/go-sdk/eventsourcing"
	"github.com/shortlink-org/go-sdk/eventsourcing/semconv"
	"github.com/shortlink-org/go-sdk/uow"
)

const (
	instrumentationName = "github.com/shortlink-org/go-sdk/eventsourcing"

	tableAggregates = "eventsourcing.aggregates"
	tableEvents     = "eventsourcing.events"
	tableSnapshots  = "eventsourcing.snapshots"

	columnAggregateID   = "aggregate_id"
	columnAggregateType = "aggregate_type"
	columnSchemaVersion = "schema_version"
	columnVersion       = "version"
)

var (
	//go:embed migrations/*.sql
	migrations embed.FS

	psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	_ eventsourcing.Store             = (*Store)(nil)
	_ eventsourcing.GlobalReader      = (*Store)(nil)
	_ eventsourcing.SnapshotScheduler = (*Store)(nil)
)

// Migrations returns the schema of the store, a "migrations" directory, for
// applications that apply it themselves. See WithoutMigrations.
func Migrations() fs.FS {
	return migrations
}

// Option configures a Store.
type Option func(*options)

type options struct {
	skipMigrations bool
}

// WithoutMigrations stops New from applying the schema, for applications that
// run without DDL privileges and apply Migrations themselves.
func WithoutMigrations() Option {
	return func(o *options) {
		o.skipMigrations = true
	}
}

// Store keeps events, aggregates and snapshots in PostgreSQL. It always works on
// the primary pool: aggregates are rebuilt from what it reads, so reads must see
// the latest write.
type Store struct {
	db *pgxpool.Pool

	eventsAppended semconv.EventsAppendedTotal
	conflicts      semconv.ConcurrencyConflictsTotal
}

// New returns a Store on store, which must be the PostgreSQL driver.
func New(ctx context.Context, store db.DB, opts ...Option) (*Store, error) {
	conn, err := db.Conn[*pgxpool.Pool](store)
	if err != nil {
		return nil, err
	}

	var config options
	for _, opt := range opts {
		opt(&config)
	}

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

	return &Store{db: conn, eventsAppended: eventsAppended, conflicts: conflicts}, nil
}

// querier is what pgxpool.Pool and pgx.Tx have in common.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// conn returns the transaction ctx carries, see the uow package, or the pool.
//
//nolint:ireturn // the caller must not care which of the two it got
func (s *Store) conn(ctx context.Context) querier {
	if tx := uow.FromContext(ctx); tx != nil {
		return tx
	}

	return s.db
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
