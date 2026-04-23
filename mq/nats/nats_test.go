//go:build unit || (mq && nats)

package nats

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/mq/query"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestNATS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	mq := New(cfg)

	c, err := testcontainers.Run(ctx, "nats:2.10-alpine",
		testcontainers.WithExposedPorts("4222/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("4222/tcp").WithStartupTimeout(2*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "4222/tcp")
	require.NoError(t, err)

	t.Setenv("MQ_NATS_URI", fmt.Sprintf("nats://%s:%s", host, mapped.Port()))
	require.NoError(t, mq.Init(ctx, nil))

	t.Run("Subscribe", func(t *testing.T) {
		respCh := make(chan query.ResponseMessage)
		msg := query.Response{
			Chan: respCh,
		}

		err := mq.Subscribe(ctx, "test", msg)
		require.Nil(t, err, "Cannot subscribe")

		err = mq.Publish(ctx, "", []byte("test"), []byte("test"))
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
