package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	promExporter "go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/http/server"
	"github.com/shortlink-org/go-sdk/logger"

	"github.com/shortlink-org/go-sdk/observability/common"
)

type Monitoring struct {
	Handler    *http.ServeMux
	Prometheus *prometheus.Registry
	Metrics    *api.MeterProvider
	exporter   *otlpmetricgrpc.Exporter
}

// New - Monitoring endpoints
func New(ctx context.Context, log logger.Logger, tracer trace.TracerProvider) (*Monitoring, func(), error) {
	var err error
	monitoring := &Monitoring{}

	// Create a "common" meter provider for metrics
	monitoring.Metrics, err = monitoring.SetMetrics(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Create a "common" listener
	monitoring.Handler, err = monitoring.SetHandler()
	if err != nil {
		return nil, nil, err
	}

	go func() {
		// Create a new HTTP server for Prometheus metrics
		config := http_server.Config{
			Port:    9090,             //nolint:mnd // port for Prometheus metrics
			Timeout: 30 * time.Second, //nolint:mnd // timeout for Prometheus metrics
		}
		server := http_server.New(ctx, monitoring.Handler, config, tracer)

		errListenAndServe := server.ListenAndServe()
		if errListenAndServe != nil {
			log.Error(errListenAndServe.Error())
		}
	}()
	log.Info("Run monitoring",
		slog.String("addr", "0.0.0.0:9090"),
	)

	return monitoring, func() {
		viper.SetDefault("OTEL_METRIC_SHUTDOWN_TIMEOUT", "10s")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), viper.GetDuration("OTEL_METRIC_SHUTDOWN_TIMEOUT"))
		defer cancel()

		if monitoring.Metrics != nil {
			if errShutdown := monitoring.Metrics.Shutdown(shutdownCtx); errShutdown != nil {
				log.ErrorWithContext(shutdownCtx, errShutdown.Error())
			}
		}

		if monitoring.exporter != nil {
			exporterCtx, exporterCancel := context.WithTimeout(context.Background(), viper.GetDuration("OTEL_METRIC_SHUTDOWN_TIMEOUT"))
			defer exporterCancel()

			if errShutdown := monitoring.exporter.Shutdown(exporterCtx); errShutdown != nil {
				log.ErrorWithContext(exporterCtx, errShutdown.Error())
			}
		}
	}, nil
}

// SetMetrics - Create a "common" meter provider for metrics
func (m *Monitoring) SetMetrics(ctx context.Context) (*api.MeterProvider, error) {
	viper.SetDefault("OTEL_METRIC_EXPORT_INTERVAL", "60s")
	viper.SetDefault("OTEL_METRIC_EXPORT_TIMEOUT", "30s")

	// See the go.opentelemetry.io/otel/sdk/resource package for more
	// information about how to create and use Resources.
	// Setup resource.
	res, err := common.NewResource(ctx, viper.GetString("SERVICE_NAME"), viper.GetString("SERVICE_VERSION"))
	if err != nil {
		return nil, err
	}

	// Create a new Prometheus registry
	err = m.SetPrometheus()
	if err != nil {
		return nil, err
	}

	// Create a new OTLP exporter for sending metrics to the OpenTelemetry Collector.
	m.exporter, err = otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	periodicReader := api.NewPeriodicReader(
		m.exporter,
		api.WithInterval(viper.GetDuration("OTEL_METRIC_EXPORT_INTERVAL")),
		api.WithTimeout(viper.GetDuration("OTEL_METRIC_EXPORT_TIMEOUT")),
	)

	// prometheus.DefaultRegisterer is used by default
	// so that metrics are available via promhttp.Handler.
	prometheusReader, err := promExporter.New(
		promExporter.WithRegisterer(m.Prometheus),
	)
	if err != nil {
		return nil, err
	}

	provider := api.NewMeterProvider(
		api.WithResource(res),
		api.WithReader(prometheusReader),
		api.WithReader(periodicReader),
		api.WithExemplarFilter(exemplar.TraceBasedFilter),
	)

	otel.SetMeterProvider(provider)

	return provider, nil
}

// SetHandler - Create a "common" handler for metrics
func (m *Monitoring) SetHandler() (*http.ServeMux, error) {
	// Create a "common" listener
	handler := http.NewServeMux()

	// Expose prometheus metrics on /metrics
	handler.Handle("/metrics", promhttp.HandlerFor(
		m.Prometheus,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,

			ErrorHandling: promhttp.ContinueOnError,
		},
	))

	// Create a metrics-exposing Handler for the Prometheus registry
	// The health check related metrics will be prefixed with the provided namespace
	health := healthcheck.NewMetricsHandler(m.Prometheus, "common")

	// Expose a liveness check on /live
	handler.HandleFunc("/live", health.LiveEndpoint)

	// Expose a readiness check on /ready
	handler.HandleFunc("/ready", health.ReadyEndpoint)

	return handler, nil
}

// SetPrometheus - Create a new Prometheus registry
func (m *Monitoring) SetPrometheus() error {
	m.Prometheus = prometheus.NewRegistry()

	// Add Go module build info.
	err := prometheus.Register(collectors.NewBuildInfoCollector())
	if err != nil {
		return err
	}

	return nil
}
