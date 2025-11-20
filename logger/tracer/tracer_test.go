package tracer_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/shortlink-org/go-sdk/logger/tracer"
)

//nolint:gocognit,funlen,maintidx // test function with multiple assertions
func Test_NewTraceFromContext_EventOnActiveSpan_ERROR(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider()
	tp.RegisterSpanProcessor(rec)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		err := tp.Shutdown(context.Background())
		require.NoError(t, err)
	})

	// Active/root span
	ctx, root := otel.Tracer("test").Start(context.Background(), "root")

	// Should add Event to existing span, not create a new one
	fields, err := tracer.NewTraceFromContext(
		ctx, "ERROR", "boom",
		nil, // tags
		slog.String("k", "v"), slog.Any("err", assert.AnError), slog.Bool("is_error", true),
	)
	require.NoError(t, err)

	// Check if traceID and spanID are present in fields
	var hasTraceID, hasSpanID bool

	for _, field := range fields {
		if field.Key == "traceID" {
			hasTraceID = true
		}

		if field.Key == "spanID" {
			hasSpanID = true
		}
	}

	require.True(t, hasTraceID)
	require.True(t, hasSpanID)

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

	var (
		severity   string
		severityOK bool
	)

	for _, a := range logEv.Attributes {
		if string(a.Key) == "log.severity" {
			if v, ok := a.Value.AsInterface().(string); ok {
				severity = v
				severityOK = true

				break
			}
		}
	}

	if assert.True(t, severityOK) {
		assert.Equal(t, "ERROR", severity)
	}

	var (
		message   string
		messageOK bool
	)

	for _, a := range logEv.Attributes {
		if string(a.Key) == "log.message" {
			if v, ok := a.Value.AsInterface().(string); ok {
				message = v
				messageOK = true

				break
			}
		}
	}

	if assert.True(t, messageOK) {
		assert.Equal(t, "boom", message)
	}

	var (
		exceptionMessage   string
		exceptionMessageOK bool
	)

	for _, a := range logEv.Attributes {
		if string(a.Key) == "exception.message" {
			if v, ok := a.Value.AsInterface().(string); ok {
				exceptionMessage = v
				exceptionMessageOK = true

				break
			}
		}
	}

	if assert.True(t, exceptionMessageOK) {
		assert.Equal(t, assert.AnError.Error(), exceptionMessage)
	}

	var exceptionTypeOK bool

	for _, a := range logEv.Attributes {
		if string(a.Key) == "exception.type" {
			_, ok := a.Value.AsInterface().(string)
			exceptionTypeOK = ok

			break
		}
	}

	assert.True(t, exceptionTypeOK, "exception.type should be set")

	var (
		isError   bool
		isErrorOK bool
	)

	for _, a := range logEv.Attributes {
		if string(a.Key) == "log.is_error" {
			if v, ok := a.Value.AsInterface().(bool); ok {
				isError = v
				isErrorOK = true

				break
			}
		}
	}

	if assert.True(t, isErrorOK) {
		assert.True(t, isError)
	}
}

func Test_NewTraceFromContext_NoActiveSpan_Info_NoSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider()
	tp.RegisterSpanProcessor(rec)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		err := tp.Shutdown(context.Background())
		require.NoError(t, err)
	})

	out, err := tracer.NewTraceFromContext(context.Background(), "INFO", "hello", nil, slog.String("a", "b"))
	require.NoError(t, err)

	// INFO without active span must NOT create a span
	require.Empty(t, rec.Ended())
	// and returns original fields without correlation ids
	assert.Equal(t, []slog.Attr{slog.String("a", "b")}, out)
}

func Test_NewTraceFromContext_NoActiveSpan_Warn_CreatesShortSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider()
	tp.RegisterSpanProcessor(rec)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		err := tp.Shutdown(context.Background())
		require.NoError(t, err)
	})

	out, err := tracer.NewTraceFromContext(context.Background(), "WARN", "heads-up", nil, slog.Int("x", 1))
	require.NoError(t, err)

	// creates exactly one short span
	spans := rec.Ended()
	require.Len(t, spans, 1)

	ros := spans[0]

	attrs := ros.Attributes()

	var (
		severity   string
		severityOK bool
	)

	for _, a := range attrs {
		if string(a.Key) == "log.severity" {
			if v, ok := a.Value.AsInterface().(string); ok {
				severity = v
				severityOK = true

				break
			}
		}
	}

	if assert.True(t, severityOK) {
		assert.Equal(t, "WARN", severity)
	}

	var (
		message   string
		messageOK bool
	)

	for _, a := range attrs {
		if string(a.Key) == "log.message" {
			if v, ok := a.Value.AsInterface().(string); ok {
				message = v
				messageOK = true

				break
			}
		}
	}

	if assert.True(t, messageOK) {
		assert.Equal(t, "heads-up", message)
	}

	// returned fields include correlation ids equal to created span
	var (
		traceID   string
		traceIDOK bool
	)

	for _, field := range out {
		if field.Key == "traceID" {
			if v, ok := field.Value.Any().(string); ok {
				traceID = v
				traceIDOK = true

				break
			}
		}
	}

	require.True(t, traceIDOK)

	var (
		spanID   string
		spanIDOK bool
	)

	for _, field := range out {
		if field.Key == "spanID" {
			if v, ok := field.Value.Any().(string); ok {
				spanID = v
				spanIDOK = true

				break
			}
		}
	}

	require.True(t, spanIDOK)
	assert.Equal(t, ros.SpanContext().TraceID().String(), traceID)
	assert.Equal(t, ros.SpanContext().SpanID().String(), spanID)
}

func Test_NewTraceFromContext_Normalization_IsErrorAndStringErr_OnActiveSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider()
	tp.RegisterSpanProcessor(rec)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		err := tp.Shutdown(context.Background())
		require.NoError(t, err)
	})

	ctx, root := otel.Tracer("test").Start(context.Background(), "root")
	_, err := tracer.NewTraceFromContext(
		ctx, "INFO", "msg",
		nil,
		slog.String("is_error", "true"),
		slog.String("err", "oops"),
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

	var (
		isError   bool
		isErrorOK bool
	)

	for _, a := range logEv.Attributes {
		if string(a.Key) == "log.is_error" {
			if v, ok := a.Value.AsInterface().(bool); ok {
				isError = v
				isErrorOK = true

				break
			}
		}
	}

	if assert.True(t, isErrorOK) {
		assert.True(t, isError, "string 'true' should be coerced to bool true")
	}

	var (
		exceptionMessage   string
		exceptionMessageOK bool
	)

	for _, a := range logEv.Attributes {
		if string(a.Key) == "exception.message" {
			if v, ok := a.Value.AsInterface().(string); ok {
				exceptionMessage = v
				exceptionMessageOK = true

				break
			}
		}
	}

	if assert.True(t, exceptionMessageOK) {
		assert.Equal(t, "oops", exceptionMessage)
	}
}
