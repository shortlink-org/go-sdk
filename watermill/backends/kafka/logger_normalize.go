package kafka

import "log/slog"

// orDiscard substitutes a discarding logger for a nil one. A missing logger
// silences output rather than failing the call: logging is not a
// precondition for talking to Kafka
func orDiscard(log *slog.Logger) *slog.Logger {
	if log == nil {
		return slog.New(slog.DiscardHandler)
	}

	return log
}
