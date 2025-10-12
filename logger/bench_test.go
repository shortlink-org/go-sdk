package logger_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/shortlink-org/go-sdk/logger"
)

// benchContextKey is a type for context keys to avoid collisions.
type benchContextKey string

const benchRequestIDKey benchContextKey = "request_id"

func BenchmarkNew(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	b.ResetTimer()

	for range b.N {
		_, err := logger.New(conf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInfo(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := range b.N {
		log.Info("Benchmark message",
			slog.Int("iteration", i),
			slog.Time("timestamp", time.Now()),
			slog.String("request_id", "bench-123"),
		)
	}
}

func BenchmarkInfoContext(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	// Example context carrying some metadata; your tracer will pick up active spans if present.
	ctx := context.WithValue(context.Background(), benchRequestIDKey, "bench-ctx-123")

	b.ResetTimer()

	for i := range b.N {
		log.InfoWithContext(ctx, "Benchmark message (ctx)",
			slog.Int("iteration", i),
			slog.Time("timestamp", time.Now()),
			slog.String("request_id", "bench-ctx-123"),
		)
	}
}

func BenchmarkError(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.ERROR_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := range b.N {
		log.Error("Error message",
			slog.Int("iteration", i),
			slog.Int("error_code", 500),
			slog.String("request_id", "bench-err-123"),
		)
	}
}

func BenchmarkWarn(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.WARN_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := range b.N {
		log.Warn("Warning message",
			slog.Int("iteration", i),
			slog.String("memory_usage", "85%"),
			slog.String("request_id", "bench-warn-123"),
		)
	}
}

func BenchmarkDebug(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.DEBUG_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := range b.N {
		log.Debug("Debug message",
			slog.Int("iteration", i),
			slog.String("debug_info", "processing step"),
			slog.String("request_id", "bench-debug-123"),
		)
	}
}
