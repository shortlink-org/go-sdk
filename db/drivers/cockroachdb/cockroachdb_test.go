//go:build unit || (database && cockroachdb)

package cockroachdb

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

func TestCockroachDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(cfg)

	c, err := testcontainers.Run(ctx, "cockroachdb/cockroach:v23.1.3",
		testcontainers.WithEnv(map[string]string{
			"COCKROACH_PASSWORD": "password",
			"COCKROACH_DATABASE": "shortlink",
		}),
		testcontainers.WithCmd("start-single-node", "--insecure"),
		testcontainers.WithExposedPorts("26257/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("26257/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "26257/tcp")
	require.NoError(t, err)

	t.Setenv("STORE_COCKROACHDB_URI", fmt.Sprintf("postgresql://root:password@%s:%s/shortlink?sslmode=disable", host, mapped.Port()))
	require.NoError(t, store.Init(ctx))
}
