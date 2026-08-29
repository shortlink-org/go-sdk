package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
)

// newTestServer builds a server around a handler that answers 200, and returns
// the handler the server ended up with.
func newTestServer(t *testing.T, opts ...Option) http.Handler {
	t.Helper()

	cfg, err := config.New()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})

	server := New(t.Context(), handler, Config{Port: 0, Timeout: time.Second}, cfg, opts...)

	return server.Handler
}

func TestServerTracing(t *testing.T) {
	t.Parallel()

	t.Run("records a server span when a tracer is given", func(t *testing.T) {
		t.Parallel()

		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

		handler := newTestServer(t, WithTracer(provider))

		writer := httptest.NewRecorder()
		handler.ServeHTTP(writer, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody))

		require.NoError(t, provider.ForceFlush(context.Background()))

		spans := recorder.Ended()
		require.Len(t, spans, 1)
		require.Equal(t, trace.SpanKindServer, spans[0].SpanKind())
		require.Equal(t, http.MethodGet, spans[0].Name())
	})

	t.Run("stays uninstrumented without a tracer", func(t *testing.T) {
		t.Parallel()

		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

		handler := newTestServer(t)

		writer := httptest.NewRecorder()
		handler.ServeHTTP(writer, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody))

		require.NoError(t, provider.ForceFlush(context.Background()))
		require.Empty(t, recorder.Ended())
	})

	t.Run("a nil provider is the same as no option", func(t *testing.T) {
		t.Parallel()

		handler := newTestServer(t, WithTracer(nil))

		writer := httptest.NewRecorder()
		handler.ServeHTTP(writer, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody))

		require.Equal(t, http.StatusOK, writer.Code)
	})
}
