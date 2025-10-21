package logger

import (
	"context"
	"time"

	"github.com/spf13/viper"
)

// New creates a new logger instance
//
//nolint:ireturn // It's made by design
func NewDefault(_ context.Context) (Logger, func(), error) {
	viper.SetDefault("LOG_LEVEL", INFO_LEVEL)
	viper.SetDefault("LOG_TIME_FORMAT", time.RFC3339Nano)

	conf := Configuration{
		Level:      viper.GetInt("LOG_LEVEL"),
		TimeFormat: viper.GetString("LOG_TIME_FORMAT"),
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
