package logger

import (
	"log/slog"
)

type SlogLogger struct {
	logger *slog.Logger
}

func New(cfg Configuration) (*SlogLogger, error) {
	handler, err := NewHandler(cfg)
	if err != nil {
		return nil, err
	}

	return &SlogLogger{logger: slog.New(handler)}, nil
}

func (log *SlogLogger) Close() error {
	// slog has nothing to close
	return nil
}
