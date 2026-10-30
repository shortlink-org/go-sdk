//go:build database

package es_postgres

import (
	"context"
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
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shortlink-org/go-sdk/config"
	db "github.com/shortlink-org/go-sdk/db/drivers/postgres"
	eventsourcing "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	"github.com/shortlink-org/go-sdk/uow"
)

func TestMain(m *testing.M) {
	// TODO: research how correct close store
	// pgxpool: https://github.com/jackc/pgx/pull/1642
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("github.com/golang/glog.(*fileSink).flushDaemon"),
		goleak.IgnoreTopFunction("sync.runtime_Semacquire"))
}

func newEvent(aggregateID string, version int32) *eventsourcing.Event {
	return &eventsourcing.Event{
		AggregateId:   aggregateID,
		AggregateType: "test-type",
		Type:          fmt.Sprintf("test-event-%d", version),
		Payload:       fmt.Sprintf(`{"version": %d}`, version),
		Version:       version,
	}
}

func TestPostgres(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cfg, err := config.New()
	require.NoError(t, err)
	st := db.New(noop.NewTracerProvider(), metric.NewMeterProvider(), cfg)

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

	// new event sourcing store
	eventSourcing, err := New(ctx, st)
	if err != nil {
		t.Fatalf("Could not create store: %s", err)
	}

	t.Run("TestEventSourcingSaveAndLoad", func(t *testing.T) {
		id := uuid.NewV7().String()
		event := newEvent(id, 1)

		require.NoError(t, eventSourcing.Save(ctx, []*eventsourcing.Event{event}), "Could not save event")

		_, events, err := eventSourcing.Load(ctx, id)
		require.NoError(t, err, "Could not load events")
		require.Len(t, events, 1)

		loaded := events[0]
		require.Equal(t, event.GetAggregateId(), loaded.GetAggregateId())
		require.Equal(t, event.GetAggregateType(), loaded.GetAggregateType())
		require.Equal(t, event.GetType(), loaded.GetType())
		require.JSONEq(t, event.GetPayload(), loaded.GetPayload())
		require.Equal(t, event.GetVersion(), loaded.GetVersion())
	})

	t.Run("TestEventSourcingKeepsIDAndCreatedAt", func(t *testing.T) {
		id := uuid.NewV7().String()
		createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

		event := newEvent(id, 1)
		event.Id = uuid.NewV7().String()
		event.CreatedAt = timestamppb.New(createdAt)

		generated := newEvent(id, 2)

		require.NoError(t, eventSourcing.Save(ctx, []*eventsourcing.Event{event, generated}))
		require.NotEmpty(t, generated.GetId(), "Save fills in the id")
		require.NotNil(t, generated.GetCreatedAt(), "Save fills in the time")

		_, events, err := eventSourcing.Load(ctx, id)
		require.NoError(t, err)
		require.Len(t, events, 2)
		require.Equal(t, event.GetId(), events[0].GetId())
		require.True(t, createdAt.Equal(events[0].GetCreatedAt().AsTime()))
		require.Equal(t, generated.GetId(), events[1].GetId())
	})

	t.Run("TestEventSourcingLoadOrdersByVersion", func(t *testing.T) {
		id := uuid.NewV7().String()
		sameSecond := timestamppb.New(time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))

		var batch []*eventsourcing.Event

		for version := int32(1); version <= 50; version++ {
			event := newEvent(id, version)
			event.CreatedAt = sameSecond
			batch = append(batch, event)
		}

		require.NoError(t, eventSourcing.Save(ctx, batch))

		_, events, err := eventSourcing.Load(ctx, id)
		require.NoError(t, err)
		require.Len(t, events, 50)

		for i, event := range events {
			require.EqualValues(t, i+1, event.GetVersion())
		}
	})

	t.Run("TestEventSourcingLoadNoEvents", func(t *testing.T) {
		snapshot, events, err := eventSourcing.Load(ctx, uuid.NewV7().String())
		require.NoError(t, err)
		require.Empty(t, events)
		require.EqualValues(t, 0, snapshot.GetAggregateVersion())
	})

	t.Run("TestEventSourcingRejectsInvalidID", func(t *testing.T) {
		_, _, err := eventSourcing.Load(ctx, "order-1")
		require.ErrorIs(t, err, ErrInvalidAggregateID)

		require.ErrorIs(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent("order-1", 1)}), ErrInvalidAggregateID)
	})

	t.Run("TestEventSourcingConflict", func(t *testing.T) {
		t.Run("on the first event", func(t *testing.T) {
			id := uuid.NewV7().String()

			require.NoError(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 1)}))
			require.ErrorIs(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 1)}), ErrConcurrencyConflict)
		})

		t.Run("on a later event", func(t *testing.T) {
			id := uuid.NewV7().String()

			require.NoError(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 1), newEvent(id, 2)}))
			require.ErrorIs(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 2)}), ErrConcurrencyConflict)
			require.ErrorIs(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 4)}), ErrConcurrencyConflict)
		})
	})

	t.Run("TestEventSourcingSaveIsAtomic", func(t *testing.T) {
		id := uuid.NewV7().String()

		require.NoError(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 1)}))

		broken := newEvent(id, 3)
		broken.Payload = "not json"

		require.Error(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 2), broken}))

		_, events, err := eventSourcing.Load(ctx, id)
		require.NoError(t, err)
		require.Len(t, events, 1, "the failed batch must leave nothing behind")

		// The aggregate must not be stuck: the retry succeeds.
		require.NoError(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 2), newEvent(id, 3)}))

		_, events, err = eventSourcing.Load(ctx, id)
		require.NoError(t, err)
		require.Len(t, events, 3)
	})

	t.Run("TestEventSourcingFirstEventFailureLeavesNoAggregate", func(t *testing.T) {
		id := uuid.NewV7().String()

		broken := newEvent(id, 1)
		broken.Payload = "not json"

		require.Error(t, eventSourcing.Save(ctx, []*eventsourcing.Event{broken}))
		require.NoError(t, eventSourcing.Save(ctx, []*eventsourcing.Event{newEvent(id, 1)}), "the retry must not hit the primary key")
	})

	t.Run("TestEventSourcingUsesTransactionFromContext", func(t *testing.T) {
		conn, ok := st.GetConn().(*pgxpool.Pool)
		require.True(t, ok)

		rolledBack := uuid.NewV7().String()
		committed := uuid.NewV7().String()

		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, eventSourcing.Save(uow.WithTx(ctx, tx), []*eventsourcing.Event{newEvent(rolledBack, 1)}))

		_, events, err := eventSourcing.Load(uow.WithTx(ctx, tx), rolledBack)
		require.NoError(t, err)
		require.Len(t, events, 1, "Load must see the open transaction")
		require.NoError(t, tx.Rollback(ctx))

		_, events, err = eventSourcing.Load(ctx, rolledBack)
		require.NoError(t, err)
		require.Empty(t, events, "the rollback of the caller must undo Save")

		require.NoError(t, pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			return eventSourcing.Save(uow.WithTx(ctx, tx), []*eventsourcing.Event{newEvent(committed, 1)})
		}))

		_, events, err = eventSourcing.Load(ctx, committed)
		require.NoError(t, err)
		require.Len(t, events, 1)
	})

	t.Run("TestEventSourcingSnapshots", func(t *testing.T) {
		id := uuid.NewV7().String()

		var batch []*eventsourcing.Event
		for version := int32(1); version <= 5; version++ {
			batch = append(batch, newEvent(id, version))
		}

		require.NoError(t, eventSourcing.Save(ctx, batch))

		snapshotAt := func(version int32) *eventsourcing.Snapshot {
			return &eventsourcing.Snapshot{
				AggregateId:      id,
				AggregateType:    "test-type",
				AggregateVersion: version,
				Payload:          fmt.Sprintf(`{"version": %d}`, version),
			}
		}

		require.NoError(t, eventSourcing.SaveSnapshot(ctx, snapshotAt(3)))

		snapshot, events, err := eventSourcing.Load(ctx, id)
		require.NoError(t, err)
		require.EqualValues(t, 3, snapshot.GetAggregateVersion())
		require.NotNil(t, snapshot.GetCreatedAt())
		require.Len(t, events, 2, "only the events after the snapshot")
		require.EqualValues(t, 4, events[0].GetVersion())

		// An older snapshot must not replace a newer one.
		require.NoError(t, eventSourcing.SaveSnapshot(ctx, snapshotAt(2)))

		snapshot, _, err = eventSourcing.Load(ctx, id)
		require.NoError(t, err)
		require.EqualValues(t, 3, snapshot.GetAggregateVersion())

		require.NoError(t, eventSourcing.SaveSnapshot(ctx, snapshotAt(5)))

		snapshot, events, err = eventSourcing.Load(ctx, id)
		require.NoError(t, err)
		require.EqualValues(t, 5, snapshot.GetAggregateVersion())
		require.Empty(t, events)
	})

	t.Run("TestEventSourcingGetAggregateWithoutSnapshot", func(t *testing.T) {
		store, err := New(ctx, st, WithSnapshotBatchSize(1_000_000), WithSnapshotThreshold(3))
		require.NoError(t, err)

		contains := func(id string) bool {
			aggregates, err := store.GetAggregateWithoutSnapshot(ctx)
			require.NoError(t, err)

			for _, aggregate := range aggregates {
				if aggregate.GetId() == id {
					return true
				}
			}

			return false
		}

		id := uuid.NewV7().String()

		require.NoError(t, store.Save(ctx, []*eventsourcing.Event{newEvent(id, 1), newEvent(id, 2)}))
		require.False(t, contains(id), "below the threshold")

		require.NoError(t, store.Save(ctx, []*eventsourcing.Event{newEvent(id, 3)}))
		require.True(t, contains(id), "at the threshold")

		require.NoError(t, store.SaveSnapshot(ctx, &eventsourcing.Snapshot{
			AggregateId: id, AggregateType: "test-type", AggregateVersion: 3, Payload: `{}`,
		}))
		require.False(t, contains(id), "the snapshot resets the count")

		limited, err := New(ctx, st, WithSnapshotBatchSize(1), WithSnapshotThreshold(1))
		require.NoError(t, err)

		aggregates, err := limited.GetAggregateWithoutSnapshot(ctx)
		require.NoError(t, err)
		require.Len(t, aggregates, 1)
	})
}
