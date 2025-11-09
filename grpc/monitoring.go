package grpc

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

func exemplarFromContext(ctx context.Context) prometheus.Labels {
	span := trace.SpanContextFromContext(ctx)
	if span.IsSampled() && span.HasTraceID() {
		return prometheus.Labels{"trace_id": span.TraceID().String()}
	}

	return nil
}
