package logger

import (
	"context"
	"log/slog"
)

// Warn ================================================================================================================

func (log *SlogLogger) Warn(msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(context.Background(), slog.LevelWarn, msg, fields...)
}

func (log *SlogLogger) WarnWithContext(ctx context.Context, msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(ctx, slog.LevelWarn, msg, fields...)
}

// Error ===============================================================================================================

func (log *SlogLogger) Error(msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(context.Background(), slog.LevelError, msg, fields...)
}

func (log *SlogLogger) ErrorWithContext(ctx context.Context, msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(ctx, slog.LevelError, msg, fields...)
}

// Info ================================================================================================================

func (log *SlogLogger) Info(msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(context.Background(), slog.LevelInfo, msg, fields...)
}

func (log *SlogLogger) InfoWithContext(ctx context.Context, msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(ctx, slog.LevelInfo, msg, fields...)
}

// Debug ===============================================================================================================

func (log *SlogLogger) Debug(msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(context.Background(), slog.LevelDebug, msg, fields...)
}

func (log *SlogLogger) DebugWithContext(ctx context.Context, msg string, fields ...slog.Attr) {
	log.logger.LogAttrs(ctx, slog.LevelDebug, msg, fields...)
}
