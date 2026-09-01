package logger

import (
	"log/slog"
)

// New builds the SDK logger: a standard-library *slog.Logger writing JSON
// through the OpenTelemetry-enriching handler.
//
// The SDK deliberately exposes no Logger interface of its own. Everything in
// go-sdk accepts *slog.Logger, so a consumer that already has one does not
// need this module at all -- it is only the handler that is worth importing.
func New(cfg Configuration) (*slog.Logger, error) {
	handler, err := NewHandler(cfg)
	if err != nil {
		return nil, err
	}

	return slog.New(handler), nil
}
