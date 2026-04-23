package logger_middleware_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	logger_middleware "github.com/shortlink-org/go-sdk/http/middleware/logger"
	"github.com/shortlink-org/go-sdk/http/middleware/logger/mocks"
)

const (
	attrKeyStatus     = "status"
	mockVariadicSlots = 15
)

func extractAttrs(args mock.Arguments) []slog.Attr {
	var out []slog.Attr

	for _, a := range args[2:] { // skip ctx + msg
		switch v := a.(type) {
		case slog.Attr:
			out = append(out, v)

		case []slog.Attr: // rarely happens
			out = append(out, v...)

		case []any:
			for _, iv := range v {
				if attr, ok := iv.(slog.Attr); ok {
					out = append(out, attr)
				}
			}

		case any:
			if attr, ok := v.(slog.Attr); ok {
				out = append(out, attr)
			}

		default:
			// ignore unsupported slog handler shapes in tests
		}
	}

	return out
}

// setupMockLoggerCall sets up a mock logger call that captures variadic attributes
// without hardcoding the number of arguments
func setupMockLoggerCall(mockLogger *mocks.MockLogger, method string, msg any) *mock.Call {
	variadicMatchers := make([]any, mockVariadicSlots)
	for i := range variadicMatchers {
		variadicMatchers[i] = mock.Anything
	}

	return mockLogger.On(method, append([]any{mock.Anything, msg}, variadicMatchers...)...)
}

func attrsContainStatus(attrs []slog.Attr, want int64) bool {
	for _, attr := range attrs {
		if attr.Key == attrKeyStatus && attr.Value.Int64() == want {
			return true
		}
	}

	return false
}

func TestLoggerMiddleware_RequestCompletedByStatus(t *testing.T) {
	tests := []struct {
		name       string
		mockMethod string
		path       string
		setup      func(*testing.T, http.ResponseWriter)
		wantCode   int
	}{
		{
			name:       "info_200",
			mockMethod: "InfoWithContext",
			path:       "/info",
			setup: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.WriteHeader(http.StatusOK)

				_, err := w.Write([]byte("hello"))
				assert.NoError(t, err)
			},
			wantCode: http.StatusOK,
		},
		{
			name:       "warn_400",
			mockMethod: "WarnWithContext",
			path:       "/bad",
			setup: func(_ *testing.T, w http.ResponseWriter) {
				http.Error(w, "bad request", http.StatusBadRequest)
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:       "error_500",
			mockMethod: "ErrorWithContext",
			path:       "/err",
			setup: func(_ *testing.T, w http.ResponseWriter) {
				http.Error(w, "fail", http.StatusInternalServerError)
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := mocks.NewMockLogger(t)

			var capturedAttrs []slog.Attr

			setupMockLoggerCall(mockLogger, tt.mockMethod, "request completed").Run(func(args mock.Arguments) {
				capturedAttrs = extractAttrs(args)
			}).Return().Maybe()

			mw := logger_middleware.Logger(mockLogger)

			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				tt.setup(t, w)
			}))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tt.path, http.NoBody)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.wantCode, rr.Code)
			require.True(t, attrsContainStatus(capturedAttrs, int64(tt.wantCode)),
				"expected status=%d in attributes", tt.wantCode)

			mockLogger.AssertExpectations(t)
		})
	}
}

// Panic recovery → 500 + ERROR
func TestLoggerMiddleware_Panic(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)

	var capturedMsg string

	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "ErrorWithContext", mock.Anything).Run(func(args mock.Arguments) {
		if s, ok := args[1].(string); ok {
			capturedMsg = s
		}

		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	handler := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/panic", http.NoBody)
	rr := httptest.NewRecorder()

	defer func() {
		recover() //nolint:errcheck // recover return is the panic value, not an error
	}()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, capturedMsg, "panic recovered")

	hasPanic := false
	hasStack := false

	for _, attr := range capturedAttrs {
		if attr.Key == "panic" {
			hasPanic = true
		}

		if attr.Key == "stack" {
			hasStack = true
		}
	}

	require.True(t, hasPanic, "expected panic attribute")
	require.True(t, hasStack, "expected stack attribute")

	mockLogger.AssertExpectations(t)
}

// BytesWritten
func TestLoggerMiddleware_BytesWritten(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)

	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "InfoWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte("abc"))
		assert.NoError(t, err)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/bytes", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	found := false

	for _, attr := range capturedAttrs {
		if attr.Key == "bytes" && attr.Value.Int64() == 3 {
			found = true
			break
		}
	}

	require.True(t, found, "expected bytes=3 in attributes")

	mockLogger.AssertExpectations(t)
}

// Query string logged
func TestLoggerMiddleware_QueryString(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)

	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "InfoWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/search?term=go", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	found := false

	for _, attr := range capturedAttrs {
		if attr.Key == "query" && attr.Value.String() == "term=go" {
			found = true
			break
		}
	}

	require.True(t, found, "expected query=term=go in attributes")

	mockLogger.AssertExpectations(t)
}

// Parent span must propagate trace_id + span_id
func TestLoggerMiddleware_OtelTracePropagation(t *testing.T) {
	tp := sdktrace.NewTracerProvider()

	otel.SetTracerProvider(tp)

	defer otel.SetTracerProvider(tracenoop.NewTracerProvider())

	mockLogger := mocks.NewMockLogger(t)

	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "InfoWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	tracer := otel.Tracer("test-tracer")

	ctx, span := tracer.Start(context.Background(), "parent-span")

	defer span.End()

	spanCtx := span.SpanContext()
	traceID := spanCtx.TraceID().String()
	spanID := spanCtx.SpanID().String()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/otel", http.NoBody)
	rr := httptest.NewRecorder()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	hasTraceID := false
	hasSpanID := false

	for _, attr := range capturedAttrs {
		if attr.Key == "trace_id" && attr.Value.String() == traceID {
			hasTraceID = true
		}

		if attr.Key == "span_id" && attr.Value.String() == spanID {
			hasSpanID = true
		}
	}

	require.True(t, hasTraceID, "trace_id must be logged")
	require.True(t, hasSpanID, "span_id must be logged")

	mockLogger.AssertExpectations(t)
}

// No span in context → no trace_id / span_id fields
func TestLoggerMiddleware_Otel_NoSpan(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)

	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "InfoWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nospan", http.NoBody)
	rr := httptest.NewRecorder()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	for _, attr := range capturedAttrs {
		require.NotEqual(t, "trace_id", attr.Key, "should not have trace_id")
		require.NotEqual(t, "span_id", attr.Key, "should not have span_id")
	}

	mockLogger.AssertExpectations(t)
}

// Child span created inside handler must override parent
func TestLoggerMiddleware_Otel_ChildSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider()

	otel.SetTracerProvider(tp)

	defer otel.SetTracerProvider(tracenoop.NewTracerProvider())

	mockLogger := mocks.NewMockLogger(t)

	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "InfoWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	tracer := otel.Tracer("test-tracer")

	ctx, parent := tracer.Start(context.Background(), "parent")

	defer parent.End()

	type spanIDs struct {
		traceID string
		spanID  string
	}

	childIDsCh := make(chan spanIDs, 1)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/child", http.NoBody)
	rr := httptest.NewRecorder()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, childSpan := tracer.Start(r.Context(), "child")

		defer childSpan.End()

		sc := childSpan.SpanContext()
		childIDsCh <- spanIDs{
			traceID: sc.TraceID().String(),
			spanID:  sc.SpanID().String(),
		}

		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	childIDs := <-childIDsCh

	parentCtx := parent.SpanContext()
	parentTraceID := parentCtx.TraceID().String()
	parentSpanID := parentCtx.SpanID().String()

	hasTraceID := false
	hasSpanID := false

	for _, attr := range capturedAttrs {
		if attr.Key == "trace_id" && attr.Value.String() == parentTraceID {
			hasTraceID = true
		}

		if attr.Key == "span_id" && attr.Value.String() == parentSpanID {
			hasSpanID = true
		}
	}

	require.True(t, hasTraceID, "parent trace_id must be logged")
	require.True(t, hasSpanID, "parent span_id must be logged")

	childTraceID := childIDs.traceID
	childSpanID := childIDs.spanID

	require.Equal(t, parentTraceID, childTraceID, "child should have same trace_id as parent")
	require.NotEqual(t, parentSpanID, childSpanID, "child should have different span_id than parent")

	mockLogger.AssertExpectations(t)
}
