package logger

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWithFields(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	// Test WithFields
	userLogger := log.WithFields("user_id", "123", "component", "auth")
	userLogger.Info("User action", "action", "login")

	// Check that fields were added
	require.Contains(t, b.String(), `"user_id":"123"`)
	require.Contains(t, b.String(), `"component":"auth"`)
	require.Contains(t, b.String(), `"action":"login"`)
}

func TestWithError(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      ERROR_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	// Test WithError
	testErr := errors.New("test error")
	errorLogger := log.WithError(testErr)
	errorLogger.Error("Operation failed", "operation", "query")

	// Check that error was added
	require.Contains(t, b.String(), `"error":"test error"`)
	require.Contains(t, b.String(), `"operation":"query"`)
}

func TestWithTags(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
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
	require.Contains(t, b.String(), `"service":"api"`)
	require.Contains(t, b.String(), `"version":"1.0"`)
	require.Contains(t, b.String(), `"env":"production"`)
	require.Contains(t, b.String(), `"port":8080`)
}

func TestWithErrorNil(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	// Test WithError with nil error
	errorLogger := log.WithError(nil)
	errorLogger.Info("Message")

	// Should not add error field
	require.NotContains(t, b.String(), `"error"`)
}

func TestWithTagsEmpty(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	// Test WithTags with empty map
	emptyLogger := log.WithTags(map[string]string{})
	emptyLogger.Info("Message")

	// Should work without adding fields
	require.Contains(t, b.String(), `"Message"`)
}

func TestWithFieldsEmpty(t *testing.T) {
	var b bytes.Buffer
	conf := Configuration{
		Level:      INFO_LEVEL,
		Writer:     &b,
		TimeFormat: time.RFC822,
	}

	log, err := New(conf)
	require.NoError(t, err)

	// Test WithFields with no fields
	emptyLogger := log.WithFields()
	emptyLogger.Info("Message")

	// Should work without adding fields
	require.Contains(t, b.String(), `"Message"`)
}
