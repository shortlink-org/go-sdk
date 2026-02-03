//go:build integration

package bus_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	wmsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/cqrs/bus"
	"github.com/shortlink-org/go-sdk/cqrs/message"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/uow"
)

var integrationGoleakOpts = []goleak.Option{
	goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
	goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
	goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
}

const (
	forwarderTopic = "shortlink_cqrs_outbox_test"
	serviceName    = "cqrs-integration-test"
)

func setupPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err, "postgres container: ensure Docker is running")

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
		_ = container.Terminate(context.Background())
	})

	return pool
}

func TestIntegration_CommandBus_Outbox_NoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, integrationGoleakOpts...)

	pool := setupPostgres(t)
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	wmLogger := watermill.NewStdLogger(false, false)
	schema := wmsql.DefaultPostgreSQLSchema{}
	pgxBeginner := wmsql.PgxBeginner{Conn: pool}

	sqlPub, err := wmsql.NewPublisher(pgxBeginner, wmsql.PublisherConfig{
		SchemaAdapter:        schema,
		AutoInitializeSchema: true,
	}, wmLogger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlPub.Close() })

	sqlSub, err := wmsql.NewSubscriber(
		wmsql.BeginnerFromPgx(pool),
		wmsql.SubscriberConfig{
			SchemaAdapter:    schema,
			OffsetsAdapter:   wmsql.DefaultPostgreSQLOffsetsAdapter{},
			InitializeSchema: true,
			ConsumerGroup:    "test-consumer",
			PollInterval:     100 * time.Millisecond,
			AckDeadline:      ptrDuration(5 * time.Second),
		},
		wmLogger,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlSub.Close() })

	realPub := gochannel.NewGoChannel(gochannel.Config{}, wmLogger)
	namer := message.NewShortlinkNamer(serviceName)
	marshaler := message.NewJSONMarshaler(namer)

	cfg := logger.Default()
	cfg.Writer = io.Discard
	cfg.Level = logger.WARN_LEVEL
	log, err := logger.New(cfg)
	require.NoError(t, err)

	cmdBus, err := bus.NewCommandBusWithOptions(sqlPub, marshaler, namer,
		bus.WithOutbox(bus.OutboxConfig{
			DB:            sqlDB,
			Subscriber:    sqlSub,
			RealPublisher: realPub,
			ForwarderName: forwarderTopic,
			Logger:        log,
			MeterProvider: noop.NewMeterProvider(),
		}),
	)
	require.NoError(t, err)

	cmdTopic := namer.TopicForCommand(namer.CommandName(&testCommand{}))
	sub, err := realPub.Subscribe(ctx, cmdTopic)
	require.NoError(t, err)

	forwarderCtx, stopForwarder := context.WithCancel(ctx)
	var forwarderErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		forwarderErr = cmdBus.RunForwarder(forwarderCtx)
	}()

	err = cmdBus.Send(ctx, &testCommand{ID: "cmd-1"})
	require.NoError(t, err)

	select {
	case msg := <-sub:
		require.NotNil(t, msg)
		msg.Ack()
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for forwarded message")
	}

	stopForwarder()
	<-done
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = cmdBus.CloseForwarder(closeCtx)
	closeCancel()
	if err != nil && err != context.DeadlineExceeded {
		require.NoError(t, err)
	}
	_ = forwarderErr
}

func TestIntegration_EventBus_TxAwareOutbox_NoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, integrationGoleakOpts...)

	pool := setupPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wmLogger := watermill.NewStdLogger(false, false)
	schema := wmsql.DefaultPostgreSQLSchema{}
	pgxBeginner := wmsql.PgxBeginner{Conn: pool}

	sqlPub, err := wmsql.NewPublisher(pgxBeginner, wmsql.PublisherConfig{
		SchemaAdapter:        schema,
		AutoInitializeSchema: true,
	}, wmLogger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlPub.Close() })

	sqlSub, err := wmsql.NewSubscriber(
		wmsql.BeginnerFromPgx(pool),
		wmsql.SubscriberConfig{
			SchemaAdapter:    schema,
			OffsetsAdapter:   wmsql.DefaultPostgreSQLOffsetsAdapter{},
			InitializeSchema: true,
			ConsumerGroup:    "tx-outbox-init",
			PollInterval:     10 * time.Millisecond,
			AckDeadline:      ptrDuration(2 * time.Second),
		},
		wmLogger,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlSub.Close() })
	require.NoError(t, sqlSub.SubscribeInitialize(forwarderTopic))

	namer := message.NewShortlinkNamer(serviceName)
	marshaler := message.NewJSONMarshaler(namer)

	evtBus, err := bus.NewEventBusWithOptions(nil, marshaler, namer,
		bus.WithTxAwareOutbox(forwarderTopic, wmLogger),
	)
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	txCtx := uow.WithTx(ctx, tx)
	err = evtBus.Publish(txCtx, &testEvent{ID: "evt-1"})
	require.NoError(t, err)
	err = tx.Commit(ctx)
	require.NoError(t, err)
}

func TestIntegration_ForwarderClose_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t, integrationGoleakOpts...)

	pool := setupPostgres(t)
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx := context.Background()

	wmLogger := watermill.NewStdLogger(false, false)
	schema := wmsql.DefaultPostgreSQLSchema{}
	pgxBeginner := wmsql.PgxBeginner{Conn: pool}

	sqlPub, err := wmsql.NewPublisher(pgxBeginner, wmsql.PublisherConfig{
		SchemaAdapter:        schema,
		AutoInitializeSchema: true,
	}, wmLogger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlPub.Close() })

	sqlSub, err := wmsql.NewSubscriber(
		wmsql.BeginnerFromPgx(pool),
		wmsql.SubscriberConfig{
			SchemaAdapter:    schema,
			OffsetsAdapter:   wmsql.DefaultPostgreSQLOffsetsAdapter{},
			InitializeSchema: true,
			ConsumerGroup:    "leak-test",
			PollInterval:     50 * time.Millisecond,
			AckDeadline:      ptrDuration(2 * time.Second),
		},
		wmLogger,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlSub.Close() })

	realPub := gochannel.NewGoChannel(gochannel.Config{}, wmLogger)
	namer := message.NewShortlinkNamer(serviceName)
	marshaler := message.NewJSONMarshaler(namer)

	cfg := logger.Default()
	cfg.Writer = io.Discard
	cfg.Level = logger.WARN_LEVEL
	log, err := logger.New(cfg)
	require.NoError(t, err)

	cmdBus, err := bus.NewCommandBusWithOptions(sqlPub, marshaler, namer,
		bus.WithOutbox(bus.OutboxConfig{
			DB:            sqlDB,
			Subscriber:    sqlSub,
			RealPublisher: realPub,
			ForwarderName: forwarderTopic + "_leak",
			Logger:        log,
			MeterProvider: noop.NewMeterProvider(),
		}),
	)
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(ctx)
	go func() { _ = cmdBus.RunForwarder(runCtx) }()

	time.Sleep(200 * time.Millisecond)
	cancel()
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = cmdBus.CloseForwarder(closeCtx)
	closeCancel()
}

type testCommand struct {
	ID string `json:"id"`
}

type testEvent struct {
	ID string `json:"id"`
}

func ptrDuration(d time.Duration) *time.Duration { return &d }
