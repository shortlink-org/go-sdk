package config

// Logger is our contract for the logger
type Logger interface {
	Warn(msg string, fields ...Fields)
}

type Fields map[string]any
