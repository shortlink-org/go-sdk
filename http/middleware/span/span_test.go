package span_middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestSpanMiddleware(t *testing.T) {
	tp := trace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(noop.NewTracerProvider())

	// Test handler that does nothing
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Apply the Span middleware
	middleware := Span()(testHandler)

	// Create a context with a span
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	// Create an HTTP request with context
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody).WithContext(ctx)

	// Using a ResponseRecorder to capture the response
	rr := httptest.NewRecorder()

	// Serve the request using our middleware
	middleware.ServeHTTP(rr, req)

	// Assert the trace-id is in the response header
	traceID := rr.Header().Get(TraceIDHeader)
	require.NotEmpty(t, traceID, "trace-id header should be set")
}

func TestSpanMiddleware_NoSpan(t *testing.T) {
	// Test handler that does nothing
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Apply the Span middleware
	middleware := Span()(testHandler)

	// Create an HTTP request without span in context
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	// Using a ResponseRecorder to capture the response
	rr := httptest.NewRecorder()

	// Serve the request using our middleware
	middleware.ServeHTTP(rr, req)

	// Assert the trace-id is NOT in the response header (no span in context)
	traceID := rr.Header().Get(TraceIDHeader)
	require.Empty(t, traceID, "trace-id header should not be set when no span in context")
}

func TestSpanMiddleware_StatusCodes(t *testing.T) {
	tp := trace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(noop.NewTracerProvider())

	exporter := tracetest.NewInMemoryExporter()
	tp = trace.NewTracerProvider(trace.WithBatcher(exporter))
	otel.SetTracerProvider(tp)

	testCases := []struct {
		name       string
		statusCode int
	}{
		{"2xx OK", http.StatusOK},
		{"3xx Redirect", http.StatusMovedPermanently},
		{"4xx Client Error", http.StatusBadRequest},
		{"5xx Server Error", http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset exporter
			exporter.Reset()

			// Test handler with specific status code
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			})

			// Apply the Span middleware
			middleware := Span()(testHandler)

			// Create a context with a span
			ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
			defer span.End()

			// Create an HTTP request with context
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody).WithContext(ctx)

			// Using a ResponseRecorder to capture the response
			rr := httptest.NewRecorder()

			// Serve the request using our middleware
			middleware.ServeHTTP(rr, req)

			// Assert the trace-id is in the response header
			traceID := rr.Header().Get(TraceIDHeader)
			require.NotEmpty(t, traceID, "trace-id header should be set")
		})
	}
}
