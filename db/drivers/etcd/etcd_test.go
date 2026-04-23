//go:build unit || (database && etcd)

package etcd

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

func TestETCD(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	c, err := testcontainers.Run(ctx, "docker.io/bitnami/etcd:3",
		testcontainers.WithExposedPorts("2379/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("2379/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "2379/tcp")
	require.NoError(t, err)

	cfg.Set("STORE_ETCD_URI", fmt.Sprintf("%s:%s", host, mapped.Port()))
	require.NoError(t, store.Init(ctx))
}
