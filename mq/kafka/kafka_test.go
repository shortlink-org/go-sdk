//go:build unit || (mq && kafka)

package kafka

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"

	tccontainer "github.com/moby/moby/api/types/container"
	dockernetwork "github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
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

	nw, err := network.New(ctx)
	require.NoError(t, err)

	zk, err := testcontainers.Run(ctx, "confluentinc/cp-zookeeper:7.5.3",
		network.WithNetwork([]string{"test-kafka-zookeeper"}, nw),
		testcontainers.WithEnv(map[string]string{
			"ZOOKEEPER_CLIENT_PORT": "2181",
			"ZOOKEEPER_TICK_TIME":   "2000",
		}),
		testcontainers.WithExposedPorts("2181/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("2181/tcp").WithStartupTimeout(2*time.Minute),
		),
	)
	require.NoError(t, err)

	kafkaC, err := testcontainers.Run(ctx, "confluentinc/cp-kafka:7.5.3",
		network.WithNetwork([]string{"kafka"}, nw),
		testcontainers.WithConfigModifier(func(c *tccontainer.Config) {
			c.Hostname = "kafka"
		}),
		testcontainers.WithHostConfigModifier(func(hc *tccontainer.HostConfig) {
			hc.PortBindings = dockernetwork.PortMap{
				dockernetwork.MustParsePort("19093/tcp"): []dockernetwork.PortBinding{{HostIP: netip.IPv4Unspecified(), HostPort: "19093"}},
			}
		}),
		testcontainers.WithEnv(map[string]string{
			"KAFKA_BROKER_ID":                        "1",
			"KAFKA_ZOOKEEPER_CONNECT":                "test-kafka-zookeeper:2181",
			"KAFKA_ADVERTISED_LISTENERS":             "INSIDE://kafka:9092,OUTSIDE://localhost:19093",
			"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":   "INSIDE:PLAINTEXT,OUTSIDE:PLAINTEXT",
			"KAFKA_LISTENERS":                        "INSIDE://0.0.0.0:9092,OUTSIDE://0.0.0.0:19093",
			"KAFKA_INTER_BROKER_LISTENER_NAME":       "INSIDE",
			"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
		}),
		testcontainers.WithExposedPorts("19093/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("19093/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		_ = zk.Terminate(context.Background())
		_ = nw.Remove(context.Background())
	}
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = kafkaC.Terminate(context.Background())
		_ = zk.Terminate(context.Background())
		_ = nw.Remove(context.Background())
	})

	require.Eventually(t, func() bool {
		cfg.Set("MQ_KAFKA_URI", "localhost:19093")
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
