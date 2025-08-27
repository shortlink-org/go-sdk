package logger_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/logger"
)

// ErrTest is an error used in tests.
var ErrTest = errors.New("test error")

func TestWithFields(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	// Test WithFields
	userLogger := log.WithFields("user_id", "123", "component", "auth")
	userLogger.Info("User action", "action", "login")

	// Check that fields were added
	require.Contains(t, buffer.String(), `"user_id":"123"`)
	require.Contains(t, buffer.String(), `"component":"auth"`)
	require.Contains(t, buffer.String(), `"action":"login"`)
}

func TestWithError(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.ERROR_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	// Test WithError
	errorLogger := log.WithError(ErrTest)
	errorLogger.Error("Operation failed", "operation", "query")

	// Check that error was added
	require.Contains(t, buffer.String(), `"error":"test error"`)
	require.Contains(t, buffer.String(), `"operation":"query"`)
}

func TestWithTags(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	// Test WithTags
	tags := map[string]string{
		"service": "api",
		"version": "1.0",
		"env":     "production",
	}
	taggedLogger := log.WithTags(tags)
	taggedLogger.Info("Service started", "port", 8080)

	// Check that tags were added
	require.Contains(t, buffer.String(), `"service":"api"`)
	require.Contains(t, buffer.String(), `"version":"1.0"`)
	require.Contains(t, buffer.String(), `"env":"production"`)
	require.Contains(t, buffer.String(), `"port":8080`)
}

func TestWithErrorNil(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	// Test WithError with nil error
	errorLogger := log.WithError(nil)
	errorLogger.Info("Message")

	// Should not add error field
	require.NotContains(t, buffer.String(), `"error"`)
}

func TestWithTagsEmpty(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	// Test WithTags with empty map
	emptyLogger := log.WithTags(map[string]string{})
	emptyLogger.Info("Message")

	// Should work without adding fields
	require.Contains(t, buffer.String(), `"Message"`)
}

func TestWithFieldsEmpty(t *testing.T) {
	var buffer bytes.Buffer

	conf := logger.Configuration{
		Level:      logger.INFO_LEVEL,
		Writer:     &buffer,
		TimeFormat: time.RFC822,
	}

	log, err := logger.New(conf)
	require.NoError(t, err)

	// Test WithFields with no fields
	emptyLogger := log.WithFields()
	emptyLogger.Info("Message")

	// Should work without adding fields
	require.Contains(t, buffer.String(), `"Message"`)
}
