// Package tracing wraps the OpenTelemetry tracer provider.
package tracing

import (
	"context"
	"log/slog"
	"sync"

	otelpyroscope "github.com/grafana/otel-profiling-go"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	traceProvider "go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/observability/common"
)

// New returns a new instance of the TracerProvider.
//
// Tracing is optional and is switched on by TRACER_URI. When the variable is
// empty, New installs no global provider, leaves the propagators alone and
// returns a nil provider with a no-op cleanup. Off costs nothing: no exporter
// is built, so there are no retries against a collector and nothing reaches
// the otel error handler.
//
//nolint:ireturn // It's make by specification
func New(ctx context.Context, log *slog.Logger, cfg *config.Config) (traceProvider.TracerProvider, func(), error) {
	tracingConfig := Config{
		ServiceName:    cfg.GetString("SERVICE_NAME"),
		ServiceVersion: cfg.GetString("SERVICE_VERSION"),
		URI:            cfg.GetString("TRACER_URI"), // Tracing addr:host; empty means "do not trace"
	}

	tracer, tracerClose, err := Init(ctx, tracingConfig, log, cfg)
	if err != nil {
		return nil, nil, err
	}

	if tracer == nil {
		return nil, func() {}, nil
	}

	return tracer, tracerClose, nil
}

// Init returns an instance of Tracer Provider that samples 100% of traces and exports
// them over OTLP/gRPC to cnf.URI. An empty cnf.URI turns tracing off: Init returns
// a nil provider, a no-op cleanup and no error.
//
// The returned cleanup flushes pending spans and shuts the exporter down. It is
// bounded by TRACING_SHUTDOWN_TIMEOUT and is safe to call more than once.
func Init(ctx context.Context, cnf Config, log *slog.Logger, cfg *config.Config) (*trace.TracerProvider, func(), error) {
	if cnf.URI == "" {
		return nil, func() {}, nil
	}

	// Setup resource.
	res, err := common.NewResource(ctx, cnf.ServiceName, cnf.ServiceVersion)
	if err != nil {
		return nil, nil, err
	}

	// Setup trace provider.
	provider, err := newTraceProvider(ctx, res, cnf.URI, cfg)
	if err != nil {
		return nil, nil, err
	}

	// The shutdown budget is deliberately separate from TRACING_MAX_ELAPSED_TIME.
	// That one bounds how long a single export may keep retrying while the service
	// is running and can reasonably be a minute. A service told to stop should stop:
	// it flushes what it can within this timeout and drops the rest rather than
	// hanging on an unreachable collector.
	cfg.SetDefault("TRACING_SHUTDOWN_TIMEOUT", "10s")
	shutdownTimeout := cfg.GetDuration("TRACING_SHUTDOWN_TIMEOUT")

	var once sync.Once

	cleanup := func() { //nolint:contextcheck // the startup ctx is usually canceled by now; shutdown needs its own bounded one
		once.Do(func() {
			// The ctx Init was started with is usually already canceled by the time
			// the service stops, and Shutdown with a canceled ctx aborts instead of
			// flushing. Use a fresh, bounded one.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			errShutdown := provider.Shutdown(shutdownCtx)
			if errShutdown != nil {
				log.Error(`Tracing disable`,
					slog.String("uri", cnf.URI),
					slog.Any("err", errShutdown),
				)
			}
		})
	}

	log.Info(`Tracing enable`,
		slog.String("uri", cnf.URI),
	)

	return provider, cleanup, nil
}

func newTraceProvider(ctx context.Context, res *resource.Resource, uri string, cfg *config.Config) (*trace.TracerProvider, error) {
	cfg.SetDefault("TRACING_INITIAL_INTERVAL", "2s")
	cfg.SetDefault("TRACING_MAX_INTERVAL", "30s")
	cfg.SetDefault("TRACING_MAX_ELAPSED_TIME", "1m")

	initialInterval := cfg.GetDuration("TRACING_INITIAL_INTERVAL")
	maxInterval := cfg.GetDuration("TRACING_MAX_INTERVAL")
	maxElapsedTime := cfg.GetDuration("TRACING_MAX_ELAPSED_TIME")

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(uri),
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: initialInterval,
			MaxInterval:     maxInterval,
			MaxElapsedTime:  maxElapsedTime,
		}),
	)
	if err != nil {
		return nil, err
	}

	traceProviderService := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter, trace.WithBatchTimeout(initialInterval)),
		trace.WithResource(res),
		trace.WithSampler(trace.ParentBased(trace.AlwaysSample())),
	)

	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(traceProviderService))

	// Register propagators for trace context propagation across services
	// - TraceContext: W3C standard (traceparent header)
	// - Baggage: W3C baggage propagation
	// - B3: Zipkin/Istio/Envoy compatibility (b3, x-b3-* headers)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
			b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader|b3.B3SingleHeader)),
		),
	)

	return traceProviderService, nil
}
