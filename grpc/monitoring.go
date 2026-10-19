package grpc

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

// handlingTimeBuckets are the histogram buckets (seconds) shared by the client and server handling-time metrics.
var handlingTimeBuckets = []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}

func exemplarFromContext(ctx context.Context) prometheus.Labels {
	span := trace.SpanContextFromContext(ctx)
	if span.IsSampled() && span.HasTraceID() {
		return prometheus.Labels{"trace_id": span.TraceID().String()}
	}

	return nil
}
