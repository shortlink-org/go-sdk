//go:build unit || (database && postgres)

package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestPostgres(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(trace.NewNoopTracerProvider(), metric.NewMeterProvider(), cfg)

	pgContainer, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("shortlink"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("shortlink"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, pgContainer)
	require.NoError(t, err)

	t.Cleanup(cancel)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	t.Setenv("STORE_POSTGRES_URI", connStr)

	require.NoError(t, store.Init(ctx))
}
