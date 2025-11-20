package logger

import (
	"context"
	"time"

	"github.com/shortlink-org/go-sdk/config"
)

// New creates a new logger instance using the provided configuration.
//
//nolint:ireturn // It's made by design
func NewDefault(_ context.Context, cfg *config.Config) (Logger, func(), error) {
	cfg.SetDefault("LOG_LEVEL", INFO_LEVEL)
	cfg.SetDefault("LOG_TIME_FORMAT", time.RFC3339Nano)

	conf := Configuration{
		Level:      cfg.GetInt("LOG_LEVEL"),
		TimeFormat: cfg.GetString("LOG_TIME_FORMAT"),
	}

	log, err := New(conf)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		// flushes buffer, if any
		_ = log.Close() //nolint:errcheck // ignore
	}

	return log, cleanup, nil
}
