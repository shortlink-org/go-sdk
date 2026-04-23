//go:build unit || (database && dgraph)

package dgraph

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("google.golang.org/grpc/internal/grpcsync.(*CallbackSerializer).run"),
		goleak.IgnoreTopFunction("google.golang.org/grpc.(*addrConn).resetTransport"))

	os.Exit(m.Run())
}

func TestDgraph(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)

	logConf := logger.Configuration{
		Level: logger.INFO_LEVEL,
	}
	log, err := logger.New(logConf)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = log.Close()
	})
	store := New(log, cfg)

	nw, err := network.New(ctx)
	require.NoError(t, err)

	zero, err := testcontainers.Run(ctx, "dgraph/dgraph:v21.03.0",
		network.WithNetwork([]string{"test-dgraph-zero"}, nw),
		testcontainers.WithCmd("dgraph", "zero", "--my=test-dgraph-zero:5080"),
		testcontainers.WithExposedPorts("5080/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5080/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	alpha, err := testcontainers.Run(ctx, "dgraph/dgraph:v21.03.0",
		network.WithNetwork([]string{}, nw),
		testcontainers.WithCmd("dgraph", "alpha", "--my=localhost:7080", "--lru_mb=2048", "--zero=test-dgraph-zero:5080"),
		testcontainers.WithExposedPorts("9080/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("9080/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		_ = zero.Terminate(context.Background())
		_ = nw.Remove(context.Background())
	}
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = alpha.Terminate(context.Background())
		_ = zero.Terminate(context.Background())
		_ = nw.Remove(context.Background())
	})

	host, err := alpha.Host(ctx)
	require.NoError(t, err)
	mapped, err := alpha.MappedPort(ctx, "9080/tcp")
	require.NoError(t, err)

	t.Setenv("STORE_DGRAPH_URI", fmt.Sprintf("%s:%s", host, mapped.Port()))

	require.Eventually(t, func() bool {
		return store.Init(ctx) == nil
	}, 2*time.Minute, time.Second, "dgraph init")
}
