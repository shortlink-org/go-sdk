// Code generated from the go-sdk semantic convention registry. DO NOT EDIT.
//
// Source: semconv/registry/cqrs.yaml. Regenerate with `make -C semconv generate`.

// Package semconv holds the attribute keys and metric instruments of the
// cqrs module as declared in the go-sdk semantic convention registry.
package semconv

import "go.opentelemetry.io/otel/attribute"

const (
	// ForwarderNameKey is the "forwarder_name" attribute key. The name the outbox
	// forwarder was registered under.
	//
	// Type: string
	// Stability: Development
	ForwarderNameKey = attribute.Key("forwarder_name")
)

// ForwarderName returns a "forwarder_name" attribute. The name the outbox
// forwarder was registered under.
func ForwarderName(val string) attribute.KeyValue {
	return ForwarderNameKey.String(val)
}
