package logger

import (
	"context"
	"io"
	"log/slog"
)

// Logger is our contract for the logger.
type Logger interface {
	Error(msg string, fields ...slog.Attr)
	ErrorWithContext(ctx context.Context, msg string, fields ...slog.Attr)

	Warn(msg string, fields ...slog.Attr)
	WarnWithContext(ctx context.Context, msg string, fields ...slog.Attr)

	Info(msg string, fields ...slog.Attr)
	InfoWithContext(ctx context.Context, msg string, fields ...slog.Attr)

	Debug(msg string, fields ...slog.Attr)
	DebugWithContext(ctx context.Context, msg string, fields ...slog.Attr)

	// Closer is the interface that wraps the basic Close method.
	io.Closer
}
