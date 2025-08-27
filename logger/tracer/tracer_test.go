package tracer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/shortlink-org/go-sdk/logger/tracer"
)

func setupTracer(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider()
	tp.RegisterSpanProcessor(rec)
	otel.SetTracerProvider(tp)

	cleanup := func() {
		_ = tp.Shutdown(context.Background())
	}

	return rec, cleanup
}

// findAttr returns attribute value by key with type-safety.
func findAttr[T any](attrs []attribute.KeyValue, key string) (T, bool) {
	var zero T

	for _, a := range attrs {
		if string(a.Key) != key {
			continue
		}
		// no unnecessary conversion
		if v, ok := a.Value.AsInterface().(T); ok {
			return v, true
		}
	}

	return zero, false
}

// valueOf fetches a value from returned fields (key,value,...).
func valueOf[T any](fields []any, key string) (T, bool) {
	var zero T

	for i := 0; i+1 < len(fields); i += 2 {
		k, ok := fields[i].(string)
		if !ok || k != key {
			continue
		}

		if v, ok := fields[i+1].(T); ok {
			return v, true
		}
	}

	return zero, false
}

func Test_NewTraceFromContext_EventOnActiveSpan_ERROR(t *testing.T) {
	rec, cleanup := setupTracer(t)
	defer cleanup()

	// Active/root span
	ctx, root := otel.Tracer("test").Start(context.Background(), "root")

	// Should add Event to existing span, not create a new one
	fields, err := tracer.NewTraceFromContext(
		ctx, "ERROR", "boom",
		nil, // tags
		"k", "v", "err", assert.AnError, "is_error", true,
	)
	require.NoError(t, err)
	require.Contains(t, fields, "traceID")
	require.Contains(t, fields, "spanID")

	root.End()

	spans := rec.Ended()
	require.Len(t, spans, 1, "must not create extra spans when active span exists")

	ros := spans[0]
	require.Equal(t, codes.Error, ros.Status().Code)

	// Event "log.ERROR" with attrs
	evs := ros.Events()
	require.NotEmpty(t, evs)

	var logEv sdktrace.Event

	found := false

	for _, e := range evs {
		if e.Name == "log.ERROR" {
			logEv = e
			found = true

			break
		}
	}

	require.True(t, found, "expected event 'log.ERROR'")

	if v, ok := findAttr[string](logEv.Attributes, "log.severity"); assert.True(t, ok) {
		assert.Equal(t, "ERROR", v)
	}

	if v, ok := findAttr[string](logEv.Attributes, "log.message"); assert.True(t, ok) {
		assert.Equal(t, "boom", v)
	}

	if v, ok := findAttr[string](logEv.Attributes, "exception.message"); assert.True(t, ok) {
		assert.Equal(t, assert.AnError.Error(), v)
	}

	_, ok := findAttr[string](logEv.Attributes, "exception.type")
	assert.True(t, ok, "exception.type should be set")

	if v, ok := findAttr[bool](logEv.Attributes, "log.is_error"); assert.True(t, ok) {
		assert.True(t, v)
	}
}

func Test_NewTraceFromContext_NoActiveSpan_Info_NoSpan(t *testing.T) {
	rec, cleanup := setupTracer(t)
	defer cleanup()

	out, err := tracer.NewTraceFromContext(context.Background(), "INFO", "hello", nil, "a", "b")
	require.NoError(t, err)

	// INFO without active span must NOT create a span
	require.Empty(t, rec.Ended())
	// and returns original fields without correlation ids
	assert.Equal(t, []any{"a", "b"}, out)
}

func Test_NewTraceFromContext_NoActiveSpan_Warn_CreatesShortSpan(t *testing.T) {
	rec, cleanup := setupTracer(t)
	defer cleanup()

	out, err := tracer.NewTraceFromContext(context.Background(), "WARN", "heads-up", nil, "x", 1)
	require.NoError(t, err)

	// creates exactly one short span
	spans := rec.Ended()
	require.Len(t, spans, 1)

	ros := spans[0]

	attrs := ros.Attributes()
	if v, ok := findAttr[string](attrs, "log.severity"); assert.True(t, ok) {
		assert.Equal(t, "WARN", v)
	}

	if v, ok := findAttr[string](attrs, "log.message"); assert.True(t, ok) {
		assert.Equal(t, "heads-up", v)
	}

	// returned fields include correlation ids equal to created span
	traceID, ok := valueOf[string](out, "traceID")
	require.True(t, ok)
	spanID, ok := valueOf[string](out, "spanID")
	require.True(t, ok)
	assert.Equal(t, ros.SpanContext().TraceID().String(), traceID)
	assert.Equal(t, ros.SpanContext().SpanID().String(), spanID)
}

func Test_NewTraceFromContext_Normalization_IsErrorAndStringErr_OnActiveSpan(t *testing.T) {
	rec, cleanup := setupTracer(t)
	defer cleanup()

	ctx, root := otel.Tracer("test").Start(context.Background(), "root")
	_, err := tracer.NewTraceFromContext(
		ctx, "INFO", "msg",
		nil,
		"is_error", "true",
		"err", "oops",
	)
	require.NoError(t, err)
	root.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)

	var logEv sdktrace.Event

	for _, e := range spans[0].Events() {
		if e.Name == "log.INFO" {
			logEv = e

			break
		}
	}

	if v, ok := findAttr[bool](logEv.Attributes, "log.is_error"); assert.True(t, ok) {
		assert.True(t, v, "string 'true' should be coerced to bool true")
	}

	if v, ok := findAttr[string](logEv.Attributes, "exception.message"); assert.True(t, ok) {
		assert.Equal(t, "oops", v)
	}
}
