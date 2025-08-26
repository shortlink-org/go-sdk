package config

// Logger is our contract for the logger
type Logger interface {
	Warn(msg string, fields ...any)
}
