package tracer

import (
	"context"
	"fmt"
	"runtime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func NewTraceFromContext(
	ctx context.Context, //nolint:contextcheck,maintidx // contextcheck: ctx is not nil
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
// for set name of span
func getNameFunc() string {
	pc := make([]uintptr, 1)
	if n := runtime.Callers(3, pc); n > 0 {
		if f := runtime.FuncForPC(pc[0]); f != nil {
			return f.Name()
		}
	}

	return "log"
}

// FieldsToOpenTelemetry converts fields to OpenTelemetry attributes
func FieldsToOpenTelemetry(fields ...any) []attribute.KeyValue {
	if len(fields) == 0 {
		return nil
	}

	openTelemetryFields := make([]attribute.KeyValue, 0, len(fields)/2)

	// Process fields in pairs (key, value)
	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			break // Skip incomplete pairs
		}

		key, ok := fields[i].(string)
		if !ok {
			continue // Skip non-string keys
		}

		value := fields[i+1]
		switch v := value.(type) {
		case string:
			openTelemetryFields = append(openTelemetryFields, attribute.String(key, v))
		case bool:
			openTelemetryFields = append(openTelemetryFields, attribute.Bool(key, v))
		case int:
			openTelemetryFields = append(openTelemetryFields, attribute.Int(key, v))
		case int32:
			openTelemetryFields = append(openTelemetryFields, attribute.Int(key, int(v)))
		case int64:
			openTelemetryFields = append(openTelemetryFields, attribute.Int64(key, v))
		case error:
			openTelemetryFields = append(openTelemetryFields, attribute.String(key, v.Error()))
		default:
			// For other types, convert to string
			openTelemetryFields = append(openTelemetryFields, attribute.String(key, toString(v)))
		}
	}

	return openTelemetryFields
}

// toString converts any value to string
func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
