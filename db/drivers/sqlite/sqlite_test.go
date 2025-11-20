//go:build unit || (database && sqlite)

package sqlite

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestSQLite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg, err := config.New()
	require.NoError(t, err)
	store := New(trace.NewNoopTracerProvider(), metric.NewMeterProvider(), cfg)

	err = store.Init(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()
	})
}
