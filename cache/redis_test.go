//go:build unit || (database && redis)

package cache_test

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/shortlink-org/go-sdk/cache"
)

// unreachable points at a port nothing listens on: the commands below are
// expected to fail, and what is under test is that they are traced anyway.
const unreachable = "127.0.0.1:1"

func TestRedisClientTracing(t *testing.T) {
	t.Parallel()

	t.Run("traces commands when a tracer is given", func(t *testing.T) {
		t.Parallel()

		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

		//nolint:exhaustruct // only the address matters here
		client, err := cache.NewRedisClient(&redis.Options{Addr: unreachable}, cache.WithRedisTracer(provider))
		require.NoError(t, err)

		t.Cleanup(func() { _ = client.Close() })

		_, err = client.Get(t.Context(), "key")
		require.Error(t, err)

		require.NoError(t, provider.ForceFlush(t.Context()))
		require.NotEmpty(t, recorder.Ended())
	})

	t.Run("stays uninstrumented without a tracer", func(t *testing.T) {
		t.Parallel()

		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

		//nolint:exhaustruct // only the address matters here
		client, err := cache.NewRedisClient(&redis.Options{Addr: unreachable})
		require.NoError(t, err)

		t.Cleanup(func() { _ = client.Close() })

		_, err = client.Get(t.Context(), "key")
		require.Error(t, err)

		require.NoError(t, provider.ForceFlush(t.Context()))
		require.Empty(t, recorder.Ended())
	})
}
