package grpc

import (
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/timeout"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	api "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	grpc_logger "github.com/shortlink-org/go-sdk/grpc/middleware/logger"
	session_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/session"
	"github.com/shortlink-org/go-sdk/logger"
)

type Option func(*Client)

// Apply a batch of options
func (c *Client) apply(options ...Option) {
	for _, option := range options {
		option(c)
	}
}

// WithTimeout sets a unary timeout interceptor
func WithTimeout() Option {
	return func(c *Client) {
		c.cfg.SetDefault("GRPC_CLIENT_TIMEOUT", "10s") // Set timeout for gRPC-Client
		timeoutClient := c.cfg.GetDuration("GRPC_CLIENT_TIMEOUT")

		c.interceptorUnaryClientList = append(
			c.interceptorUnaryClientList,
			timeout.UnaryClientInterceptor(timeoutClient),
		)
	}
}

// WithLogger adds unary & stream logging interceptors
func WithLogger(log logger.Logger) Option {
	return func(c *Client) {
		c.cfg.SetDefault("GRPC_CLIENT_LOGGER_ENABLED", true) // Enable logging for gRPC-Client
		isEnableLogger := c.cfg.GetBool("GRPC_CLIENT_LOGGER_ENABLED")
		if !isEnableLogger {
			return
		}

		c.interceptorUnaryClientList = append(c.interceptorUnaryClientList, grpc_logger.UnaryClientInterceptor(log))
		c.interceptorStreamClientList = append(c.interceptorStreamClientList, grpc_logger.StreamClientInterceptor(log))
	}
}

// WithTracer wires up otel handler
func WithTracer(tracer trace.TracerProvider, prom *prometheus.Registry, metrics *api.MeterProvider) Option {
	return func(c *Client) {
		if tracer == nil || prom == nil {
			return
		}

		c.optionsNewClient = append(c.optionsNewClient, grpc.WithStatsHandler(
			otelgrpc.NewClientHandler(
				otelgrpc.WithTracerProvider(tracer),
				otelgrpc.WithMeterProvider(metrics),
				otelgrpc.WithMessageEvents(otelgrpc.ReceivedEvents, otelgrpc.SentEvents),
			),
		))
	}
}

// WithMetrics registers Prom metrics + interceptors
func WithMetrics(prom *prometheus.Registry) Option {
	return func(c *Client) {
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

		c.interceptorUnaryClientList = append(c.interceptorUnaryClientList, clientMetrics.UnaryClientInterceptor(exemplarFromCtx))
		c.interceptorStreamClientList = append(c.interceptorStreamClientList, clientMetrics.StreamClientInterceptor(exemplarFromCtx))

		defer func() {
			// ignore duplicate-registration panic
			_ = recover()
		}()
		prom.MustRegister(clientMetrics)
	}
}

// WithSession adds session interceptors with optional ignore rules
func WithSession() Option {
	return func(c *Client) {
		c.interceptorUnaryClientList = append(c.interceptorUnaryClientList, session_interceptor.SessionUnaryClientInterceptor())
		c.interceptorStreamClientList = append(c.interceptorStreamClientList, session_interceptor.SessionStreamClientInterceptor())
	}
}
