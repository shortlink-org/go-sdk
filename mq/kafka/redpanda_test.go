//go:build unit || (mq && kafka)

package kafka

import (
	"context"
	"net/netip"
	"testing"
	"time"

	tccontainer "github.com/moby/moby/api/types/container"
	dockernetwork "github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

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

	nw, err := network.New(ctx)
	require.NoError(t, err)

	redpanda, err := testcontainers.Run(ctx, "docker.redpanda.com/vectorized/redpanda:v23.2.14",
		network.WithNetwork([]string{"redpanda"}, nw),
		testcontainers.WithCmd(
			"redpanda", "start",
			"--smp", "1",
			"--overprovisioned",
			"--reserve-memory", "0M",
			"--node-id", "0",
			"--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
			"--advertise-kafka-addr", "internal://redpanda:9092,external://localhost:19092",
			"--pandaproxy-addr", "internal://0.0.0.0:8082,external://0.0.0.0:18082",
			"--advertise-pandaproxy-addr", "internal://redpanda:8082,external://localhost:18082",
		),
		testcontainers.WithHostConfigModifier(func(hc *tccontainer.HostConfig) {
			hc.PortBindings = dockernetwork.PortMap{
				dockernetwork.MustParsePort("19092/tcp"): []dockernetwork.PortBinding{{HostIP: netip.IPv4Unspecified(), HostPort: "19092"}},
			}
		}),
		testcontainers.WithExposedPorts("19092/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("19092/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		_ = nw.Remove(context.Background())
	}
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = redpanda.Terminate(context.Background())
		_ = nw.Remove(context.Background())
	})

	require.Eventually(t, func() bool {
		cfg.Set("MQ_KAFKA_URI", "localhost:19092")
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
