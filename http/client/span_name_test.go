package http_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TestClientSpanName checks the client span is named after the method alone,
// as the HTTP semantic conventions require, rather than otelhttp's "HTTP GET".
func TestClientSpanName(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	previous := otel.GetTracerProvider()

	otel.SetTracerProvider(provider)

	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client, err := New()
	require.NoError(t, err)

	for _, method := range []string{http.MethodGet, http.MethodPost, "PURGE"} {
		request, err := http.NewRequestWithContext(t.Context(), method, server.URL+"/users/1", http.NoBody)
		require.NoError(t, err)

		response, err := client.Do(request)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
	}

	// The client stack opens internal spans of its own, such as the rate
	// limiter wait; only the otelhttp client span is under test here.
	var names []string

	for _, span := range recorder.Ended() {
		if span.SpanKind() == trace.SpanKindClient {
			names = append(names, span.Name())
		}
	}

	require.Equal(t, []string{http.MethodGet, http.MethodPost, "HTTP"}, names)
}
