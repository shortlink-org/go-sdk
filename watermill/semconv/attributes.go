// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/watermill.yaml. Regenerate with `make -C semconv generate`.

// Package semconv holds the attribute keys and metric instruments of the
// watermill module as declared in the go-sdk semantic convention registry.
package semconv

import "go.opentelemetry.io/otel/attribute"

const (
	// ErrorKey is the "error" attribute key. The error message of the failed
	// operation, truncated to 128 characters.
	//
	// Type: string
	// Stability: Development
	// Note: Free text with unbounded cardinality. Kept for compatibility with the
	// existing dashboards; prefer `stage` and the span's recorded error.
	ErrorKey = attribute.Key("error")

	// OutcomeKey is the "outcome" attribute key. How the operation ended. The set
	// of values is listed on each metric that records it.
	//
	// Type: string
	// Stability: Development
	OutcomeKey = attribute.Key("outcome")

	// SpanIDKey is the "span_id" attribute key. The span id of the span active when
	// the measurement was taken.
	//
	// Type: string
	// Stability: Development
	// Note: Recorded only when the context carries a valid span. See `trace_id`.
	SpanIDKey = attribute.Key("span_id")

	// StageKey is the "stage" attribute key. Which side of the message flow the
	// failed operation belongs to.
	//
	// Type: Enum
	// Stability: Development
	StageKey = attribute.Key("stage")

	// TopicKey is the "topic" attribute key. The Watermill topic the message was
	// published to or consumed from.
	//
	// Type: string
	// Stability: Development
	TopicKey = attribute.Key("topic")

	// TraceIDKey is the "trace_id" attribute key. The trace id of the span active
	// when the measurement was taken.
	//
	// Type: string
	// Stability: Development
	// Note: Recorded only when the context carries a valid span. Exemplars carry
	// the same information without widening the time series; this attribute stays
	// for the dashboards that already query it.
	TraceIDKey = attribute.Key("trace_id")
)

// Error returns a "error" attribute. The error message of the failed operation,
// truncated to 128 characters.
func Error(val string) attribute.KeyValue {
	return ErrorKey.String(val)
}

// Outcome returns a "outcome" attribute. How the operation ended. The set of
// values is listed on each metric that records it.
func Outcome(val string) attribute.KeyValue {
	return OutcomeKey.String(val)
}

// SpanID returns a "span_id" attribute. The span id of the span active when the
// measurement was taken.
func SpanID(val string) attribute.KeyValue {
	return SpanIDKey.String(val)
}

// Topic returns a "topic" attribute. The Watermill topic the message was
// published to or consumed from.
func Topic(val string) attribute.KeyValue {
	return TopicKey.String(val)
}

// TraceID returns a "trace_id" attribute. The trace id of the span active when
// the measurement was taken.
func TraceID(val string) attribute.KeyValue {
	return TraceIDKey.String(val)
}

// StageAttr is a value of the "stage" attribute.
type StageAttr string

const (
	// The failure happened while publishing.
	// Stability: Development
	StagePublish StageAttr = "publish"
	// The failure happened in a message handler.
	// Stability: Development
	StageConsume StageAttr = "consume"
)

// Stage returns a "stage" attribute.
func Stage(val StageAttr) attribute.KeyValue {
	return StageKey.String(string(val))
}
