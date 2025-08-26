package logger

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

// TestOutputInfoWithContextSlog ...
func TestOutputInfoWithContextSlog(t *testing.T) {
	var b bytes.Buffer

	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err, "Error init a logger")

	log.InfoWithContext(context.Background(), "Hello World")

	expectedTime := time.Now().Format(time.RFC822)
	expected := map[string]any{
		"level": "INFO",
		"time":  expectedTime,
		"source": map[string]any{
			"file":     "/Users/user/myprojects/shortlink/go-sdk/logger/logger.go",
			"function": "github.com/shortlink-org/go-sdk/logger.(*SlogLogger).logWithContext",
			"line":     float64(71),
		},
		"msg": "Hello World",
	}
	var response map[string]any
	require.NoError(t, json.Unmarshal(b.Bytes(), &response), "Error unmarshalling")

	if !reflect.DeepEqual(expected, response) {
		assert.Equal(t, expected, response)
	}
}

func BenchmarkOutputSlog(bench *testing.B) {
	var b bytes.Buffer

	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, _ := New(conf)

	for i := 0; i < bench.N; i++ {
		log.Info("Hello World")
	}
}

func TestFieldsSlog(t *testing.T) {
	var b bytes.Buffer

	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err, "Error init a logger")

	log.InfoWithContext(context.Background(), "Hello World", "hello", "world", "first", 1)

	expectedTime := time.Now().Format(time.RFC822)
	expected := map[string]any{
		"level": "INFO",
		"time":  expectedTime,
		"msg":   "Hello World",
		"source": map[string]any{
			"file":     "/Users/user/myprojects/shortlink/go-sdk/logger/logger.go",
			"function": "github.com/shortlink-org/go-sdk/logger.(*SlogLogger).logWithContext",
			"line":     float64(71),
		},
		"first": float64(1),
		"hello": "world",
	}
	var response map[string]any
	require.NoError(t, json.Unmarshal(b.Bytes(), &response), "Error unmarshalling")

	if !reflect.DeepEqual(expected, response) {
		assert.Equal(t, expected, response)
	}
}

func TestSetLevel(t *testing.T) {
	var b bytes.Buffer

	conf := Configuration{
		Level:      ERROR_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err, "Error init a logger")

	// ERROR_LEVEL = 0, INFO_LEVEL = 2, so INFO logs should not appear
	log.Info("Hello World")

	expectedStr := ``

	if b.String() != expectedStr {
		assert.Errorf(t, err, "Expected: %sgot: %s", expectedStr, b.String())
	}
}

func TestDefaultConfig(t *testing.T) {
	conf := Default()

	// Test default values
	assert.Equal(t, os.Stdout, conf.Writer)
	assert.Equal(t, time.RFC3339Nano, conf.TimeFormat)
	assert.Equal(t, INFO_LEVEL, conf.Level)
}

func TestConfigValidation(t *testing.T) {
	// Test invalid level
	conf := Configuration{
		Level: 999, // Invalid level
	}

	err := conf.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidLogLevel)

	// Test valid level
	conf.Level = DEBUG_LEVEL
	err = conf.Validate()
	assert.NoError(t, err)
}

func TestError(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      ERROR_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	log.Error("Database error", "operation", "query", "table", "users")

	// Check that error was logged
	require.Contains(t, b.String(), `"level":"ERROR"`)
	require.Contains(t, b.String(), `"msg":"Database error"`)
	require.Contains(t, b.String(), `"operation":"query"`)
	require.Contains(t, b.String(), `"table":"users"`)
}

func TestWarn(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      WARN_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	log.Warn("High memory usage", "usage", "85%", "threshold", "80%")

	// Check that warning was logged
	require.Contains(t, b.String(), `"level":"WARN"`)
	require.Contains(t, b.String(), `"msg":"High memory usage"`)
	require.Contains(t, b.String(), `"usage":"85%"`)
	require.Contains(t, b.String(), `"threshold":"80%"`)
}

func TestDebug(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      DEBUG_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	log.Debug("Processing request", "headers", "content-type: application/json", "method", "POST")

	// Check that debug was logged
	require.Contains(t, b.String(), `"level":"DEBUG"`)
	require.Contains(t, b.String(), `"msg":"Processing request"`)
	require.Contains(t, b.String(), `"headers":"content-type: application/json"`)
	require.Contains(t, b.String(), `"method":"POST"`)
}

func TestErrorWithContext(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      ERROR_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), "request_id", "req-123")
	log.ErrorWithContext(ctx, "Request failed", "status", 500, "path", "/api/users")

	// Check that error with context was logged
	require.Contains(t, b.String(), `"level":"ERROR"`)
	require.Contains(t, b.String(), `"msg":"Request failed"`)
	require.Contains(t, b.String(), `"status":500`)
	require.Contains(t, b.String(), `"path":"/api/users"`)
	require.Contains(t, b.String(), `"error":true`)
	require.Contains(t, b.String(), `"traceID"`)
}

func TestWarnWithContext(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      WARN_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), "user_id", "user-456")
	log.WarnWithContext(ctx, "Slow query detected", "duration", "2.5s", "query", "SELECT * FROM users")

	// Check that warning with context was logged
	require.Contains(t, b.String(), `"level":"WARN"`)
	require.Contains(t, b.String(), `"msg":"Slow query detected"`)
	require.Contains(t, b.String(), `"duration":"2.5s"`)
	require.Contains(t, b.String(), `"query":"SELECT * FROM users"`)
	require.Contains(t, b.String(), `"traceID"`)
}

func TestDebugWithContext(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      DEBUG_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), "session_id", "sess-789")
	log.DebugWithContext(ctx, "Processing step", "step", "validation", "data_size", 1024)

	// Check that debug with context was logged
	require.Contains(t, b.String(), `"level":"DEBUG"`)
	require.Contains(t, b.String(), `"msg":"Processing step"`)
	require.Contains(t, b.String(), `"step":"validation"`)
	require.Contains(t, b.String(), `"data_size":1024`)
	require.Contains(t, b.String(), `"traceID"`)
}
