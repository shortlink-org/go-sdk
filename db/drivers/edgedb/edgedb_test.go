//go:build unit || (database && edgedb)

package edgedb

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

func TestEdgeDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	c, err := testcontainers.Run(ctx, "edgedb/edgedb:4",
		testcontainers.WithEnv(map[string]string{
			"EDGEDB_SERVER_SECURITY": "insecure_dev_mode",
			"EDGEDB_SERVER_DATABASE": "shortlink",
		}),
		testcontainers.WithExposedPorts("5656/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5656/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "5656/tcp")
	require.NoError(t, err)

	cfg.Set("STORE_EDGEDB_URI", fmt.Sprintf("edgedb://%s:%s", host, mapped.Port()))
	require.NoError(t, store.Init(ctx))
}
