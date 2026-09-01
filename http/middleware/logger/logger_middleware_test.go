package logger_middleware_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	logger_middleware "github.com/shortlink-org/go-sdk/http/middleware/logger"
)

const (
	attrKeyStatus = "status"
	bytesWritten  = 3
)

// captureHandler keeps every record the middleware emits, so a test can
// assert on the level and the attributes without a mock
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

// Handle stores a clone: slog.Record keeps its first attrs in an inline array
// and the rest in a slice it may share, so holding an unmodified record past
// the call is a documented aliasing hazard
//
//nolint:gocritic // slog.Handler.Handle takes the record by value; the signature is not ours
func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.records = append(h.records, record.Clone())

	return nil
}

//nolint:ireturn // slog.Handler is the stdlib contract
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

//nolint:ireturn // slog.Handler is the stdlib contract
func (h *captureHandler) WithGroup(string) slog.Handler { return h }

// only asserts the middleware logged exactly once and returns that record
func (h *captureHandler) only(t *testing.T) slog.Record {
	t.Helper()

	h.mu.Lock()
	defer h.mu.Unlock()

	require.Len(t, h.records, 1, "the middleware must log the request exactly once")

	return h.records[0]
}

// newCapturingMiddleware wires the middleware to a capturing handler
func newCapturingMiddleware(t *testing.T) (*captureHandler, func(http.Handler) http.Handler) {
	t.Helper()

	handler := &captureHandler{mu: sync.Mutex{}, records: nil}

	return handler, logger_middleware.Logger(slog.New(handler))
}

//nolint:gocritic // a test helper: copying the record here is not a hot path
func attrsOf(record slog.Record) []slog.Attr {
	out := make([]slog.Attr, 0, record.NumAttrs())

	record.Attrs(func(attr slog.Attr) bool {
		out = append(out, attr)

		return true
	})

	return out
}

func attrsContainStatus(attrs []slog.Attr, want int64) bool {
	for _, attr := range attrs {
		if attr.Key == attrKeyStatus && attr.Value.Int64() == want {
			return true
		}
	}

	return false
}

func serve(t *testing.T, mw func(http.Handler) http.Handler, req *http.Request, next http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	return rr
}

func TestLoggerMiddleware_RequestCompletedByStatus(t *testing.T) {
	tests := []struct {
		name      string
		wantLevel slog.Level
		path      string
		setup     func(*testing.T, http.ResponseWriter)
		wantCode  int
	}{
		{
			name:      "info_200",
			wantLevel: slog.LevelInfo,
			path:      "/info",
			setup: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.WriteHeader(http.StatusOK)

				_, err := w.Write([]byte("hello"))
				assert.NoError(t, err)
			},
			wantCode: http.StatusOK,
		},
		{
			name:      "warn_400",
			wantLevel: slog.LevelWarn,
			path:      "/bad",
			setup: func(_ *testing.T, w http.ResponseWriter) {
				http.Error(w, "bad request", http.StatusBadRequest)
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:      "error_500",
			wantLevel: slog.LevelError,
			path:      "/err",
			setup: func(_ *testing.T, w http.ResponseWriter) {
				http.Error(w, "fail", http.StatusInternalServerError)
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture, mw := newCapturingMiddleware(t)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tt.path, http.NoBody)
			rr := serve(t, mw, req, func(w http.ResponseWriter, _ *http.Request) {
				tt.setup(t, w)
			})

			require.Equal(t, tt.wantCode, rr.Code)

			record := capture.only(t)
			require.Equal(t, tt.wantLevel, record.Level)
			require.Equal(t, "request completed", record.Message)
			require.True(t, attrsContainStatus(attrsOf(record), int64(tt.wantCode)),
				"expected status=%d in attributes", tt.wantCode)
		})
	}
}

// Panic recovery must report 500 and log the panic value with its stack
func TestLoggerMiddleware_Panic(t *testing.T) {
	capture, mw := newCapturingMiddleware(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/panic", http.NoBody)

	defer func() {
		recover() //nolint:errcheck // recover returns the panic value, not an error
	}()

	rr := serve(t, mw, req, func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	record := capture.only(t)
	require.Equal(t, slog.LevelError, record.Level)
	require.Contains(t, record.Message, "panic recovered")

	hasPanic := false
	hasStack := false

	for _, attr := range attrsOf(record) {
		if attr.Key == "panic" {
			hasPanic = true
		}

		if attr.Key == "stack" {
			hasStack = true
		}
	}

	require.True(t, hasPanic, "expected panic attribute")
	require.True(t, hasStack, "expected stack attribute")
}

func TestLoggerMiddleware_BytesWritten(t *testing.T) {
	capture, mw := newCapturingMiddleware(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/bytes", http.NoBody)
	serve(t, mw, req, func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte("abc"))
		assert.NoError(t, err)
	})

	found := false

	for _, attr := range attrsOf(capture.only(t)) {
		if attr.Key == "bytes" && attr.Value.Int64() == bytesWritten {
			found = true

			break
		}
	}

	require.True(t, found, "expected bytes=3 in attributes")
}

func TestLoggerMiddleware_QueryString(t *testing.T) {
	capture, mw := newCapturingMiddleware(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/search?term=go", http.NoBody)
	serve(t, mw, req, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	found := false

	for _, attr := range attrsOf(capture.only(t)) {
		if attr.Key == "query" && attr.Value.String() == "term=go" {
			found = true

			break
		}
	}

	require.True(t, found, "expected query=term=go in attributes")
}

// A parent span must propagate trace_id and span_id onto the log record
func TestLoggerMiddleware_OtelTracePropagation(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())

	defer otel.SetTracerProvider(tracenoop.NewTracerProvider())

	capture, mw := newCapturingMiddleware(t)

	ctx, span := otel.Tracer("test-tracer").Start(context.Background(), "parent-span")

	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/otel", http.NoBody)
	serve(t, mw, req, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	hasTraceID := false
	hasSpanID := false

	for _, attr := range attrsOf(capture.only(t)) {
		if attr.Key == "trace_id" && attr.Value.String() == traceID {
			hasTraceID = true
		}

		if attr.Key == "span_id" && attr.Value.String() == spanID {
			hasSpanID = true
		}
	}

	require.True(t, hasTraceID, "trace_id must be logged")
	require.True(t, hasSpanID, "span_id must be logged")
}

// Without a span in context there is nothing to correlate, so neither field
// may be emitted
func TestLoggerMiddleware_Otel_NoSpan(t *testing.T) {
	capture, mw := newCapturingMiddleware(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nospan", http.NoBody)
	serve(t, mw, req, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, attr := range attrsOf(capture.only(t)) {
		require.NotEqual(t, "trace_id", attr.Key, "should not have trace_id")
		require.NotEqual(t, "span_id", attr.Key, "should not have span_id")
	}
}

// A child span started inside the handler must not displace the span the
// middleware logs, which is the one it saw on the way in
func TestLoggerMiddleware_Otel_ChildSpan(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())

	defer otel.SetTracerProvider(tracenoop.NewTracerProvider())

	capture, mw := newCapturingMiddleware(t)

	tracer := otel.Tracer("test-tracer")

	ctx, parent := tracer.Start(context.Background(), "parent")

	defer parent.End()

	type spanIDs struct {
		traceID string
		spanID  string
	}

	childIDsCh := make(chan spanIDs, 1)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/child", http.NoBody)
	serve(t, mw, req, func(w http.ResponseWriter, r *http.Request) {
		_, childSpan := tracer.Start(r.Context(), "child")

		defer childSpan.End()

		childIDsCh <- spanIDs{
			traceID: childSpan.SpanContext().TraceID().String(),
			spanID:  childSpan.SpanContext().SpanID().String(),
		}

		w.WriteHeader(http.StatusOK)
	})

	childIDs := <-childIDsCh
	parentTraceID := parent.SpanContext().TraceID().String()
	parentSpanID := parent.SpanContext().SpanID().String()

	hasTraceID := false
	hasSpanID := false

	for _, attr := range attrsOf(capture.only(t)) {
		if attr.Key == "trace_id" && attr.Value.String() == parentTraceID {
			hasTraceID = true
		}

		if attr.Key == "span_id" && attr.Value.String() == parentSpanID {
			hasSpanID = true
		}
	}

	require.True(t, hasTraceID, "parent trace_id must be logged")
	require.True(t, hasSpanID, "parent span_id must be logged")

	require.Equal(t, parentTraceID, childIDs.traceID, "child should have same trace_id as parent")
	require.NotEqual(t, parentSpanID, childIDs.spanID, "child should have different span_id than parent")
}
