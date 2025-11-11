package span_middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSpanMiddlewareEnrichesTrace(t *testing.T) {
	t.Parallel()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
	)
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	})

	router := chi.NewRouter()
	router.Use(Span())
	router.Get("/v1/resource/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	handler := otelhttp.NewHandler(router, "test", otelhttp.WithTracerProvider(tp))

	req := httptest.NewRequest(http.MethodGet, "/v1/resource/123", http.NoBody)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	span := spans[0]
	require.Equal(t, "GET /v1/resource/{id}", span.Name)

	var hasRouteAttr bool
	for _, attr := range span.Attributes {
		if attr.Key == "http.route" && attr.Value.AsString() == "/v1/resource/{id}" {
			hasRouteAttr = true
			break
		}
	}
	require.True(t, hasRouteAttr, "expected http.route attribute with template value")

	traceID := rec.Header().Get(TraceIDHeader)
	require.NotEmpty(t, traceID)
	require.Equal(t, span.SpanContext.TraceID().String(), traceID)
}
