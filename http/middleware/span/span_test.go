package span_middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// newRecorder returns a provider whose ended spans the recorder keeps.
func newRecorder(t *testing.T) (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	return provider, recorder
}

// serve runs one request through the middleware under a parent span the way
// otelhttp would open it, and returns the finished span and the response.
//
//nolint:ireturn // ReadOnlySpan is what the recorder hands out
func serve(
	t *testing.T,
	provider trace.TracerProvider,
	recorder *tracetest.SpanRecorder,
	handler http.Handler,
	method, target string,
) (sdktrace.ReadOnlySpan, *httptest.ResponseRecorder) {
	t.Helper()

	ctx, parent := provider.Tracer("test").Start(t.Context(), method, trace.WithSpanKind(trace.SpanKindServer))

	request := httptest.NewRequestWithContext(ctx, method, target, http.NoBody)
	response := httptest.NewRecorder()

	Span()(handler).ServeHTTP(response, request)
	parent.End()

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	return spans[0], response
}

func attributesOf(span sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	attrs := make(map[attribute.Key]attribute.Value, len(span.Attributes()))
	for _, kv := range span.Attributes() {
		attrs[kv.Key] = kv.Value
	}

	return attrs
}

func status(code int) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(code)
	})
}

func TestSpanMiddleware_TraceIDHeader(t *testing.T) {
	t.Parallel()

	t.Run("echoes the trace id", func(t *testing.T) {
		t.Parallel()

		provider, recorder := newRecorder(t)
		span, response := serve(t, provider, recorder, status(http.StatusOK), http.MethodGet, "/")

		require.Equal(t, span.SpanContext().TraceID().String(), response.Header().Get(TraceIDHeader))
	})

	t.Run("stays silent without a span", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		response := httptest.NewRecorder()

		Span()(status(http.StatusOK)).ServeHTTP(response, request)

		require.Empty(t, response.Header().Get(TraceIDHeader))
		require.Equal(t, http.StatusOK, response.Code)
	})
}

func TestSpanMiddleware_Status(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		statusCode int
		status     codes.Code
		errorType  string
	}{
		// Instrumentation never sets OK: that is for application code.
		{name: "2xx stays unset", statusCode: http.StatusOK, status: codes.Unset},
		{name: "3xx stays unset", statusCode: http.StatusMovedPermanently, status: codes.Unset},
		// A client error is not a server failure.
		{name: "4xx stays unset", statusCode: http.StatusBadRequest, status: codes.Unset},
		// A failed span must carry error.type; for HTTP that is the status code.
		{name: "5xx is an error", statusCode: http.StatusInternalServerError, status: codes.Error, errorType: "500"},
		{name: "503 is an error", statusCode: http.StatusServiceUnavailable, status: codes.Error, errorType: "503"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			provider, recorder := newRecorder(t)
			span, _ := serve(t, provider, recorder, status(tc.statusCode), http.MethodGet, "/")

			require.Equal(t, tc.status, span.Status().Code)

			errorType, ok := attributesOf(span)["error.type"]
			if tc.errorType == "" {
				require.False(t, ok, "error.type must not be set without an error status")

				return
			}

			require.True(t, ok, "error.type must be set on a failed span")
			require.Equal(t, tc.errorType, errorType.AsString())
		})
	}
}

func TestSpanMiddleware_Route(t *testing.T) {
	t.Parallel()

	t.Run("chi router", func(t *testing.T) {
		t.Parallel()

		router := chi.NewRouter()
		router.Use(Span())
		router.Get("/users/{id}", func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusOK)
		})

		provider, recorder := newRecorder(t)
		ctx, parent := provider.Tracer("test").Start(t.Context(), http.MethodGet, trace.WithSpanKind(trace.SpanKindServer))

		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, "/users/42", http.NoBody))
		parent.End()

		spans := recorder.Ended()
		require.Len(t, spans, 1)
		require.Equal(t, "GET /users/{id}", spans[0].Name())
		require.Equal(t, "/users/{id}", attributesOf(spans[0])["http.route"].AsString())
	})

	t.Run("net/http ServeMux", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.Handle("GET /items/{id}", status(http.StatusOK))

		provider, recorder := newRecorder(t)
		span, _ := serve(t, provider, recorder, mux, http.MethodGet, "/items/7")

		require.Equal(t, "GET /items/{id}", span.Name())
		require.Equal(t, "/items/{id}", attributesOf(span)["http.route"].AsString())
	})

	t.Run("unknown method is named HTTP", func(t *testing.T) {
		t.Parallel()

		// A ServeMux pattern without a method accepts any method, including
		// one the conventions do not know.
		mux := http.NewServeMux()
		mux.Handle("/cache", status(http.StatusOK))

		provider, recorder := newRecorder(t)
		span, _ := serve(t, provider, recorder, mux, "PURGE", "/cache")

		require.Equal(t, "HTTP /cache", span.Name())
	})

	t.Run("no router keeps the name", func(t *testing.T) {
		t.Parallel()

		provider, recorder := newRecorder(t)
		span, _ := serve(t, provider, recorder, status(http.StatusOK), http.MethodGet, "/anything")

		require.Equal(t, http.MethodGet, span.Name())
		require.NotContains(t, attributesOf(span), attribute.Key("http.route"))
	})

	t.Run("route reaches the otelhttp labeler", func(t *testing.T) {
		t.Parallel()

		var labeler *otelhttp.Labeler

		router := chi.NewRouter()
		router.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				// otelhttp installs the labeler before the router; stand in for it.
				ctx := otelhttp.ContextWithLabeler(request.Context(), &otelhttp.Labeler{})
				labeler, _ = otelhttp.LabelerFromContext(ctx)
				next.ServeHTTP(writer, request.WithContext(ctx))
			})
		})
		router.Use(Span())
		router.Get("/users/{id}", func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusOK)
		})

		provider, recorder := newRecorder(t)
		serve(t, provider, recorder, router, http.MethodGet, "/users/1")

		require.NotNil(t, labeler)
		require.Contains(t, labeler.Get(), attribute.String("http.route", "/users/{id}"))
	})
}
