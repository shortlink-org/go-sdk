/*
Tracing wrapping
*/
package tracing

import (
	"context"
	"log/slog"

	"github.com/grafana/otel-profiling-go"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	traceProvider "go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/logger"

	"github.com/shortlink-org/go-sdk/observability/common"
)

// New returns a new instance of the TracerProvider.
//
//nolint:ireturn // It's make by specification
func New(ctx context.Context, log logger.Logger) (traceProvider.TracerProvider, func(), error) {
	viper.SetDefault("TRACER_URI", "localhost:4317") // Tracing addr:host

	config := Config{
		ServiceName:    viper.GetString("SERVICE_NAME"),
		ServiceVersion: viper.GetString("SERVICE_VERSION"),
		URI:            viper.GetString("TRACER_URI"),
	}

	tracer, tracerClose, err := Init(ctx, config, log)
	if err != nil {
		return nil, nil, err
	}

	if tracer == nil {
		return nil, func() {}, nil
	}

	return tracer, tracerClose, nil
}

// Init returns an instance of Tracer Provider that samples 100% of traces and logs all spans to stdout.
func Init(ctx context.Context, cnf Config, log logger.Logger) (*trace.TracerProvider, func(), error) {
	// Setup resource.
	res, err := common.NewResource(ctx, cnf.ServiceName, cnf.ServiceVersion)
	if err != nil {
		return nil, nil, err
	}

	// Setup trace provider.
	tp, err := newTraceProvider(ctx, res, cnf.URI)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		errShutdown := tp.Shutdown(ctx)
		if errShutdown != nil {
			log.Error(`Tracing disable`,
				slog.String("uri", cnf.URI),
				slog.Any("err", errShutdown),
			)
		}
	}

	log.Info(`Tracing enable`,
		slog.String("uri", cnf.URI),
	)

	// Gracefully shutdown the trace provider on exit
	go func() {
		<-ctx.Done()

		// Shutdown will flush any remaining spans and shut down the exporter.
		if errShutdown := tp.Shutdown(ctx); errShutdown != nil {
			log.Error("error shutting down trace provider",
				slog.String("err", errShutdown.Error()),
			)
		}
	}()

	return tp, cleanup, nil
}

func newTraceProvider(ctx context.Context, res *resource.Resource, uri string) (*trace.TracerProvider, error) {
	viper.SetDefault("TRACING_INITIAL_INTERVAL", "2s")
	viper.SetDefault("TRACING_MAX_INTERVAL", "30s")
	viper.SetDefault("TRACING_MAX_ELAPSED_TIME", "1m")

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(uri),
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: viper.GetDuration("TRACING_INITIAL_INTERVAL"),
			MaxInterval:     viper.GetDuration("TRACING_MAX_INTERVAL"),
			MaxElapsedTime:  viper.GetDuration("TRACING_MAX_ELAPSED_TIME"),
		}),
	)
	if err != nil {
		return nil, err
	}

	traceProviderService := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter, trace.WithBatchTimeout(viper.GetDuration("TRACING_INITIAL_INTERVAL"))),
		trace.WithResource(res),
		trace.WithSampler(trace.ParentBased(trace.AlwaysSample())),
	)

	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(traceProviderService))

	// Register the W3C trace context and baggage propagators so data is propagated across services/processes
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return traceProviderService, nil
}
