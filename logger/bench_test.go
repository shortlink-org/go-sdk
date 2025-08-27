package logger_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/shortlink-org/go-sdk/logger"
)

// ErrBenchmark is an error used in benchmarks.
var ErrBenchmark = errors.New("benchmark error")

// benchContextKey is a type for context keys to avoid collisions.
type benchContextKey string

const (
	benchRequestIDKey benchContextKey = "request_id"
)

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

	for idx := range b.N {
		log.Info("Benchmark message", "iteration", idx, "timestamp", time.Now())
	}
}

func BenchmarkInfoWithContext(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.WithValue(context.Background(), benchRequestIDKey, "bench-123")

	b.ResetTimer()

	for idx := range b.N {
		log.InfoWithContext(ctx, "Benchmark message", "iteration", idx, "timestamp", time.Now())
	}
}

func BenchmarkWithFields(b *testing.B) {
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

	for idx := range b.N {
		userLogger := log.WithFields("user_id", "123", "component", "benchmark")
		userLogger.Info("Message", "iteration", idx)
	}
}

func BenchmarkWithError(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.ERROR_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	testErr := ErrBenchmark

	b.ResetTimer()

	for idx := range b.N {
		errorLogger := log.WithError(testErr)
		errorLogger.Error("Error message", "iteration", idx)
	}
}

func BenchmarkWithTags(b *testing.B) {
	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := logger.New(conf)
	if err != nil {
		b.Fatal(err)
	}

	tags := map[string]string{
		"service": "benchmark",
		"version": "1.0",
		"env":     "test",
	}

	b.ResetTimer()

	for idx := range b.N {
		taggedLogger := log.WithTags(tags)
		taggedLogger.Info("Message", "iteration", idx)
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

	for idx := range b.N {
		log.Error("Error message", "iteration", idx, "error_code", 500)
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

	for idx := range b.N {
		log.Warn("Warning message", "iteration", idx, "memory_usage", "85%")
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

	for idx := range b.N {
		log.Debug("Debug message", "iteration", idx, "debug_info", "processing step")
	}
}
