package grpc

import (
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/timeout"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shortlink-org/go-sdk/grpc/authforward"
	grpc_logger "github.com/shortlink-org/go-sdk/grpc/middleware/logger"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	api "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// Option configures a gRPC client.
type Option func(*Client)

// Apply a batch of options.
func (c *Client) apply(options ...Option) {
	for _, option := range options {
		option(c)
	}
}

// WithTimeout sets a unary timeout interceptor.
func WithTimeout() Option {
	return func(client *Client) {
		client.cfg.SetDefault("GRPC_CLIENT_TIMEOUT", "10s") // Set timeout for gRPC-Client
		timeoutClient := client.cfg.GetDuration("GRPC_CLIENT_TIMEOUT")

		client.interceptorUnaryClientList = append(
			client.interceptorUnaryClientList,
			timeout.UnaryClientInterceptor(timeoutClient),
		)
	}
}

// WithLogger adds unary & stream logging interceptors.
func WithLogger(log logger.Logger) Option {
	return func(client *Client) {
		client.cfg.SetDefault("GRPC_CLIENT_LOGGER_ENABLED", true) // Enable logging for gRPC-Client

		isEnableLogger := client.cfg.GetBool("GRPC_CLIENT_LOGGER_ENABLED")
		if !isEnableLogger {
			return
		}

		client.interceptorUnaryClientList = append(
			client.interceptorUnaryClientList,
			grpc_logger.UnaryClientInterceptor(log),
		)
		client.interceptorStreamClientList = append(
			client.interceptorStreamClientList,
			grpc_logger.StreamClientInterceptor(log),
		)
	}
}

// WithTracer wires up otel handler.
func WithTracer(tracer trace.TracerProvider, prom *prometheus.Registry, metrics *api.MeterProvider) Option {
	return func(client *Client) {
		if tracer == nil || prom == nil {
			return
		}

		client.optionsNewClient = append(
			client.optionsNewClient,
			grpc.WithStatsHandler(
				otelgrpc.NewClientHandler(
					otelgrpc.WithTracerProvider(tracer),
					otelgrpc.WithMeterProvider(metrics),
					otelgrpc.WithMessageEvents(otelgrpc.ReceivedEvents, otelgrpc.SentEvents),
				),
			),
		)
	}
}

// WithMetrics registers Prom metrics + interceptors.
func WithMetrics(prom *prometheus.Registry) Option {
	return func(client *Client) {
		if prom == nil {
			return
		}

		clientMetrics := grpc_prometheus.NewClientMetrics(
			grpc_prometheus.WithClientHandlingTimeHistogram(
				grpc_prometheus.WithHistogramBuckets([]float64{
					0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120,
				}),
			),
		)
		exemplarFromCtx := grpc_prometheus.WithExemplarFromContext(exemplarFromContext)

		client.interceptorUnaryClientList = append(
			client.interceptorUnaryClientList,
			clientMetrics.UnaryClientInterceptor(exemplarFromCtx),
		)
		client.interceptorStreamClientList = append(
			client.interceptorStreamClientList,
			clientMetrics.StreamClientInterceptor(exemplarFromCtx),
		)

		defer func() {
			// ignore duplicate-registration panic
			_ = recover()
		}()

		prom.MustRegister(clientMetrics)
	}
}

// WithAuthForward adds auth token forwarding interceptors.
func WithAuthForward() Option {
	return func(client *Client) {
		client.interceptorUnaryClientList = append(
			client.interceptorUnaryClientList,
			authforward.UnaryClientInterceptor(),
		)
		client.interceptorStreamClientList = append(
			client.interceptorStreamClientList,
			authforward.StreamClientInterceptor(),
		)
	}
}
