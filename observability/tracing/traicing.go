/*
Tracing wrapping
*/
package tracing

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	traceProvider "go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/logger"

	"github.com/shortlink-org/go-sdk/observability/common"
)

// New returns a new instance of the TracerProvider.
//
//nolint:ireturn // It's make by specification
func New(ctx context.Context, log logger.Logger) (traceProvider.TracerProvider, func(), error) {
	viper.SetDefault("TRACER_URI", "localhost:4317")                     // Tracing addr:host
	viper.SetDefault("PYROSCOPE_URI", "http://pyroscope.pyroscope:4040") // Pyroscope addr:host
	viper.SetDefault("TRACER_INSECURE", true)                            // Default to insecure gRPC transport
	viper.SetDefault("OTEL_TRACES_SHUTDOWN_TIMEOUT", "5s")

	config := resolveConfig()

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
func Init(ctx context.Context, cnf Config, log logger.Logger) (*sdktrace.TracerProvider, func(), error) {
	// Setup resource.
	res, err := common.NewResource(ctx, cnf.ServiceName, cnf.ServiceVersion)
	if err != nil {
		return nil, nil, err
	}

	// Setup trace provider.
	tp, err := newTraceProvider(ctx, res, cnf)
	if err != nil {
		return nil, nil, err
	}

	var shutdownOnce sync.Once
	cleanup := func() {
		shutdownOnce.Do(func() {
			shutdownTimeout := viper.GetDuration("OTEL_TRACES_SHUTDOWN_TIMEOUT")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			if errShutdown := tp.Shutdown(shutdownCtx); errShutdown != nil {
				log.Error("Tracing shutdown",
					slog.String("uri", logEndpointForAttr(cnf.URI)),
					slog.Any("err", errShutdown),
				)
			}
		})
	}

	log.Info("Tracing enable",
		slog.String("uri", logEndpointForAttr(cnf.URI)),
	)

	// Gracefully shutdown the trace provider on exit
	go func() {
		<-ctx.Done()

		cleanup()
	}()

	return tp, cleanup, nil
}

func newTraceProvider(ctx context.Context, res *resource.Resource, cnf Config) (*sdktrace.TracerProvider, error) {
	viper.SetDefault("TRACING_INITIAL_INTERVAL", "5s")
	viper.SetDefault("TRACING_MAX_INTERVAL", "30s")
	viper.SetDefault("TRACING_MAX_ELAPSED_TIME", "1m")

	exporterOptions := []otlptracegrpc.Option{
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: viper.GetDuration("TRACING_INITIAL_INTERVAL"),
			MaxInterval:     viper.GetDuration("TRACING_MAX_INTERVAL"),
			MaxElapsedTime:  viper.GetDuration("TRACING_MAX_ELAPSED_TIME"),
		}),
	}

	if cnf.URI != "" {
		exporterOptions = append(exporterOptions, otlptracegrpc.WithEndpoint(cnf.URI))
	}

	if cnf.Insecure {
		exporterOptions = append(exporterOptions, otlptracegrpc.WithInsecure())
	}

	traceExporter, err := otlptracegrpc.New(ctx, exporterOptions...)
	if err != nil {
		return nil, err
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter, sdktrace.WithBatchTimeout(viper.GetDuration("TRACING_INITIAL_INTERVAL"))),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(samplerFromEnv()),
	)

	otel.SetTracerProvider(traceProvider)

	// Register the W3C trace context and baggage propagators so data is propagated across services/processes
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return traceProvider, nil
}

func resolveConfig() Config {
	serviceName := firstNonEmpty(
		viper.GetString("OTEL_SERVICE_NAME"),
		viper.GetString("SERVICE_NAME"),
	)
	if serviceName == "" {
		serviceName = "unknown_service"
	}

	serviceVersion := firstNonEmpty(
		viper.GetString("OTEL_SERVICE_VERSION"),
		viper.GetString("SERVICE_VERSION"),
	)

	endpoint := firstNonEmpty(
		viper.GetString("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
		viper.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
		viper.GetString("TRACER_URI"),
	)

	return Config{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		URI:            endpoint,
		PyroscopeURI:   viper.GetString("PYROSCOPE_URI"),
		Insecure: resolveBoolWithDefault([]string{
			"OTEL_EXPORTER_OTLP_TRACES_INSECURE",
			"OTEL_EXPORTER_OTLP_INSECURE",
			"TRACER_INSECURE",
		}, true),
	}
}

func resolveBoolWithDefault(keys []string, fallback bool) bool {
	for _, key := range keys {
		if viper.IsSet(key) {
			return viper.GetBool(key)
		}
	}

	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			return s
		}
	}

	return ""
}

func samplerFromEnv() sdktrace.Sampler {
	value := strings.ToLower(strings.TrimSpace(viper.GetString("OTEL_TRACES_SAMPLER")))
	value = strings.ReplaceAll(value, "-", "_")
	arg := strings.TrimSpace(viper.GetString("OTEL_TRACES_SAMPLER_ARG"))

	parseRatio := func() float64 {
		if arg == "" {
			return 1
		}

		ratio, err := strconv.ParseFloat(arg, 64)
		if err != nil || ratio < 0 || ratio > 1 {
			return 1
		}

		return ratio
	}

	switch value {
	case "always_off":
		return sdktrace.NeverSample()
	case "always_on":
		return sdktrace.AlwaysSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(parseRatio())
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseRatio()))
	case "parentbased_always_on", "":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func logEndpointForAttr(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}

	return firstNonEmpty(
		viper.GetString("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
		viper.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
		"auto",
	)
}
