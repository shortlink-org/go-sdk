//go:build unit || (database && couchbase)

package couchbase

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

func TestCouchbase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	c, err := testcontainers.Run(ctx, "couchbase:7.2.3",
		testcontainers.WithExposedPorts("8092/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("8092/tcp").WithStartupTimeout(5*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "8092/tcp")
	require.NoError(t, err)

	t.Setenv("STORE_COUCHBASE_URI", fmt.Sprintf("couchbase://%s:%s", host, mapped.Port()))
	require.NoError(t, store.Init(ctx))
}
