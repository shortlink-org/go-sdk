package logger

const (
	// fieldsMultiplier is used to calculate initial capacity for fields slice.
	fieldsMultiplier = 2
)

// WithFields creates a new logger with pre-set fields.
func (log *SlogLogger) WithFields(fields ...any) *SlogLogger {
	if len(fields) == 0 {
		return log
	}

	// Create a new logger with the base logger and additional fields
	newLogger := log.logger.With(fields...)

	return &SlogLogger{logger: newLogger}
}

// WithError creates a new logger with error field.
func (log *SlogLogger) WithError(err error) *SlogLogger {
	if err == nil {
		return log
	}

	return log.WithFields("error", err.Error())
}

// WithTags creates a new logger with multiple tags.
func (log *SlogLogger) WithTags(tags map[string]string) *SlogLogger {
	if len(tags) == 0 {
		return log
	}

	fields := make([]any, 0, len(tags)*fieldsMultiplier)

	for k, v := range tags {
		if k != "" && v != "" {
			fields = append(fields, k, v)
		}
	}

	if len(fields) == 0 {
		return log
	}

	return log.WithFields(fields...)
}
