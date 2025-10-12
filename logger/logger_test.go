package logger_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/logger"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userIDKey    contextKey = "user_id"
	sessionIDKey contextKey = "session_id"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
	os.Exit(m.Run())
}

// TestOutputInfoWithContextSlog ...
func TestOutputInfoWithContextSlog(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err, "Error init a logger")

	log.InfoWithContext(context.Background(), "Hello World")

	expectedTime := time.Now().Format(time.RFC822)

	var response map[string]any
	require.NoError(t, json.Unmarshal(buffer.Bytes(), &response), "Error unmarshalling")

	require.Equal(t, "INFO", response["level"])
	require.Equal(t, expectedTime, response["time"])
	require.Equal(t, "Hello World", response["msg"])

	// Flexible source assertions
	src, ok := response["source"].(map[string]any)
	require.True(t, ok, "source should be an object")

	file, ok := src["file"].(string)
	require.True(t, ok, "source.file should be a string")
	assert.True(t, strings.HasSuffix(file, "logger/logger.go"),
		"unexpected source.file suffix: %s", file)

	fun, _ := src["function"].(string)
	assert.Contains(t, fun, "SlogLogger")
	assert.Contains(t, fun, "logWithContext")

	if ln, ok := src["line"].(float64); ok {
		assert.Greater(t, ln, float64(0))
	}
}

func BenchmarkOutputSlog(bench *testing.B) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, _ := logger.New(conf)

	for range bench.N {
		log.Info("Hello World")
	}
}

func TestFieldsSlog(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err, "Error init a logger")

	log.InfoWithContext(context.Background(), "Hello World", slog.String("hello", "world"), slog.Int("first", 1))

	expectedTime := time.Now().Format(time.RFC822)

	var response map[string]any
	require.NoError(t, json.Unmarshal(buffer.Bytes(), &response), "Error unmarshalling")

	require.Equal(t, "INFO", response["level"])
	require.Equal(t, expectedTime, response["time"])
	require.Equal(t, "Hello World", response["msg"])
	require.Equal(t, "world", response["hello"])
	require.Equal(t, float64(1), response["first"])

	// Flexible source assertions
	src, ok := response["source"].(map[string]any)
	require.True(t, ok, "source should be an object")

	file, ok := src["file"].(string)
	require.True(t, ok, "source.file should be a string")
	assert.True(t, strings.HasSuffix(file, "logger/logger.go"),
		"unexpected source.file suffix: %s", file)

	fun, _ := src["function"].(string)
	assert.Contains(t, fun, "SlogLogger")
	assert.Contains(t, fun, "logWithContext")

	if ln, ok := src["line"].(float64); ok {
		assert.Greater(t, ln, float64(0))
	}
}

func TestSetLevel(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.ERROR_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err, "Error init a logger")

	// ERROR_LEVEL = 0, INFO_LEVEL = 2, so INFO logs should not appear
	log.Info("Hello World")

	expectedStr := ``
	assert.Equal(t, expectedStr, buffer.String())
}

func TestDefaultConfig(t *testing.T) {
	conf := logger.Default()

	assert.Equal(t, os.Stdout, conf.Writer)
	assert.Equal(t, time.RFC3339Nano, conf.TimeFormat)
	assert.Equal(t, logger.INFO_LEVEL, conf.Level)
}

func TestConfigValidation(t *testing.T) {
	conf := logger.Configuration{
		Level:      999,
		Writer:     io.Discard,
		TimeFormat: time.RFC3339,
	}

	err := conf.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, logger.ErrInvalidLogLevel)

	conf.Level = logger.DEBUG_LEVEL
	err = conf.Validate()
	assert.NoError(t, err)
}

func TestError(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.ERROR_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	log.Error("Database error", slog.String("operation", "query"), slog.String("table", "users"))

	require.Contains(t, buffer.String(), `"level":"ERROR"`)
	require.Contains(t, buffer.String(), `"msg":"Database error"`)
	require.Contains(t, buffer.String(), `"operation":"query"`)
	require.Contains(t, buffer.String(), `"table":"users"`)
}

func TestWarn(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.WARN_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	log.Warn("High memory usage", slog.String("usage", "85%"), slog.String("threshold", "80%"))

	require.Contains(t, buffer.String(), `"level":"WARN"`)
	require.Contains(t, buffer.String(), `"msg":"High memory usage"`)
	require.Contains(t, buffer.String(), `"usage":"85%"`)
	require.Contains(t, buffer.String(), `"threshold":"80%"`)
}

func TestDebug(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.DEBUG_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	log.Debug("Processing request", slog.String("headers", "content-type: application/json"), slog.String("method", "POST"))

	require.Contains(t, buffer.String(), `"level":"DEBUG"`)
	require.Contains(t, buffer.String(), `"msg":"Processing request"`)
	require.Contains(t, buffer.String(), `"headers":"content-type: application/json"`)
	require.Contains(t, buffer.String(), `"method":"POST"`)
}

func TestErrorWithContext(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.ERROR_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), requestIDKey, "req-123")
	log.ErrorWithContext(ctx, "Request failed", slog.Int("status", 500), slog.String("path", "/api/users"))

	require.Contains(t, buffer.String(), `"level":"ERROR"`)
	require.Contains(t, buffer.String(), `"msg":"Request failed"`)
	require.Contains(t, buffer.String(), `"status":500`)
	require.Contains(t, buffer.String(), `"path":"/api/users"`)
	require.Contains(t, buffer.String(), `"error":true`)
	require.Contains(t, buffer.String(), `"traceID"`)
}

func TestWarnWithContext(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.WARN_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), userIDKey, "user-456")
	log.WarnWithContext(ctx, "Slow query detected", slog.String("duration", "2.5s"), slog.String("query", "SELECT * FROM users"))

	require.Contains(t, buffer.String(), `"level":"WARN"`)
	require.Contains(t, buffer.String(), `"msg":"Slow query detected"`)
	require.Contains(t, buffer.String(), `"duration":"2.5s"`)
	require.Contains(t, buffer.String(), `"query":"SELECT * FROM users"`)
	require.Contains(t, buffer.String(), `"traceID"`)
}

func TestDebugWithContext(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.DEBUG_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), sessionIDKey, "sess-789")
	log.DebugWithContext(ctx, "Processing step", slog.String("step", "validation"), slog.Int("data_size", 1024))

	require.Contains(t, buffer.String(), `"level":"DEBUG"`)
	require.Contains(t, buffer.String(), `"msg":"Processing step"`)
	require.Contains(t, buffer.String(), `"step":"validation"`)
	require.Contains(t, buffer.String(), `"data_size":1024`)
	require.Contains(t, buffer.String(), `"traceID"`)
}
