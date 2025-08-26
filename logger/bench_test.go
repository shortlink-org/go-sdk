package logger

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func BenchmarkNew(b *testing.B) {
	conf := Configuration{
		Level:      INFO_LEVEL,
		TimeFormat: time.RFC3339,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := New(conf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInfo(b *testing.B) {
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Info("Benchmark message", "iteration", i, "timestamp", time.Now())
	}
}

func BenchmarkInfoWithContext(b *testing.B) {
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.WithValue(context.Background(), "request_id", "bench-123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.InfoWithContext(ctx, "Benchmark message", "iteration", i, "timestamp", time.Now())
	}
}

func BenchmarkWithFields(b *testing.B) {
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userLogger := log.WithFields("user_id", "123", "component", "benchmark")
		userLogger.Info("Message", "iteration", i)
	}
}

func BenchmarkWithError(b *testing.B) {
	conf := Configuration{
		Level:      ERROR_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	testErr := errors.New("benchmark error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errorLogger := log.WithError(testErr)
		errorLogger.Error("Error message", "iteration", i)
	}
}

func BenchmarkWithTags(b *testing.B) {
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	tags := map[string]string{
		"service": "benchmark",
		"version": "1.0",
		"env":     "test",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taggedLogger := log.WithTags(tags)
		taggedLogger.Info("Message", "iteration", i)
	}
}

func BenchmarkError(b *testing.B) {
	conf := Configuration{
		Level:      ERROR_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Error("Error message", "iteration", i, "error_code", 500)
	}
}

func BenchmarkWarn(b *testing.B) {
	conf := Configuration{
		Level:      WARN_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Warn("Warning message", "iteration", i, "memory_usage", "85%")
	}
}

func BenchmarkDebug(b *testing.B) {
	conf := Configuration{
		Level:      DEBUG_LEVEL,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	log, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Debug("Debug message", "iteration", i, "debug_info", "processing step")
	}
}
