//go:build unit || (database && neo4j)

package neo4j

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

func TestNeo4j(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	c, err := testcontainers.Run(ctx, "neo4j:4.0.3",
		testcontainers.WithExposedPorts("7687/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("7687/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "7687/tcp")
	require.NoError(t, err)

	t.Setenv("STORE_NEO4J_URI", fmt.Sprintf("neo4j://%s:%s", host, mapped.Port()))
	require.NoError(t, store.Init(ctx))
}
