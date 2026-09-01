//go:build unit

package watermill

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	wm "github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/stretchr/testify/require"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

const (
	orderTestInputTopic = "orders"
	orderTestDLQTopic   = "orders.dlq"
	orderTestMaxRetries = 3
	orderTestTimeout    = 5 * time.Second
	// How long a "nothing arrived" assertion waits. The retries it has to
	// outlast are single-digit milliseconds.
	orderTestQuiet = 250 * time.Millisecond
)

// memoryBackend is an in-process Backend: the same GoChannel both publishes
// and subscribes. Close is a no-op because the test owns the pubsub and
// drains the DLQ before tearing it down.
type memoryBackend struct {
	pubsub *gochannel.GoChannel
}

//nolint:ireturn // Backend is the SDK's own contract
func (b *memoryBackend) Publisher() message.Publisher { return b.pubsub }

//nolint:ireturn // Backend is the SDK's own contract
func (b *memoryBackend) Subscriber() message.Subscriber { return b.pubsub }

func (b *memoryBackend) Close() error { return nil }

func newOrderTestClient(t *testing.T, pubsub *gochannel.GoChannel, opts ...Option) *Client {
	t.Helper()

	cfg, err := config.New()
	require.NoError(t, err)

	log, err := logger.New(logger.Configuration{Writer: io.Discard, Level: logger.ERROR_LEVEL})
	require.NoError(t, err)

	base := []Option{
		DisableTimeout(),
		// The breaker would trip on the repeated failures these tests provoke
		// and start rejecting messages before the assertions run.
		DisableCircuitBreaker(),
		WithRetryOptions(RetryOptions{
			Enabled:         true,
			MaxRetries:      orderTestMaxRetries,
			InitialInterval: time.Millisecond,
			MaxInterval:     5 * time.Millisecond,
			Multiplier:      2,
		}),
	}

	client, err := New(
		context.Background(),
		log,
		cfg,
		&memoryBackend{pubsub: pubsub},
		metricnoop.NewMeterProvider(),
		tracenoop.NewTracerProvider(),
		append(base, opts...)...,
	)
	require.NoError(t, err)

	return client
}

func runOrderTestRouter(t *testing.T, ctx context.Context, client *Client) {
	t.Helper()

	go func() {
		_ = client.Router.Run(ctx) //nolint:errcheck // the router stops when the test cancels ctx
	}()

	select {
	case <-client.Router.Running():
	case <-time.After(orderTestTimeout):
		t.Fatal("router did not start")
	}
}

func requireNoDLQMessage(t *testing.T, dlq <-chan *message.Message) {
	t.Helper()

	select {
	case msg := <-dlq:
		t.Fatalf("unexpected dead letter: %s", string(msg.Payload))
	case <-time.After(orderTestQuiet):
	}
}

// TestPoisonQueueLeavesRetriesAlone pins the reason the poison middleware
// belongs outside retry. Underneath it, the poison queue publishes the dead
// letter and reports success, retry sees no error, and a failure that the
// second attempt would have absorbed is dead-lettered immediately.
func TestPoisonQueueLeavesRetriesAlone(t *testing.T) {
	t.Setenv("SERVICE_NAME", "orders-service")

	ctx := t.Context()

	pubsub := gochannel.NewGoChannel(gochannel.Config{}, wm.NopLogger{})
	defer pubsub.Close() //nolint:errcheck // teardown

	dlq, err := pubsub.Subscribe(ctx, orderTestDLQTopic)
	require.NoError(t, err)

	client := newOrderTestClient(t, pubsub, WithPoisonQueue(pubsub, orderTestDLQTopic))
	defer client.Close() //nolint:errcheck // teardown

	var attempts atomic.Int64

	processed := make(chan string, 1)

	client.Router.AddConsumerHandler(
		"orders-handler",
		orderTestInputTopic,
		client.Subscriber,
		func(msg *message.Message) error {
			if attempts.Add(1) == 1 {
				return errors.New("database timeout")
			}

			processed <- msg.UUID

			return nil
		},
	)

	runOrderTestRouter(t, ctx, client)

	require.NoError(t, pubsub.Publish(orderTestInputTopic, message.NewMessage("msg-1", []byte(`{"id":1}`))))

	select {
	case uuid := <-processed:
		require.Equal(t, "msg-1", uuid)
	case <-time.After(orderTestTimeout):
		t.Fatal("the message was never processed: the retry never happened")
	}

	require.EqualValues(t, 2, attempts.Load(), "the handler should have been called once more after the failure")

	requireNoDLQMessage(t, dlq)
}

// TestPoisonQueueDeadLettersOnceRetriesAreSpent covers the other half: a
// handler that always fails must dead-letter exactly once, after its retries,
// and must not wedge the queue behind it.
func TestPoisonQueueDeadLettersOnceRetriesAreSpent(t *testing.T) {
	t.Setenv("SERVICE_NAME", "orders-service")

	ctx := t.Context()

	pubsub := gochannel.NewGoChannel(gochannel.Config{}, wm.NopLogger{})
	defer pubsub.Close() //nolint:errcheck // teardown

	dlq, err := pubsub.Subscribe(ctx, orderTestDLQTopic)
	require.NoError(t, err)

	client := newOrderTestClient(t, pubsub, WithPoisonQueue(pubsub, orderTestDLQTopic))
	defer client.Close() //nolint:errcheck // teardown

	var poisonAttempts atomic.Int64

	processed := make(chan string, 1)

	client.Router.AddConsumerHandler(
		"orders-handler",
		orderTestInputTopic,
		client.Subscriber,
		func(msg *message.Message) error {
			if msg.UUID == "poison-1" {
				poisonAttempts.Add(1)

				return errors.New("always broken")
			}

			processed <- msg.UUID

			return nil
		},
	)

	runOrderTestRouter(t, ctx, client)

	require.NoError(t, pubsub.Publish(orderTestInputTopic, message.NewMessage("poison-1", []byte(`{"id":1}`))))

	select {
	case deadLetter := <-dlq:
		deadLetter.Ack()

		require.Equal(t, "always broken", deadLetter.Metadata.Get("poison_reason"))
		require.Equal(t, "orders-service", deadLetter.Metadata.Get("service_name"))
	case <-time.After(orderTestTimeout):
		t.Fatal("the always-failing message never reached the DLQ")
	}

	require.EqualValues(t, orderTestMaxRetries+1, poisonAttempts.Load(),
		"the message should be dead-lettered only after every retry is spent")

	// The queue keeps moving: without the poison queue the always-failing
	// message would be redelivered forever and block everything behind it.
	require.NoError(t, pubsub.Publish(orderTestInputTopic, message.NewMessage("good-1", []byte(`{"id":2}`))))

	select {
	case uuid := <-processed:
		require.Equal(t, "good-1", uuid)
	case <-time.After(orderTestTimeout):
		t.Fatal("the queue stalled behind the dead-lettered message")
	}

	// Exactly one dead letter, not one per attempt.
	requireNoDLQMessage(t, dlq)
}
