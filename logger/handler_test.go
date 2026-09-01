package logger_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/shortlink-org/go-sdk/logger"
)

// statusOK is an arbitrary attribute value, used only to prove attributes
// survive the round trip through the handler
const statusOK = 200

// NewRecordedProvider installs a recording TracerProvider for the duration
// of the test and hands back the recorder
func newRecordedProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	rec := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(rec)
	otel.SetTracerProvider(provider)

	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	return rec
}

func newBufferedHandler(t *testing.T, buffer *bytes.Buffer) slog.Handler {
	t.Helper()

	handler, err := logger.NewHandler(logger.Configuration{
		Level:      logger.DEBUG_LEVEL,
		Writer:     buffer,
		TimeFormat: time.RFC3339Nano,
	})
	require.NoError(t, err)

	return handler
}

func decode(t *testing.T, buffer *bytes.Buffer) map[string]any {
	t.Helper()

	var out map[string]any
	require.NoError(t, json.Unmarshal(buffer.Bytes(), &out))

	return out
}

// TestHandlerBareCallCreatesNoSpan pins the guard that keeps bare calls
// untraced. Handle receives the background context from slog, and treating
// that as a traced call would mint an orphan root span for every error line
func TestHandlerBareCallCreatesNoSpan(t *testing.T) {
	rec := newRecordedProvider(t)

	var buffer bytes.Buffer

	log := slog.New(newBufferedHandler(t, &buffer))
	log.Error("boom", slog.String("k", "v"))

	assert.Empty(t, rec.Ended(), "a bare Error must not create a span")

	response := decode(t, &buffer)
	assert.Equal(t, "ERROR", response["level"])
	assert.Equal(t, "v", response["k"])
	assert.NotContains(t, response, "traceID")
}

// TestHandlerBackgroundContextCreatesNoSpan pins the same guard for an
// explicit background context, which is indistinguishable from a bare call
func TestHandlerBackgroundContextCreatesNoSpan(t *testing.T) {
	rec := newRecordedProvider(t)

	var buffer bytes.Buffer

	log := slog.New(newBufferedHandler(t, &buffer))
	log.ErrorContext(context.Background(), "boom")

	assert.Empty(t, rec.Ended())
	assert.NotContains(t, decode(t, &buffer), "traceID")
}

func TestHandlerAddsCorrelationToActiveSpan(t *testing.T) {
	rec := newRecordedProvider(t)

	var buffer bytes.Buffer

	log := slog.New(newBufferedHandler(t, &buffer))

	ctx, root := otel.Tracer("test").Start(context.Background(), "root")
	log.InfoContext(ctx, "handled", slog.Int("status", statusOK))
	root.End()

	response := decode(t, &buffer)
	assert.Equal(t, root.SpanContext().TraceID().String(), response["traceID"])
	assert.Equal(t, root.SpanContext().SpanID().String(), response["spanID"])
	assert.InDelta(t, float64(statusOK), response["status"], 0)

	ended := rec.Ended()
	require.Len(t, ended, 1, "the existing span is reused, not replaced")
	require.Len(t, ended[0].Events(), 1)
	assert.Equal(t, "log.INFO", ended[0].Events()[0].Name)
}

// TestHandlerWithAttrsKeepsEnrichmentAndDoesNotDuplicate pins that the
// wrapper survives WithAttrs. Handing back the inner handler would silently
// disable enrichment for every logger derived through With
func TestHandlerWithAttrsKeepsEnrichmentAndDoesNotDuplicate(t *testing.T) {
	newRecordedProvider(t)

	var buffer bytes.Buffer

	log := slog.New(newBufferedHandler(t, &buffer)).With(slog.String("service", "test"))

	ctx, root := otel.Tracer("test").Start(context.Background(), "root")
	log.InfoContext(ctx, "handled", slog.Int("status", statusOK))
	root.End()

	assert.Equal(t, 1, bytes.Count(buffer.Bytes(), []byte(`"service"`)),
		"the WithAttrs prefix must be rendered exactly once")

	response := decode(t, &buffer)
	assert.Equal(t, "test", response["service"])
	assert.Equal(t, root.SpanContext().TraceID().String(), response["traceID"])
}

func TestHandlerWithGroupKeepsEnrichment(t *testing.T) {
	newRecordedProvider(t)

	var buffer bytes.Buffer

	log := slog.New(newBufferedHandler(t, &buffer)).WithGroup("http")

	ctx, root := otel.Tracer("test").Start(context.Background(), "root")
	log.InfoContext(ctx, "handled", slog.Int("status", statusOK))
	root.End()

	response := decode(t, &buffer)
	group, ok := response["http"].(map[string]any)
	require.True(t, ok, "the group must still be rendered")
	assert.InDelta(t, float64(statusOK), group["status"], 0)
	assert.Equal(t, root.SpanContext().TraceID().String(), group["traceID"])
}

func TestHandlerRejectsInvalidLevel(t *testing.T) {
	_, err := logger.NewHandler(logger.Configuration{
		Level:      -1,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339Nano,
	})
	require.ErrorIs(t, err, logger.ErrInvalidLogLevel)
}
