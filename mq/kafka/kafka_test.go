//go:build unit || (mq && kafka)

package kafka

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/mq/query"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("github.com/rcrowley/go-metrics.(*meterArbiter).tick"))

	os.Exit(m.Run())
}

func TestKafka(t *testing.T) {
	cfg, err := config.New()
	require.NoError(t, err)
	cfg.SetDefault("SERVICE_NAME", "shortlink")

	ctx, cancel := context.WithCancel(context.Background())
	mq := New(cfg)

	log, err := logger.New(logger.Configuration{})
	require.NoError(t, err, "Cannot create logger")

	kafkaC, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.5.0",
		tckafka.WithClusterID("go-sdk-test-kraft"),
	)
	testcontainers.CleanupContainer(t, kafkaC)
	require.NoError(t, err)

	t.Cleanup(cancel)

	brokers, err := kafkaC.Brokers(ctx)
	require.NoError(t, err)
	brokerURI := strings.Join(brokers, ",")

	require.Eventually(t, func() bool {
		cfg.Set("MQ_KAFKA_URI", brokerURI)
		return mq.Init(ctx, log) == nil
	}, 3*time.Minute, time.Second, "kafka init")

	t.Run("Subscribe", func(t *testing.T) {
		respCh := make(chan query.ResponseMessage)
		msg := query.Response{
			Chan: respCh,
		}

		err := mq.Subscribe(ctx, "test", msg)
		require.Nil(t, err, "Cannot subscribe")

		err = mq.Publish(ctx, "test", []byte("test"), []byte("test"))
		require.Nil(t, err, "Cannot publish")

		select {
		case <-ctx.Done():
			t.Fatal("Timeout")
		case resp := <-respCh:
			require.Equal(t, []byte("test"), resp.Body, "Payloads are not equal")
		}

		err = mq.UnSubscribe("test")
		require.Nil(t, err, "Cannot unsubscribe")
	})
}
