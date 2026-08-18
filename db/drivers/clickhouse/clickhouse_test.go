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

// The entrypoint disables network access for the default user unless
// credentials are supplied, so they are passed explicitly below.
const (
	clickhouseImage    = "clickhouse/clickhouse-server:latest"
	clickhouseUser     = "default"
	clickhousePassword = "clickhouse"
	clickhouseDB       = "default"
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

	c, err := testcontainers.Run(ctx, clickhouseImage,
		testcontainers.WithExposedPorts("9000/tcp", "8123/tcp"),
		testcontainers.WithEnv(map[string]string{
			"CLICKHOUSE_USER":     clickhouseUser,
			"CLICKHOUSE_PASSWORD": clickhousePassword,
			"CLICKHOUSE_DB":       clickhouseDB,
		}),
		// /ping answers without credentials and only once the server is
		// actually serving, unlike a bare port check.
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/ping").
				WithPort("8123/tcp").
				WithStartupTimeout(3*time.Minute),
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

	t.Setenv("STORE_CLICKHOUSE_URI", fmt.Sprintf(
		"clickhouse://%s:%s@%s:%s/%s?sslmode=disable",
		clickhouseUser, clickhousePassword, host, mapped.Port(), clickhouseDB,
	))
	require.NoError(t, store.Init(ctx))

	t.Run("Close", func(t *testing.T) {
		cancel()
	})
}
