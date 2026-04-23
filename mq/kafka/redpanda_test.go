//go:build unit || (mq && kafka)

package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/mq/query"
)

func TestRedPanda(t *testing.T) {
	cfg, err := config.New()
	require.NoError(t, err)
	cfg.SetDefault("SERVICE_NAME", "shortlink")
	cfg.Set("MQ_KAFKA_SARAMA_VERSION", "DEFAULT")

	ctx, cancel := context.WithCancel(context.Background())
	mq := New(cfg)

	log, err := logger.New(logger.Configuration{})
	require.NoError(t, err, "Cannot create logger")

	rp, err := tcredpanda.Run(ctx,
		"docker.redpanda.com/redpandadata/redpanda:v23.3.18",
		tcredpanda.WithAutoCreateTopics(),
	)
	testcontainers.CleanupContainer(t, rp)
	require.NoError(t, err)

	t.Cleanup(cancel)

	broker, err := rp.KafkaSeedBroker(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		cfg.Set("MQ_KAFKA_URI", broker)
		return mq.Init(ctx, log) == nil
	}, 3*time.Minute, time.Second, "redpanda init")

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
