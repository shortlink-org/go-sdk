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
	"go.opentelemetry.io/otel"
	promExporter "go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	http_server "github.com/shortlink-org/go-sdk/http/server"
	"github.com/shortlink-org/go-sdk/observability/common"
)

type Monitoring struct {
	Handler    *http.ServeMux
	Prometheus *prometheus.Registry
	Metrics    *api.MeterProvider
	cfg        *config.Config
}

// New - Monitoring endpoints
func New(ctx context.Context, log *slog.Logger, tracer trace.TracerProvider, cfg *config.Config) (*Monitoring, func(), error) {
	var err error

	monitoring := &Monitoring{cfg: cfg}

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
		serverConfig := http_server.Config{
			Port:    9090,             //nolint:mnd // port for Prometheus metrics
			Timeout: 30 * time.Second, //nolint:mnd // timeout for Prometheus metrics
		}
		server := http_server.New(ctx, monitoring.Handler, serverConfig, cfg)

		errListenAndServe := server.ListenAndServe()
		if errListenAndServe != nil {
			log.Error(errListenAndServe.Error())
		}
	}()

	log.Info("Run monitoring",
		slog.String("addr", "0.0.0.0:9090"),
	)

	return monitoring, func() {
		errShutdown := monitoring.Metrics.Shutdown(ctx)
		if errShutdown != nil {
			log.ErrorContext(ctx, errShutdown.Error())
		}
	}, nil
}

// SetMetrics - Create a "common" meter provider for metrics
func (m *Monitoring) SetMetrics(ctx context.Context) (*api.MeterProvider, error) {
	// See the go.opentelemetry.io/otel/sdk/resource package for more
	// information about how to create and use Resources.
	// Setup resource.
	res, err := common.NewResource(ctx, m.cfg.GetString("SERVICE_NAME"), m.cfg.GetString("SERVICE_VERSION"))
	if err != nil {
		return nil, err
	}

	// Create a new Prometheus registry
	err = m.SetPrometheus()
	if err != nil {
		return nil, err
	}

	// prometheus.DefaultRegisterer is used by default
	// so that metrics are available via promhttp.Handler.
	registry, err := promExporter.New(
		promExporter.WithRegisterer(m.Prometheus),
	)
	if err != nil {
		return nil, err
	}

	provider := api.NewMeterProvider(
		api.WithResource(res),
		api.WithReader(registry),
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

			// Overlapping scrapes share one collection cycle instead of each
			// starting its own, which keeps goroutines from piling up when the
			// scrape rate outpaces collection.
			CoalesceGather: true,

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
//
// Collectors go into this registry, not the global one: /metrics serves
// m.Prometheus, so anything registered globally would never be exposed.
func (m *Monitoring) SetPrometheus() error {
	m.Prometheus = prometheus.NewRegistry()

	collectorSet := []prometheus.Collector{
		// Go module build info.
		collectors.NewBuildInfoCollector(),

		// Go runtime metrics, including the scheduler ones added in Go 1.26
		// (goroutines created, runnable, and not-in-Go).
		collectors.NewGoCollector(
			collectors.WithGoCollectorRuntimeMetrics(
				collectors.MetricsGC,
				collectors.MetricsMemory,
				collectors.MetricsScheduler,
			),
		),

		// Process metrics: CPU, memory, file descriptors.
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
			PidFn:        nil,
			Namespace:    "",
			ReportErrors: false,
		}),
	}

	for _, collector := range collectorSet {
		err := m.Prometheus.Register(collector)
		if err != nil {
			return err
		}
	}

	return nil
}
