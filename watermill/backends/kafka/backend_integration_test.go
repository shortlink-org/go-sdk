package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/require"
	tc_kafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

func TestBackendPublishSubscribeWithTestcontainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	kafkaContainer, err := tc_kafka.Run(ctx, "confluentinc/confluent-local:7.5.0")
	if err != nil {
		t.Skipf("kafka container not available: %v", err)
	}

	t.Cleanup(func() {
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = kafkaContainer.Terminate(context.Background())
	})

	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err)

	cfg := newTestConfig(t)
	cfg.Set("SERVICE_NAME", "watermill-kafka-int")
	cfg.Set("WATERMILL_KAFKA_BROKERS", brokers)
	cfg.Set("WATERMILL_KAFKA_SARAMA_VERSION", "default")

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	backend, err := New(ctx, log, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = backend.Close()
	})

	topic := fmt.Sprintf("watermill-int-%d", time.Now().UnixNano())

	messages, err := backend.Subscriber().Subscribe(ctx, topic)
	require.NoError(t, err)

	msg := message.NewMessage(watermill.NewUUID(), []byte("payload"))
	require.NoError(t, backend.Publisher().Publish(topic, msg))

	select {
	case received := <-messages:
		require.Equal(t, msg.UUID, received.UUID)
		received.Ack()
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for kafka message")
	}
}
