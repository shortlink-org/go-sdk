package permission

import (
	"context"

	"github.com/authzed/authzed-go/v1"
	"github.com/shortlink-org/go-sdk/auth"
	rpc "github.com/shortlink-org/go-sdk/grpc"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/observability/metrics"
	"go.opentelemetry.io/otel/trace"
)

func New(_ context.Context, log logger.Logger, tracer trace.TracerProvider, monitor *metrics.Monitoring) (*authzed.Client, error) {
	// Initialize gRPC Client's interceptor.
	opts := []rpc.Option{
		rpc.WithSession(),
		rpc.WithMetrics(monitor.Prometheus),
		rpc.WithTracer(tracer, monitor.Prometheus, monitor.Metrics),
		rpc.WithTimeout(),
		rpc.WithLogger(log),
	}

	permission, err := auth.New(opts...)
	if err != nil {
		return nil, err
	}

	return permission, nil
}
