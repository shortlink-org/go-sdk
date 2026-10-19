package logger

import (
	"io"
	"os"
	"time"
)

// The severity levels. Higher values are more considered more important.
const (
	// ERROR_LEVEL level. Logs. Used for errors that should definitely be noted.
	// Commonly used for hooks to send errors to an error tracking service.
	ERROR_LEVEL int = iota
	// WARN_LEVEL level. Non-critical entries that deserve eyes.
	WARN_LEVEL
	// INFO_LEVEL level. General operational entries about what's going on inside the
	// application.
	INFO_LEVEL
	// DEBUG_LEVEL level. Usually only enabled when debugging. Very verbose logging.
	DEBUG_LEVEL
)

// Configuration - options for logger.
type Configuration struct {
	Writer     io.Writer
	TimeFormat string
	Level      int
}

func (c *Configuration) Validate() error {
	if c.Writer == nil {
		c.Writer = os.Stdout
	}

	if c.TimeFormat == "" {
		c.TimeFormat = time.RFC3339Nano
	}

	if c.Level < ERROR_LEVEL || c.Level > DEBUG_LEVEL {
		return ErrInvalidLogLevel
	}

	return nil
}

// Default returns a default configuration.
func Default() Configuration {
	return Configuration{
		Writer:     os.Stdout,
		TimeFormat: time.RFC3339Nano,
		Level:      INFO_LEVEL,
	}
}
