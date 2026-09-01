//go:build unit || (database && postgres)

package outbox_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/migrate"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/outbox"
	"github.com/shortlink-org/go-sdk/uow"
	sdkwm "github.com/shortlink-org/go-sdk/watermill"
)

const (
	testTopic    = "orders"
	testDLQTopic = "orders.dlq"
	testTimeout  = 30 * time.Second
	// How long a "nothing arrived" assertion waits. It has to outlast a poll
	// interval and a round of retries, both single-digit milliseconds here.
	testQuiet      = 750 * time.Millisecond
	testMaxRetries = 3
)

// testStore is the db.DB the relay needs: the postgres driver hands out a
// *pgxpool.Pool, and so does this.
type testStore struct {
	pool *pgxpool.Pool
}

func (s *testStore) Init(context.Context) error { return nil }
func (s *testStore) GetConn() any               { return s.pool }

func setupPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:latest",
		postgres.WithDatabase("outbox"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("outbox"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, container)
	require.NoError(t, err)

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	t.Cleanup(pool.Close)

	// Through the documented path, so that the embedded layout and the file
	// names are covered too, not just the SQL.
	require.NoError(t, migrate.Migration(ctx, &testStore{pool: pool}, outbox.Migrations, "outbox"))

	return pool
}

func newTestLogger(t *testing.T) *logger.SlogLogger {
	t.Helper()

	log, err := logger.New(logger.Configuration{
		Writer:     io.Discard,
		TimeFormat: time.RFC3339Nano,
		Level:      logger.ERROR_LEVEL,
	})
	require.NoError(t, err)

	return log
}

// memoryBackend is the in-process Backend watermill.New requires. The relay
// reads the outbox, not this, so it exists only to satisfy the constructor and
// to carry dead letters.
type memoryBackend struct {
	pubsub *gochannel.GoChannel
}

//nolint:ireturn // Backend is the SDK's own contract
func (b *memoryBackend) Publisher() message.Publisher { return b.pubsub }

//nolint:ireturn // Backend is the SDK's own contract
func (b *memoryBackend) Subscriber() message.Subscriber { return b.pubsub }

func (b *memoryBackend) Close() error { return nil }

// newRouter returns a router carrying the SDK's middleware stack: retry
// wrapped by the poison queue, which is the arrangement the relay's delivery
// guarantees depend on.
func newRouter(t *testing.T, pubsub *gochannel.GoChannel) *sdkwm.Client {
	t.Helper()

	cfg, err := config.New()
	require.NoError(t, err)

	client, err := sdkwm.New(
		context.Background(),
		newTestLogger(t),
		cfg,
		&memoryBackend{pubsub: pubsub},
		metricnoop.NewMeterProvider(),
		tracenoop.NewTracerProvider(),
		sdkwm.DisableTimeout(),
		// The breaker would trip on the repeated failures these tests provoke
		// and start rejecting before the assertions run.
		sdkwm.DisableCircuitBreaker(),
		sdkwm.WithRetryOptions(sdkwm.RetryOptions{
			Enabled:             true,
			MaxRetries:          testMaxRetries,
			InitialInterval:     time.Millisecond,
			MaxInterval:         5 * time.Millisecond,
			Multiplier:          2,
			Jitter:              0,
			MaxElapsedTime:      0,
			ResetContextOnRetry: false,
		}),
		sdkwm.WithPoisonQueue(pubsub, testDLQTopic),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = client.Close() //nolint:errcheck // teardown
	})

	return client
}

// publish writes msgs to the outbox in a transaction and commits it.
func publish(t *testing.T, pool *pgxpool.Pool, topic string, msgs ...*message.Message) {
	t.Helper()

	inTx(t, pool, func(ctx context.Context) {
		publisher, err := outbox.NewPublisher(uow.FromContext)
		require.NoError(t, err)
		require.NoError(t, publisher.Publish(ctx, topic, msgs...))
	}, true)
}

// inTx runs fn with a transaction in the context, then commits or rolls back.
func inTx(t *testing.T, pool *pgxpool.Pool, fn func(ctx context.Context), commit bool) {
	t.Helper()

	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	fn(uow.WithTx(ctx, tx))

	if commit {
		require.NoError(t, tx.Commit(ctx))

		return
	}

	require.NoError(t, tx.Rollback(ctx))
}

// runRelay starts the relay and stops it when the test ends.
func runRelay(t *testing.T, relay *outbox.Relay) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		_ = relay.Run(ctx) //nolint:errcheck // the relay stops when the test cancels ctx
	}()

	t.Cleanup(func() {
		cancel()

		select {
		case <-stopped:
		case <-time.After(testTimeout):
			t.Error("relay did not stop")
		}
	})
}

func countRows(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()

	var count int

	err := pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox WHERE "+where, args...).Scan(&count)
	require.NoError(t, err)

	return count
}

// requireNoMessage asserts that nothing arrives on ch within testQuiet.
func requireNoMessage(t *testing.T, ch <-chan *message.Message, reason string) {
	t.Helper()

	select {
	case msg := <-ch:
		t.Fatalf("%s: got %s", reason, msg.UUID)
	case <-time.After(testQuiet):
	}
}
