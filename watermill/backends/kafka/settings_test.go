package kafka

import (
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/config"
)

func TestNewKafkaConfigDefaults(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Set("SERVICE_NAME", "svc-default")

	kcfg, err := newKafkaConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, []string{"localhost:9092"}, kcfg.brokers)
	assert.Equal(t, "svc-default", kcfg.consumerGroup)
	assert.Equal(t, "svc-default", kcfg.clientID)
	assert.True(t, kcfg.enableOTEL)
	assert.Equal(t, sarama.OffsetNewest, kcfg.initialOffset)
	assert.Equal(t, sarama.RangeBalanceStrategyName, kcfg.rebalanceStrategy.Name())
	assert.Equal(t, 100*time.Millisecond, kcfg.nackSleep)
	assert.Equal(t, time.Second, kcfg.reconnectSleep)
	assert.Equal(t, 10*time.Second, kcfg.waitForTopicTimeout)
	assert.False(t, kcfg.skipTopicInitialization)
	assert.Equal(t, sarama.MaxVersion, kcfg.version)
	assert.Equal(t, 10, kcfg.producerRetryMax)
	assert.Equal(t, sarama.CompressionSnappy, kcfg.compression)
	assert.True(t, kcfg.idempotentProducer)
}

func TestNewKafkaConfigOverrides(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Set("SERVICE_NAME", "ignored-by-override")

	cfg.Set("WATERMILL_KAFKA_BROKERS", []string{"broker1:9092", "broker2:9092"})
	cfg.Set("WATERMILL_KAFKA_CONSUMER_GROUP", "custom-group")
	cfg.Set("WATERMILL_KAFKA_CLIENT_ID", "custom-client")
	cfg.Set("WATERMILL_KAFKA_CONSUMER_INITIAL_OFFSET", "oldest")
	cfg.Set("WATERMILL_KAFKA_REBALANCE_STRATEGY", "roundrobin")
	cfg.Set("WATERMILL_KAFKA_SARAMA_VERSION", "2.5.0")
	cfg.Set("WATERMILL_KAFKA_PRODUCER_COMPRESSION", "gzip")
	cfg.Set("WATERMILL_KAFKA_PRODUCER_RETRY_MAX", 42)
	cfg.Set("WATERMILL_KAFKA_PRODUCER_IDEMPOTENT", false)
	cfg.Set("WATERMILL_KAFKA_OTEL_ENABLED", false)
	cfg.Set("WATERMILL_KAFKA_SUBSCRIBER_NACK_SLEEP", 250*time.Millisecond)
	cfg.Set("WATERMILL_KAFKA_SUBSCRIBER_RECONNECT_SLEEP", 2*time.Second)
	cfg.Set("WATERMILL_KAFKA_WAIT_FOR_TOPIC_TIMEOUT", 30*time.Second)
	cfg.Set("WATERMILL_KAFKA_SKIP_TOPIC_INIT", true)

	kcfg, err := newKafkaConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, kcfg.brokers)
	assert.Equal(t, "custom-group", kcfg.consumerGroup)
	assert.Equal(t, "custom-client", kcfg.clientID)
	assert.False(t, kcfg.enableOTEL)
	assert.Equal(t, sarama.OffsetOldest, kcfg.initialOffset)
	assert.Equal(t, sarama.RoundRobinBalanceStrategyName, kcfg.rebalanceStrategy.Name())
	expectedVersion, _ := sarama.ParseKafkaVersion("2.5.0")
	assert.Equal(t, expectedVersion, kcfg.version)
	assert.Equal(t, sarama.CompressionGZIP, kcfg.compression)
	assert.Equal(t, 42, kcfg.producerRetryMax)
	assert.False(t, kcfg.idempotentProducer)
	assert.Equal(t, 250*time.Millisecond, kcfg.nackSleep)
	assert.Equal(t, 2*time.Second, kcfg.reconnectSleep)
	assert.Equal(t, 30*time.Second, kcfg.waitForTopicTimeout)
	assert.True(t, kcfg.skipTopicInitialization)
}

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.New()
	require.NoError(t, err)

	cfg.Reset()

	return cfg
}
