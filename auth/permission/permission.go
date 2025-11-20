package permission

import (
	"github.com/authzed/authzed-go/v1"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/auth"
	"github.com/shortlink-org/go-sdk/config"
	rpc "github.com/shortlink-org/go-sdk/grpc"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/observability/metrics"
)

func New(log logger.Logger, tracer trace.TracerProvider, monitor *metrics.Monitoring, cfg *config.Config) (*authzed.Client, error) {
	// Initialize gRPC Client's interceptor.
	opts := []rpc.Option{
		rpc.WithSession(),
		rpc.WithMetrics(monitor.Prometheus),
		rpc.WithTracer(tracer, monitor.Prometheus, monitor.Metrics),
		rpc.WithTimeout(),
		rpc.WithLogger(log),
	}

	permission, err := auth.New(cfg, opts...)
	if err != nil {
		return nil, err
	}

	return permission, nil
}
