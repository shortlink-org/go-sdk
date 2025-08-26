package logger

import "errors"

var (
	// ErrInvalidLoggerInstance is an error when logger instance is invalid
	ErrInvalidLoggerInstance = errors.New("invalid logger instance")

	// ErrInvalidLogLevel is an error when log level is invalid
	ErrInvalidLogLevel = errors.New("invalid log level")
)
