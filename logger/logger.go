package logger

import (
	"context"
	"log/slog"
	"time"

	"github.com/shortlink-org/go-sdk/logger/tracer"
)

type SlogLogger struct {
	logger *slog.Logger
}

func New(cfg Configuration) (*SlogLogger, error) {
	// Check config and set default values if needed
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	// Create slog handler with JSON format
	handler := slog.NewJSONHandler(cfg.Writer, &slog.HandlerOptions{
		Level:     convertLevel(cfg.Level),
		AddSource: true, // Always include source location
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize timestamp format
			if a.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, time.Now().Format(cfg.TimeFormat))
			}
			return a
		},
	})

	logger := slog.New(handler)
	return &SlogLogger{logger: logger}, nil
}

func (log *SlogLogger) Close() error {
	// slog.Logger doesn't have a Close method, so we just return nil
	return nil
}

// convertLevel converts our log level to slog level
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

// logWithContext is a helper function to reduce code duplication
func (log *SlogLogger) logWithContext(ctx context.Context, level slog.Level, msg string, fields ...any) {
	// Add tracing if context is provided
	if ctx != nil && ctx != context.Background() {
		var err error
		fields, err = tracer.NewTraceFromContext(ctx, msg, nil, fields...)
		if err != nil {
			log.logger.ErrorContext(ctx, "Error sending span to OpenTelemetry: "+err.Error())
		}
	}

	log.logger.Log(ctx, level, msg, fields...)
}
