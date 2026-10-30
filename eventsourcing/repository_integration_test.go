//go:build database

package eventsourcing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/shortlink-org/go-sdk/config"
	db "github.com/shortlink-org/go-sdk/db/drivers/postgres"
	"github.com/shortlink-org/go-sdk/eventsourcing"
	v1 "github.com/shortlink-org/go-sdk/eventsourcing/domain/eventsourcing/v1"
	es_postgres "github.com/shortlink-org/go-sdk/eventsourcing/store/postgres"
)

// ledger is an aggregate with snapshots.
type ledger struct {
	v1.BaseAggregate

	Balance  int `json:"balance"`
	restored bool
}

func (l *ledger) ApplyChange(_ context.Context, event *v1.Event) error {
	var payload struct {
		Amount int `json:"amount"`
	}

	err := json.Unmarshal([]byte(event.GetPayload()), &payload)
	if err != nil {
		return err
	}

	l.Balance += payload.Amount

	return nil
}

func (*ledger) HandleCommand(context.Context, *v1.BaseCommand) error {
	return nil
}

func (*ledger) SnapshotSchemaVersion() int32 {
	return 1
}

func (l *ledger) MarshalSnapshot() (string, error) {
	raw, err := json.Marshal(l)

	return string(raw), err
}

func (l *ledger) RestoreSnapshot(payload string) error {
	l.restored = true

	return json.Unmarshal([]byte(payload), l)
}

func (l *ledger) deposit(t *testing.T, amount int) {
	t.Helper()

	event := &v1.Event{Type: "deposited", Payload: fmt.Sprintf(`{"amount": %d}`, amount)}
	require.NoError(t, l.ApplyChangeHelper(t.Context(), l, event, true))
}

func TestRepositoryOnPostgres(t *testing.T) {
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

	store, err := es_postgres.New(ctx, st)
	require.NoError(t, err)

	repository, err := eventsourcing.NewRepository(store, "ledger", func() *ledger { return &ledger{} }, eventsourcing.WithSnapshotEvery(2))
	require.NoError(t, err)

	aggregate := repository.New()
	aggregate.deposit(t, 10)
	aggregate.deposit(t, 20)
	aggregate.deposit(t, 30)
	require.NoError(t, repository.Save(ctx, aggregate))

	loaded, err := repository.Load(ctx, aggregate.GetId())
	require.NoError(t, err)
	require.True(t, loaded.restored, "the save crossed the snapshot policy")
	require.Equal(t, 60, loaded.Balance)
	require.EqualValues(t, 3, loaded.GetVersion())

	loaded.deposit(t, 1)
	require.NoError(t, repository.Save(ctx, loaded))

	aggregate.deposit(t, 1)
	require.ErrorIs(t, repository.Save(ctx, aggregate), eventsourcing.ErrConcurrencyConflict)

	reloaded, err := repository.Load(ctx, aggregate.GetId())
	require.NoError(t, err)
	require.Equal(t, 61, reloaded.Balance)
	require.EqualValues(t, 4, reloaded.GetVersion())

	events, err := store.ReadAll(ctx, "", 0)
	require.NoError(t, err)
	require.Len(t, events, 4)
}
