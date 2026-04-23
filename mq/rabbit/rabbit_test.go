//go:build unit || (mq && rabbitmq)

package rabbit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/mq/query"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestRabbitMQ(t *testing.T) {
	cfg, err := config.New()
	require.NoError(t, err)
	cfg.SetDefault("SERVICE_NAME", "shortlink")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	log, err := logger.New(logger.Configuration{})
	require.NoError(t, err)

	ctr, err := tcrabbit.Run(ctx, "rabbitmq:3.13-management-alpine")
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err, "rabbitmq container: ensure Docker is running")

	amqpURI, err := ctr.AmqpURL(ctx)
	require.NoError(t, err)

	cfg.Set("MQ_RABBIT_URI", amqpURI)

	mq := New(log, cfg)
	require.NoError(t, mq.Init(ctx, log))

	t.Run("Subscribe", func(t *testing.T) {
		respCh := make(chan query.ResponseMessage, 1)
		msg := query.Response{Chan: respCh}

		err := mq.Subscribe(ctx, "test", msg)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return mq.Publish(ctx, "test", []byte("rk"), []byte("test")) == nil
		}, 30*time.Second, 200*time.Millisecond, "publish to rabbit")

		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for message")
		case resp := <-respCh:
			require.Equal(t, []byte("test"), resp.Body)
		}

		require.NoError(t, mq.UnSubscribe("test"))
	})
}
