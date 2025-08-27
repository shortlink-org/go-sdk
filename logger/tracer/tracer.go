package tracer

import (
	"context"
	"fmt"
	"runtime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// callersSkip is the number of callers to skip when getting function name.
	callersSkip = 3

	// fieldsDivisor is used to calculate initial capacity for OpenTelemetry fields.
	fieldsDivisor = 2
)

func NewTraceFromContext(
	ctx context.Context, //nolint:contextcheck // contextcheck: ctx is not nil
	msg string,
	tags []attribute.KeyValue,
	fields ...any,
) ([]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	_, span := otel.Tracer("logger").Start(ctx, getNameFunc())
	defer span.End()

	span.SetAttributes(FieldsToOpenTelemetry(fields...)...)
	span.SetAttributes(attribute.String("log", msg))
	span.SetAttributes(tags...)

	// Get span ID and add traceID to fields
	traceFields := []any{"traceID", span.SpanContext().TraceID().String()}

	// Combine original fields with traceID
	result := make([]any, 0, len(fields)+len(traceFields))
	result = append(result, fields...)
	result = append(result, traceFields...)

	return result, nil
}

// getNameFunc returns the name of the function calling this package
// for set name of span.
func getNameFunc() string {
	pc := make([]uintptr, 1)
	if n := runtime.Callers(callersSkip, pc); n > 0 {
		if f := runtime.FuncForPC(pc[0]); f != nil {
			return f.Name()
		}
	}

	return "log"
}

// FieldsToOpenTelemetry converts fields to OpenTelemetry attributes.
func FieldsToOpenTelemetry(fields ...any) []attribute.KeyValue {
	if len(fields) == 0 {
		return nil
	}

	openTelemetryFields := make([]attribute.KeyValue, 0, len(fields)/fieldsDivisor)

	// Process fields in pairs (key, value)
	for idx := 0; idx < len(fields); idx += 2 {
		if idx+1 >= len(fields) {
			break // Skip incomplete pairs
		}

		key, ok := fields[idx].(string)
		if !ok {
			continue // Skip non-string keys
		}

		value := fields[idx+1]
		switch val := value.(type) {
		case string:
			openTelemetryFields = append(openTelemetryFields, attribute.String(key, val))
		case bool:
			openTelemetryFields = append(openTelemetryFields, attribute.Bool(key, val))
		case int:
			openTelemetryFields = append(openTelemetryFields, attribute.Int(key, val))
		case int32:
			openTelemetryFields = append(openTelemetryFields, attribute.Int(key, int(val)))
		case int64:
			openTelemetryFields = append(openTelemetryFields, attribute.Int64(key, val))
		case error:
			openTelemetryFields = append(openTelemetryFields, attribute.String(key, val.Error()))
		default:
			// For other types, convert to string
			openTelemetryFields = append(openTelemetryFields, attribute.String(key, toString(val)))
		}
	}

	return openTelemetryFields
}

// toString converts any value to string.
func toString(v any) string {
	if v == nil {
		return ""
	}

	return fmt.Sprintf("%v", v)
}
