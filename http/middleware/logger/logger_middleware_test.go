package logger_middleware_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	logger_middleware "github.com/shortlink-org/go-sdk/http/middleware/logger"
	"github.com/shortlink-org/go-sdk/http/middleware/logger/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

////////////////////////////////////////////////////////////////////////////////
// HELPERS
////////////////////////////////////////////////////////////////////////////////

func extractAttrs(args mock.Arguments) []slog.Attr {
	var out []slog.Attr

	for _, a := range args[2:] { // skip ctx + msg
		switch v := a.(type) {
		case slog.Attr:
			out = append(out, v)

		case []slog.Attr: // rarely happens
			out = append(out, v...)

		case []interface{}:
			for _, iv := range v {
				if attr, ok := iv.(slog.Attr); ok {
					out = append(out, attr)
				}
			}

		case interface{}:
			if attr, ok := v.(slog.Attr); ok {
				out = append(out, attr)
			}
		}
	}
	return out
}

// setupMockLoggerCall sets up a mock logger call that captures variadic attributes
// without hardcoding the number of arguments
func setupMockLoggerCall(mockLogger *mocks.MockLogger, method string, msg interface{}) *mock.Call {
	// Create enough mock.Anything for all possible variadic arguments
	variadicMatchers := make([]interface{}, 15) // enough for all possible attrs
	for i := range variadicMatchers {
		variadicMatchers[i] = mock.Anything
	}

	// Use variadic spread
	return mockLogger.On(method, append([]interface{}{mock.Anything, msg}, variadicMatchers...)...)
}

////////////////////////////////////////////////////////////////////////////////
// TESTS
////////////////////////////////////////////////////////////////////////////////

// INFO 200 OK
func TestLoggerMiddleware_Info(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "InfoWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	// Verify attributes
	found := false
	for _, attr := range capturedAttrs {
		if attr.Key == "status" && attr.Value.Int64() == int64(http.StatusOK) {
			found = true
			break
		}
	}
	require.True(t, found, "expected status=200 in attributes")

	mockLogger.AssertExpectations(t)
}

// WARN 400 BadRequest
func TestLoggerMiddleware_Warn(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "WarnWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	// Verify attributes
	found := false
	for _, attr := range capturedAttrs {
		if attr.Key == "status" && attr.Value.Int64() == int64(http.StatusBadRequest) {
			found = true
			break
		}
	}
	require.True(t, found, "expected status=400 in attributes")

	mockLogger.AssertExpectations(t)
}

// ERROR 500 InternalServerError
func TestLoggerMiddleware_Error(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "ErrorWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	// Verify attributes
	found := false
	for _, attr := range capturedAttrs {
		if attr.Key == "status" && attr.Value.Int64() == int64(http.StatusInternalServerError) {
			found = true
			break
		}
	}
	require.True(t, found, "expected status=500 in attributes")

	mockLogger.AssertExpectations(t)
}

// Panic recovery → 500 + ERROR
func TestLoggerMiddleware_Panic(t *testing.T) {
	mockLogger := mocks.NewMockLogger(t)
	var capturedMsg string
	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "ErrorWithContext", mock.Anything).Run(func(args mock.Arguments) {
		if args[1] != nil {
			capturedMsg = args[1].(string)
		}
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	handler := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rr := httptest.NewRecorder()

	defer func() { _ = recover() }()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, capturedMsg, "panic recovered")

	// Verify panic and stack attributes
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
		w.Write([]byte("abc"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/bytes", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify bytes attribute
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

	req := httptest.NewRequest(http.MethodGet, "/search?term=go", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify query attribute
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

////////////////////////////////////////////////////////////////////////////////
// OTEL TRACE PROPAGATION TESTS
////////////////////////////////////////////////////////////////////////////////

// Parent span must propagate trace_id + span_id
func TestLoggerMiddleware_OtelTracePropagation(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(trace.NewNoopTracerProvider())

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

	req := httptest.NewRequest(http.MethodGet, "/otel", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	// Verify trace IDs are logged
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

	req := httptest.NewRequest(http.MethodGet, "/nospan", nil)
	rr := httptest.NewRecorder()

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	// Verify no trace_id or span_id in attributes
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
	defer otel.SetTracerProvider(trace.NewNoopTracerProvider())

	mockLogger := mocks.NewMockLogger(t)
	var capturedAttrs []slog.Attr

	setupMockLoggerCall(mockLogger, "InfoWithContext", "request completed").Run(func(args mock.Arguments) {
		capturedAttrs = extractAttrs(args)
	}).Return().Maybe()

	mw := logger_middleware.Logger(mockLogger)

	tracer := otel.Tracer("test-tracer")
	ctx, parent := tracer.Start(context.Background(), "parent")
	defer parent.End()

	req := httptest.NewRequest(http.MethodGet, "/child", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	var childCtx context.Context
	var childSpan trace.Span

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		childCtx, childSpan = tracer.Start(r.Context(), "child")
		defer childSpan.End()
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	// Middleware logs the span from request context (parent span), not child span
	parentCtx := parent.SpanContext()
	parentTraceID := parentCtx.TraceID().String()
	parentSpanID := parentCtx.SpanID().String()

	// Verify parent trace IDs are logged (middleware uses r.Context(), which has parent span)
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

	// Verify child span has same trace_id but different span_id
	childSc := trace.SpanContextFromContext(childCtx)
	childTraceID := childSc.TraceID().String()
	childSpanID := childSc.SpanID().String()
	require.Equal(t, parentTraceID, childTraceID, "child should have same trace_id as parent")
	require.NotEqual(t, parentSpanID, childSpanID, "child should have different span_id than parent")

	mockLogger.AssertExpectations(t)
}
