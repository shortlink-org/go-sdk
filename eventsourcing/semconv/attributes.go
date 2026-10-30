// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/eventsourcing.yaml. Regenerate with `make -C semconv generate`.

// Package semconv holds the attribute keys and metric instruments of the
// eventsourcing module as declared in the go-sdk semantic convention registry.
package semconv

import "go.opentelemetry.io/otel/attribute"

const (
	// AggregateIDKey is the "aggregate_id" attribute key. The identifier of the
	// aggregate. Recorded on spans only, its cardinality is unbounded.
	//
	// Type: string
	// Stability: Development
	AggregateIDKey = attribute.Key("aggregate_id")

	// AggregateTypeKey is the "aggregate_type" attribute key. The type of the
	// aggregate.
	//
	// Type: string
	// Stability: Development
	AggregateTypeKey = attribute.Key("aggregate_type")

	// EventIDKey is the "event_id" attribute key. The identifier of the event.
	// Recorded on spans only, its cardinality is unbounded.
	//
	// Type: string
	// Stability: Development
	EventIDKey = attribute.Key("event_id")
)

// AggregateID returns a "aggregate_id" attribute. The identifier of the
// aggregate. Recorded on spans only, its cardinality is unbounded.
func AggregateID(val string) attribute.KeyValue {
	return AggregateIDKey.String(val)
}

// AggregateType returns a "aggregate_type" attribute. The type of the aggregate.
func AggregateType(val string) attribute.KeyValue {
	return AggregateTypeKey.String(val)
}

// EventID returns a "event_id" attribute. The identifier of the event. Recorded
// on spans only, its cardinality is unbounded.
func EventID(val string) attribute.KeyValue {
	return EventIDKey.String(val)
}
