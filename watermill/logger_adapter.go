package watermill

import (
	"log/slog"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/shortlink-org/go-sdk/logger"
)

type watermillLoggerAdapter struct {
	log    logger.Logger
	fields watermill.LogFields
}

func NewWatermillLogger(log logger.Logger) watermill.LoggerAdapter {
	return &watermillLoggerAdapter{
		log:    log,
		fields: make(watermill.LogFields),
	}
}

func (l *watermillLoggerAdapter) With(fields watermill.LogFields) watermill.LoggerAdapter {
	// Merge new fields with existing ones
	merged := make(watermill.LogFields, len(l.fields)+len(fields))
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return &watermillLoggerAdapter{
		log:    l.log,
		fields: merged,
	}
}

func (l *watermillLoggerAdapter) mergeFields(fields watermill.LogFields) []slog.Attr {
	totalLen := len(l.fields) + len(fields)
	attrs := make([]slog.Attr, 0, totalLen)

	// Add base fields first, unless caller wants to override the key
	for k, v := range l.fields {
		if _, overridden := fields[k]; overridden {
			continue
		}
		attrs = append(attrs, slog.Any(k, v))
	}

	// Add call-specific fields (they override base fields with same key)
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}

	return attrs
}

func (l *watermillLoggerAdapter) Error(msg string, err error, fields watermill.LogFields) {
	attrs := l.mergeFields(fields)
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	l.log.Error(msg, attrs...)
}

func (l *watermillLoggerAdapter) Info(msg string, fields watermill.LogFields) {
	attrs := l.mergeFields(fields)
	l.log.Info(msg, attrs...)
}

func (l *watermillLoggerAdapter) Debug(msg string, fields watermill.LogFields) {
	attrs := l.mergeFields(fields)
	l.log.Debug(msg, attrs...)
}

func (l *watermillLoggerAdapter) Trace(msg string, fields watermill.LogFields) {
	l.Debug(msg, fields)
}
