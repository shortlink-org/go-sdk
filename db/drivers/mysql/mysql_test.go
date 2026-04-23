//go:build unit || (database && mysql)

package mysql

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestMySQL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(trace.NewNoopTracerProvider(), metric.NewMeterProvider(), cfg)

	c, err := testcontainers.Run(ctx, "mysql:latest",
		testcontainers.WithEnv(map[string]string{"MYSQL_ROOT_PASSWORD": "secret"}),
		testcontainers.WithExposedPorts("3306/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("3306/tcp").WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "3306/tcp")
	require.NoError(t, err)

	t.Setenv("STORE_MYSQL_URI", fmt.Sprintf("root:secret@(%s:%s)/mysql?parseTime=true", host, mapped.Port()))
	require.NoError(t, store.Init(ctx))
}
