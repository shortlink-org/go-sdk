package watermill

import "log/slog"

// orDiscard substitutes a discarding logger for a nil one, so that nothing
// downstream has to guard against nil. Logging is not a precondition for
// publishing a message, so a missing logger silences output rather than
// failing the call
func orDiscard(log *slog.Logger) *slog.Logger {
	if log == nil {
		return slog.New(slog.DiscardHandler)
	}

	return log
}
