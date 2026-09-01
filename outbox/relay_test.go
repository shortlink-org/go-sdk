//go:build unit || (database && postgres)

package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/outbox"
)

// newRelay wires a relay onto a router carrying the SDK middleware stack, and
// returns the relay together with the dead letter channel.
func newRelay(t *testing.T, pool *pgxpool.Pool, opts ...outbox.Option) (*outbox.Relay, <-chan *message.Message) {
	t.Helper()

	pubsub := gochannel.NewGoChannel(gochannel.Config{
		OutputChannelBuffer:            0,
		Persistent:                     false,
		BlockPublishUntilSubscriberAck: false,
		PreserveContext:                false,
	}, nil)

	t.Cleanup(func() {
		_ = pubsub.Close() //nolint:errcheck // teardown
	})

	dlq, err := pubsub.Subscribe(context.Background(), testDLQTopic)
	require.NoError(t, err)

	client := newRouter(t, pubsub)

	relay, err := outbox.NewRelay(
		&testStore{pool: pool},
		newTestLogger(t),
		client.Router,
		append([]outbox.Option{
			outbox.WithPollInterval(20 * time.Millisecond),
			outbox.WithBatchSize(16),
		}, opts...)...,
	)
	require.NoError(t, err)

	return relay, dlq
}

// TestRelayRetriesBeforeDeadLettering is the reason the poison queue has to
// wrap retry rather than sit under it: a handler that fails once must not cost
// a message.
func TestRelayRetriesBeforeDeadLettering(t *testing.T) {
	pool := setupPostgres(t)
	relay, dlq := newRelay(t, pool)

	var attempts atomic.Int64

	delivered := make(chan string, 1)

	require.NoError(t, relay.Handle(testTopic, func(_ context.Context, msg *message.Message) error {
		if attempts.Add(1) == 1 {
			return errors.New("database timeout")
		}

		delivered <- msg.UUID

		return nil
	}))

	runRelay(t, relay)
	publish(t, pool, testTopic, message.NewMessage("msg-1", []byte(`{"id":1}`)))

	select {
	case uuid := <-delivered:
		require.Equal(t, "msg-1", uuid)
	case <-time.After(testTimeout):
		t.Fatal("the message was never delivered: the retry never happened")
	}

	require.EqualValues(t, 2, attempts.Load())

	requireNoMessage(t, dlq, "a message that succeeded on retry was dead-lettered")

	requireEventually(t, func() bool {
		return countRows(t, pool, "delivered_at IS NULL") == 0
	}, "the delivered message was never marked")
}

// TestRelayDeadLettersOnceRetriesAreSpent covers the other half: a message
// that always fails goes to the DLQ exactly once and does not wedge the topic
// behind it.
func TestRelayDeadLettersOnceRetriesAreSpent(t *testing.T) {
	pool := setupPostgres(t)
	relay, dlq := newRelay(t, pool)

	var poisonAttempts atomic.Int64

	delivered := make(chan string, 1)

	require.NoError(t, relay.Handle(testTopic, func(_ context.Context, msg *message.Message) error {
		if msg.UUID == "poison-1" {
			poisonAttempts.Add(1)

			return errors.New("always broken")
		}

		delivered <- msg.UUID

		return nil
	}))

	runRelay(t, relay)
	publish(t, pool, testTopic, message.NewMessage("poison-1", []byte(`{"id":1}`)))

	select {
	case deadLetter := <-dlq:
		deadLetter.Ack()
		require.Equal(t, "always broken", deadLetter.Metadata.Get("poison_reason"))
	case <-time.After(testTimeout):
		t.Fatal("the always-failing message never reached the DLQ")
	}

	require.EqualValues(t, testMaxRetries+1, poisonAttempts.Load(),
		"the message should be dead-lettered only after every retry is spent")

	// The topic keeps moving: the dead-lettered row is acknowledged, so it no
	// longer takes a slot in every batch.
	publish(t, pool, testTopic, message.NewMessage("good-1", []byte(`{"id":2}`)))

	select {
	case uuid := <-delivered:
		require.Equal(t, "good-1", uuid)
	case <-time.After(testTimeout):
		t.Fatal("the topic stalled behind the dead-lettered message")
	}

	requireNoMessage(t, dlq, "the message was dead-lettered more than once")
}

// TestTwoRelaysShareOneCursor is the test that cannot be written by eye, and
// the reason this belongs in the SDK rather than in every service: two relays
// on one table must not both deliver the same message.
func TestTwoRelaysShareOneCursor(t *testing.T) {
	const (
		messages  = 60
		relays    = 2
		batchSize = 4
	)

	pool := setupPostgres(t)

	var (
		mu         sync.Mutex
		deliveries = map[string]int{}
	)

	done := make(chan struct{})

	record := func(_ context.Context, msg *message.Message) error {
		mu.Lock()
		defer mu.Unlock()

		deliveries[msg.UUID]++

		if len(deliveries) == messages {
			select {
			case <-done:
			default:
				close(done)
			}
		}

		return nil
	}

	for range relays {
		relay, _ := newRelay(t, pool, outbox.WithBatchSize(batchSize))
		require.NoError(t, relay.Handle(testTopic, record))
		runRelay(t, relay)
	}

	batch := make([]*message.Message, 0, messages)
	for i := range messages {
		batch = append(batch, message.NewMessage(uuidFor(i), []byte(`{"id":1}`)))
	}

	publish(t, pool, testTopic, batch...)

	select {
	case <-done:
	case <-time.After(testTimeout):
		mu.Lock()
		got := len(deliveries)
		mu.Unlock()
		t.Fatalf("only %d of %d messages were delivered", got, messages)
	}

	// Give a duplicate a chance to show up before asserting there is none.
	time.Sleep(testQuiet)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, deliveries, messages)

	for uuid, count := range deliveries {
		require.Equal(t, 1, count, "message %s was delivered more than once", uuid)
	}
}

// TestUnhandledTopicIsLeftAlone covers a message written by a newer build,
// carrying a topic this build knows nothing about. It must not block the
// topics that do have handlers, and it must not be consumed by them either:
// the row waits for the consumer that will be deployed later.
func TestUnhandledTopicIsLeftAlone(t *testing.T) {
	pool := setupPostgres(t)
	relay, dlq := newRelay(t, pool)

	delivered := make(chan string, 1)

	require.NoError(t, relay.Handle(testTopic, func(_ context.Context, msg *message.Message) error {
		delivered <- msg.UUID

		return nil
	}))

	runRelay(t, relay)

	publish(t, pool, "orders.from.the.future", message.NewMessage("unknown-1", []byte(`{"id":1}`)))
	publish(t, pool, testTopic, message.NewMessage("known-1", []byte(`{"id":2}`)))

	select {
	case uuid := <-delivered:
		require.Equal(t, "known-1", uuid)
	case <-time.After(testTimeout):
		t.Fatal("the queue stalled behind a message nobody handles")
	}

	requireNoMessage(t, dlq, "an unhandled topic was dead-lettered")

	require.Equal(t, 1, countRows(t, pool, "delivered_at IS NULL AND topic = $1", "orders.from.the.future"),
		"the unhandled message must still be waiting, not consumed and not lost")
}

// TestReaperRemovesDeliveredMessages keeps the outbox from becoming an
// append-only log of everything the service ever emitted.
func TestReaperRemovesDeliveredMessages(t *testing.T) {
	pool := setupPostgres(t)
	relay, _ := newRelay(t, pool,
		outbox.WithRetention(time.Millisecond),
		outbox.WithReapInterval(50*time.Millisecond),
	)

	delivered := make(chan string, 1)

	require.NoError(t, relay.Handle(testTopic, func(_ context.Context, msg *message.Message) error {
		delivered <- msg.UUID

		return nil
	}))

	runRelay(t, relay)
	publish(t, pool, testTopic, message.NewMessage("reap-1", []byte(`{"id":1}`)))

	select {
	case <-delivered:
	case <-time.After(testTimeout):
		t.Fatal("the message was never delivered")
	}

	requireEventually(t, func() bool {
		return countRows(t, pool, "TRUE") == 0
	}, "the delivered row was never removed")
}

// TestRelayRejectsASecondHandlerForOneTopic: two handlers on one topic would
// split its messages rather than each seeing all of them, which is never what
// the caller meant.
func TestRelayRejectsASecondHandlerForOneTopic(t *testing.T) {
	pool := setupPostgres(t)
	relay, _ := newRelay(t, pool)

	noop := func(context.Context, *message.Message) error { return nil }

	require.NoError(t, relay.Handle(testTopic, noop))

	var duplicate *outbox.DuplicateTopicError

	require.ErrorAs(t, relay.Handle(testTopic, noop), &duplicate)
	require.Equal(t, testTopic, duplicate.Topic)

	// Run it: closing a router that never ran costs its whole close timeout.
	runRelay(t, relay)
}

func requireEventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal(msg)
}

func uuidFor(i int) string {
	return fmt.Sprintf("msg-%03d", i)
}
