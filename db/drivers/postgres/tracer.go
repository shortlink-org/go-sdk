package postgres

import (
	"go.opentelemetry.io/otel/trace"
)

// Tracer is the tracer provider the store hands to otelpgx, which emits the
// query spans and their database attributes. A nil provider leaves the pool
// uninstrumented.
type Tracer struct {
	trace.TracerProvider
}
