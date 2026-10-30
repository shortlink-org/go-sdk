//go:build database

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
	driver "github.com/shortlink-org/go-sdk/db/drivers/postgres"
	"github.com/shortlink-org/go-sdk/eventsourcing"
	"github.com/shortlink-org/go-sdk/uow"
)

func TestMain(m *testing.M) {
	// TODO: research how correct close store
	// pgxpool: https://github.com/jackc/pgx/pull/1642
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("github.com/golang/glog.(*fileSink).flushDaemon"),
		goleak.IgnoreTopFunction("sync.runtime_Semacquire"))
}

func newEvent(eventType string) *eventsourcing.Event {
	return &eventsourcing.Event{AggregateType: "test-type", Type: eventType, Payload: json.RawMessage(`{"test": "payload"}`)}
}

func newEvents(count int) []*eventsourcing.Event {
	events := make([]*eventsourcing.Event, 0, count)
	for i := range count {
		events = append(events, newEvent(fmt.Sprintf("event-%d", i+1)))
	}

	return events
}

func TestPostgres(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cfg, err := config.New()
	require.NoError(t, err)
	st := driver.New(noop.NewTracerProvider(), metric.NewMeterProvider(), cfg)

	c, err := testcontainers.Run(ctx, "postgres:latest",
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "shortlink",
			"POSTGRES_DB":       "eventsourcing",
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(5*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	t.Setenv("STORE_POSTGRES_URI", fmt.Sprintf("postgres://postgres:shortlink@%s:%s/eventsourcing?sslmode=disable", host, mapped.Port()))

	require.Eventually(t, func() bool {
		return st.Init(ctx) == nil
	}, 5*time.Minute, time.Second, "postgres init")

	store, err := New(ctx, st)
	require.NoError(t, err)

	conn, ok := st.GetConn().(*pgxpool.Pool)
	require.True(t, ok)

	t.Run("AppendAndReadStream", func(t *testing.T) {
		id := uuid.NewV7().String()
		event := newEvent("created")

		require.NoError(t, store.Append(ctx, id, 0, []*eventsourcing.Event{event}))
		require.NotEmpty(t, event.ID, "Append fills in the id")
		require.False(t, event.CreatedAt.IsZero(), "Append fills in the time")
		require.Equal(t, 1, event.SchemaVersion, "Append fills in the schema version")

		events, err := store.ReadStream(ctx, id, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)

		loaded := events[0]
		require.Equal(t, event.ID, loaded.ID)
		require.Equal(t, id, loaded.AggregateID)
		require.Equal(t, "test-type", loaded.AggregateType)
		require.Equal(t, "created", loaded.Type)
		require.JSONEq(t, string(event.Payload), string(loaded.Payload))
		require.EqualValues(t, 1, loaded.Version)
		require.WithinDuration(t, event.CreatedAt, loaded.CreatedAt, time.Millisecond)
		require.NotEmpty(t, loaded.Cursor)
	})

	t.Run("ReadStreamReturnsNothingForAnUnknownAggregate", func(t *testing.T) {
		events, err := store.ReadStream(ctx, uuid.NewV7().String(), 0)
		require.NoError(t, err)
		require.Empty(t, events)
	})

	t.Run("RejectsAnInvalidAggregateID", func(t *testing.T) {
		_, err := store.ReadStream(ctx, "order-1", 0)
		require.ErrorIs(t, err, eventsourcing.ErrInvalidAggregateID)

		_, err = store.LoadSnapshot(ctx, "order-1")
		require.ErrorIs(t, err, eventsourcing.ErrInvalidAggregateID)

		require.ErrorIs(t, store.Append(ctx, "order-1", 0, newEvents(1)), eventsourcing.ErrInvalidAggregateID)
	})

	t.Run("AppendBeyondOneInsert", func(t *testing.T) {
		id := uuid.NewV7().String()
		createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

		batch := newEvents(2500)
		for _, event := range batch {
			event.CreatedAt = createdAt
		}

		require.NoError(t, store.Append(ctx, id, 0, batch))

		events, err := store.ReadStream(ctx, id, 0)
		require.NoError(t, err)
		require.Len(t, events, 2500)

		for i, event := range events {
			require.EqualValues(t, i+1, event.Version, "version order, whatever the timestamps")
		}
	})

	t.Run("Conflict", func(t *testing.T) {
		id := uuid.NewV7().String()

		require.NoError(t, store.Append(ctx, id, 0, newEvents(2)))

		require.ErrorIs(t, store.Append(ctx, id, 0, newEvents(1)), eventsourcing.ErrConcurrencyConflict, "the aggregate exists")
		require.ErrorIs(t, store.Append(ctx, id, 1, newEvents(1)), eventsourcing.ErrConcurrencyConflict, "behind")
		require.ErrorIs(t, store.Append(ctx, id, 3, newEvents(1)), eventsourcing.ErrConcurrencyConflict, "ahead")
		require.NoError(t, store.Append(ctx, id, 2, newEvents(1)))
	})

	t.Run("AppendIsAtomic", func(t *testing.T) {
		used := newEvent("used")
		require.NoError(t, store.Append(ctx, uuid.NewV7().String(), 0, []*eventsourcing.Event{used}))

		duplicate := func() []*eventsourcing.Event {
			return []*eventsourcing.Event{newEvent("fresh"), {
				ID: used.ID, AggregateType: "test-type", Type: "duplicate", Payload: json.RawMessage(`{}`),
			}}
		}

		t.Run("on a new aggregate", func(t *testing.T) {
			id := uuid.NewV7().String()

			require.ErrorIs(t, store.Append(ctx, id, 0, duplicate()), eventsourcing.ErrDuplicateEventID)

			events, err := store.ReadStream(ctx, id, 0)
			require.NoError(t, err)
			require.Empty(t, events)

			require.NoError(t, store.Append(ctx, id, 0, newEvents(2)), "the retry must not hit the primary key")
		})

		t.Run("on an existing aggregate", func(t *testing.T) {
			id := uuid.NewV7().String()

			require.NoError(t, store.Append(ctx, id, 0, newEvents(1)))
			require.ErrorIs(t, store.Append(ctx, id, 1, duplicate()), eventsourcing.ErrDuplicateEventID)

			events, err := store.ReadStream(ctx, id, 0)
			require.NoError(t, err)
			require.Len(t, events, 1, "the failed batch must leave nothing behind")

			require.NoError(t, store.Append(ctx, id, 1, newEvents(2)), "the aggregate must not be stuck")
		})
	})

	t.Run("UsesTheTransactionFromContext", func(t *testing.T) {
		rolledBack, committed := uuid.NewV7().String(), uuid.NewV7().String()

		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, store.Append(uow.WithTx(ctx, tx), rolledBack, 0, newEvents(1)))

		events, err := store.ReadStream(uow.WithTx(ctx, tx), rolledBack, 0)
		require.NoError(t, err)
		require.Len(t, events, 1, "ReadStream must see the open transaction")
		require.NoError(t, tx.Rollback(ctx))

		events, err = store.ReadStream(ctx, rolledBack, 0)
		require.NoError(t, err)
		require.Empty(t, events, "the rollback of the caller must undo Append")

		require.NoError(t, pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			return store.Append(uow.WithTx(ctx, tx), committed, 0, newEvents(1))
		}))

		events, err = store.ReadStream(ctx, committed, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
	})

	t.Run("Metadata", func(t *testing.T) {
		id := uuid.NewV7().String()

		traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
		require.NoError(t, err)
		spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
		require.NoError(t, err)

		traced := trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
		}))

		event := newEvent("created")
		event.SchemaVersion = 3
		event.CorrelationID = "request-1"
		event.CausationID = "command-1"
		event.Metadata = map[string]string{"tenant": "acme"}

		require.NoError(t, store.Append(traced, id, 0, []*eventsourcing.Event{event}))

		events, err := store.ReadStream(ctx, id, 0)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, 3, events[0].SchemaVersion)
		require.Equal(t, "request-1", events[0].CorrelationID)
		require.Equal(t, "command-1", events[0].CausationID)
		require.Equal(t, "acme", events[0].Metadata["tenant"])
		require.Contains(t, events[0].Metadata["traceparent"], traceID.String())
	})

	t.Run("ReadAll", func(t *testing.T) {
		readAfter := func(cursor string) ([]string, string) {
			var ids []string

			for {
				events, err := store.ReadAll(ctx, cursor, 500)
				require.NoError(t, err)

				if len(events) == 0 {
					return ids, cursor
				}

				for _, event := range events {
					ids = append(ids, event.AggregateID)
				}

				cursor = events[len(events)-1].Cursor
			}
		}

		_, cursor := readAfter("")

		slow, fast := uuid.NewV7().String(), uuid.NewV7().String()

		// The slow transaction takes the lower position and commits last.
		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, store.Append(uow.WithTx(ctx, tx), slow, 0, newEvents(1)))
		require.NoError(t, store.Append(ctx, fast, 0, newEvents(1)))

		ids, afterInFlight := readAfter(cursor)
		require.Empty(t, ids, "nothing may pass a transaction still in flight")
		require.Equal(t, cursor, afterInFlight)

		require.NoError(t, tx.Commit(ctx))

		ids, next := readAfter(cursor)
		require.Equal(t, []string{slow, fast}, ids, "no event may land behind the cursor")

		ids, _ = readAfter(next)
		require.Empty(t, ids)

		_, err = store.ReadAll(ctx, "garbage", 1)
		require.ErrorIs(t, err, eventsourcing.ErrInvalidCursor)
	})

	t.Run("Snapshots", func(t *testing.T) {
		id := uuid.NewV7().String()

		require.NoError(t, store.Append(ctx, id, 0, newEvents(5)))

		_, err := store.LoadSnapshot(ctx, id)
		require.ErrorIs(t, err, eventsourcing.ErrSnapshotNotFound)

		save := func(version int64, schemaVersion int) {
			require.NoError(t, store.SaveSnapshot(ctx, &eventsourcing.Snapshot{
				AggregateID: id, AggregateType: "test-type", Version: version, SchemaVersion: schemaVersion, Payload: json.RawMessage(`{}`),
			}))
		}

		loadVersions := func() (int64, int) {
			snapshot, err := store.LoadSnapshot(ctx, id)
			require.NoError(t, err)
			require.False(t, snapshot.CreatedAt.IsZero())

			return snapshot.Version, snapshot.SchemaVersion
		}

		save(3, 0)
		version, schemaVersion := loadVersions()
		require.EqualValues(t, 3, version)
		require.Equal(t, 1, schemaVersion, "the default")

		save(2, 1)
		version, _ = loadVersions()
		require.EqualValues(t, 3, version, "an older snapshot must not replace a newer one")

		save(1, 2)
		version, schemaVersion = loadVersions()
		require.EqualValues(t, 1, version, "a later schema replaces even a later version")
		require.Equal(t, 2, schemaVersion)

		save(5, 1)
		_, schemaVersion = loadVersions()
		require.Equal(t, 2, schemaVersion, "an older schema must not come back")
	})

	t.Run("AggregatesDueForSnapshot", func(t *testing.T) {
		contains := func(id string) bool {
			aggregates, err := store.AggregatesDueForSnapshot(ctx, 3, 1_000_000)
			require.NoError(t, err)

			for _, aggregate := range aggregates {
				if aggregate.ID == id {
					return true
				}
			}

			return false
		}

		id := uuid.NewV7().String()

		require.NoError(t, store.Append(ctx, id, 0, newEvents(2)))
		require.False(t, contains(id), "below the threshold")

		require.NoError(t, store.Append(ctx, id, 2, newEvents(1)))
		require.True(t, contains(id), "at the threshold")

		require.NoError(t, store.SaveSnapshot(ctx, &eventsourcing.Snapshot{
			AggregateID: id, AggregateType: "test-type", Version: 3, Payload: json.RawMessage(`{}`),
		}))
		require.False(t, contains(id), "the snapshot resets the count")

		aggregates, err := store.AggregatesDueForSnapshot(ctx, 1, 1)
		require.NoError(t, err)
		require.Len(t, aggregates, 1)
	})
}
