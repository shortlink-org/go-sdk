// Package temporal provides dependency injection for Temporal client.
// It uses the official Temporal Go SDK (go.temporal.io/sdk) and integrates
// with go-sdk/grpc for consistent gRPC configuration across services.
//
// Reference: https://docs.temporal.io/develop/go
package temporal

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/log"

	"github.com/shortlink-org/go-sdk/config"
	sdkgrpc "github.com/shortlink-org/go-sdk/grpc"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/observability/metrics"
)

// New creates a new Temporal client with full observability support.
//
// Observability features (reference: https://docs.temporal.io/develop/go/observability):
//   - OpenTelemetry tracing for workflows, activities, and child workflows
//   - OpenTelemetry metrics for Temporal SDK (workflow tasks, activities, polls)
//   - Structured logging with go-sdk logger adapter
//   - gRPC-level metrics and tracing via go-sdk/grpc
//
// Configuration is read from environment variables:
//   - TEMPORAL_HOST: Temporal server address (default: temporal-frontend.temporal.svc.cluster.local:7233)
//   - TEMPORAL_NAMESPACE: Temporal namespace (default: default)
//   - TEMPORAL_IDENTITY: Worker identity (optional)
//
// gRPC options are configured via GRPC_CLIENT_* environment variables (from go-sdk/grpc):
//   - GRPC_CLIENT_TLS_ENABLED: Enable TLS (default: false)
//   - GRPC_CLIENT_TIMEOUT: Request timeout (default: 10s)
//
// Temporal-specific overrides:
//   - TEMPORAL_TLS_ENABLED: Override TLS for Temporal connection (optional, uses GRPC_CLIENT_TLS_ENABLED if not set)
//
// Reference: https://docs.temporal.io/develop/go/temporal-client
func New(
	l logger.Logger,
	cfg *config.Config,
	tracer trace.TracerProvider,
	monitor *metrics.Monitoring,
) (client.Client, error) {
	// Set defaults
	cfg.SetDefault("TEMPORAL_HOST", "temporal-frontend.temporal.svc.cluster.local:7233")
	cfg.SetDefault("TEMPORAL_NAMESPACE", "default")

	host := cfg.GetString("TEMPORAL_HOST")
	namespace := cfg.GetString("TEMPORAL_NAMESPACE")
	identity := cfg.GetString("TEMPORAL_IDENTITY")

	// Override TLS setting for Temporal if TEMPORAL_TLS_ENABLED is explicitly set
	if cfg.IsSet("TEMPORAL_TLS_ENABLED") {
		cfg.Set("GRPC_CLIENT_TLS_ENABLED", cfg.GetBool("TEMPORAL_TLS_ENABLED"))
	}

	// Build gRPC dial options using go-sdk/grpc
	grpcOpts := []sdkgrpc.Option{
		sdkgrpc.WithLogger(l),
		sdkgrpc.WithTimeout(),
	}

	// Add tracing and metrics if available
	if tracer != nil && monitor != nil {
		grpcOpts = append(grpcOpts, sdkgrpc.WithTracer(tracer, monitor.Prometheus, monitor.Metrics))
		grpcOpts = append(grpcOpts, sdkgrpc.WithMetrics(monitor.Prometheus))
	}

	// Add auth forwarding for Istio/Oathkeeper integration
	grpcOpts = append(grpcOpts, sdkgrpc.WithAuthForward())

	// Get configured gRPC client options
	grpcClient, err := sdkgrpc.SetClientConfig(cfg, grpcOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to configure gRPC client: %w", err)
	}

	// Build Temporal interceptors
	// Reference: https://docs.temporal.io/develop/go/observability#tracing-and-context-propagation
	interceptors := make([]interceptor.ClientInterceptor, 0)

	// OpenTelemetry tracing interceptor for Temporal workflows
	tracingInterceptor, err := opentelemetry.NewTracingInterceptor(opentelemetry.TracerOptions{
		Tracer: otel.Tracer("temporal-sdk-go"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tracing interceptor: %w", err)
	}
	interceptors = append(interceptors, tracingInterceptor)

	// Build client options
	opts := client.Options{
		HostPort:     host,
		Namespace:    namespace,
		Logger:       newLogAdapter(l),
		Interceptors: interceptors,
		ConnectionOptions: client.ConnectionOptions{
			DialOptions: grpcClient.GetOptions(),
		},
	}

	// Add OpenTelemetry metrics handler for Temporal SDK metrics
	// Reference: https://docs.temporal.io/develop/go/observability#how-to-emit-metrics
	if monitor != nil && monitor.Metrics != nil {
		meter := monitor.Metrics.Meter("temporal-sdk-go")
		opts.MetricsHandler = opentelemetry.NewMetricsHandler(opentelemetry.MetricsHandlerOptions{
			Meter: meter,
			OnError: func(err error) {
				l.Error("Temporal metrics error", slog.String("error", err.Error()))
			},
		})
	}

	if identity != "" {
		opts.Identity = identity
	}

	// Create client
	c, err := client.Dial(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}

	l.Info("Temporal client created",
		slog.String("host", host),
		slog.String("namespace", namespace),
	)

	return c, nil
}

// CheckHealth verifies the connection to Temporal server.
// Reference: https://docs.temporal.io/develop/go/temporal-client
func CheckHealth(ctx context.Context, c client.Client) error {
	_, err := c.WorkflowService().GetSystemInfo(ctx, nil)
	if err != nil {
		return fmt.Errorf("temporal health check failed: %w", err)
	}
	return nil
}

// logAdapter adapts go-sdk logger to Temporal's log.Logger interface.
// Reference: https://docs.temporal.io/develop/go/observability#log-from-a-workflow
type logAdapter struct {
	logger logger.Logger
}

func newLogAdapter(l logger.Logger) log.Logger {
	return &logAdapter{logger: l}
}

func (l *logAdapter) Debug(msg string, keyvals ...any) {
	l.logger.Debug(msg, toSlogAttrs(keyvals)...)
}

func (l *logAdapter) Info(msg string, keyvals ...any) {
	l.logger.Info(msg, toSlogAttrs(keyvals)...)
}

func (l *logAdapter) Warn(msg string, keyvals ...any) {
	l.logger.Warn(msg, toSlogAttrs(keyvals)...)
}

func (l *logAdapter) Error(msg string, keyvals ...any) {
	l.logger.Error(msg, toSlogAttrs(keyvals)...)
}

// toSlogAttrs converts key-value pairs to slog attributes.
func toSlogAttrs(keyvals []any) []slog.Attr {
	if len(keyvals) == 0 {
		return nil
	}

	attrs := make([]slog.Attr, 0, len(keyvals)/2)
	for i := 0; i < len(keyvals)-1; i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		attrs = append(attrs, slog.Any(key, keyvals[i+1]))
	}

	return attrs
}
