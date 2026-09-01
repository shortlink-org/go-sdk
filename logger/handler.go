package logger

import (
	"context"
	"log/slog"

	"github.com/shortlink-org/go-sdk/logger/tracer"
)

// otelHandler enriches records with OpenTelemetry correlation before handing
// them to the wrapped handler
type otelHandler struct {
	inner slog.Handler

	// preformatted mirrors the attrs passed to WithAttrs. The inner handler
	// already renders them; they are kept here only so the span event carries
	// the same attribute set the log line does
	preformatted []slog.Attr

	// grouped records that WithGroup was called, after which keys are
	// namespaced and can no longer be flattened into span attributes
	grouped bool
}

// NewHandler builds the SDK handler: a JSON handler with source and a
// configurable timestamp layout, wrapped in OpenTelemetry enrichment
//
//nolint:ireturn // slog.Handler is the stdlib contract this constructor implements
func NewHandler(cfg Configuration) (slog.Handler, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	inner := slog.NewJSONHandler(cfg.Writer, &slog.HandlerOptions{
		Level:     convertLevel(cfg.Level),
		AddSource: true,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey && attr.Value.Kind() == slog.KindTime {
				return slog.String(slog.TimeKey, attr.Value.Time().Format(cfg.TimeFormat))
			}

			return attr
		},
	})

	return &otelHandler{inner: inner, preformatted: nil, grouped: false}, nil
}

func (h *otelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle enriches only calls that carried a real context. The bare
// Info/Warn/Error/Debug methods pass context.Background() verbatim, so that
// sentinel tells a plain call apart from a traced one -- without the check
// every bare Error would start an orphan root span
//
//nolint:gocritic // slog.Handler.Handle takes the record by value; the signature is not ours
func (h *otelHandler) Handle(ctx context.Context, record slog.Record) error {
	if ctx == nil || ctx == context.Background() {
		return h.inner.Handle(ctx, record)
	}

	attrs := make([]slog.Attr, 0, len(h.preformatted)+record.NumAttrs())
	attrs = append(attrs, h.preformatted...)

	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)

		return true
	})

	enriched, err := tracer.NewTraceFromContext(ctx, levelString(record.Level), record.Message, nil, attrs...)
	if err != nil {
		return h.inner.Handle(ctx, record)
	}

	// The inner handler already renders the WithAttrs prefix, so only the
	// record's own attrs and whatever the tracer appended are re-emitted
	out := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	out.AddAttrs(enriched[len(h.preformatted):]...)

	return h.inner.Handle(ctx, out)
}

//nolint:ireturn // slog.Handler is the stdlib contract
func (h *otelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	next := &otelHandler{
		inner:        h.inner.WithAttrs(attrs),
		preformatted: h.preformatted,
		grouped:      h.grouped,
	}

	// Once a group is open the keys are namespaced, so the flat attribute
	// list can no longer be reconstructed for the span event
	if !h.grouped {
		next.preformatted = append(append([]slog.Attr{}, h.preformatted...), attrs...)
	}

	return next
}

//nolint:ireturn // slog.Handler is the stdlib contract
func (h *otelHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	return &otelHandler{
		inner:        h.inner.WithGroup(name),
		preformatted: h.preformatted,
		grouped:      true,
	}
}

// convertLevel converts our int level to slog.Level
func convertLevel(level int) slog.Level {
	switch level {
	case ERROR_LEVEL:
		return slog.LevelError
	case WARN_LEVEL:
		return slog.LevelWarn
	case INFO_LEVEL:
		return slog.LevelInfo
	case DEBUG_LEVEL:
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// levelString maps slog.Level to the severity string the tracer expects
func levelString(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
