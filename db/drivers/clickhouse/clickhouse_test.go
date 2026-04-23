//go:build unit || (database && clickhouse)

package clickhouse

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
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestClickHouse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	c, err := testcontainers.Run(ctx, "clickhouse/clickhouse-server:latest",
		testcontainers.WithExposedPorts("9000/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("9000/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)

	t.Setenv("STORE_CLICKHOUSE_URI", fmt.Sprintf("clickhouse://%s:%s/default?sslmode=disable", host, mapped.Port()))
	require.NoError(t, store.Init(ctx))

	t.Run("Close", func(t *testing.T) {
		cancel()
	})
}
