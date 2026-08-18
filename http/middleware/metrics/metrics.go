package metrics_middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

type metrics struct {
	reqs    *prometheus.CounterVec
	latency *prometheus.HistogramVec
}

// NewMetrics returns the middleware and registers its collectors with reg.
//
// The registry is explicit because the exposed one is rarely the global
// default: observability serves its own registry, and metrics registered
// globally never reach that /metrics. Pass prometheus.DefaultRegisterer to
// keep the previous behaviour.
func NewMetrics(reg prometheus.Registerer) (func(next http.Handler) http.Handler, error) {
	collector := &metrics{
		reqs: prometheus.NewCounterVec(prometheus.CounterOpts{ //nolint:exhaustruct // Prometheus options intentionally use defaults
			Name: "http_requests_total",
			Help: "Total number of HTTP requests made.",
		}, []string{"status", "method", "path"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{ //nolint:exhaustruct // Prometheus options intentionally use defaults
			Name: "http_request_duration_seconds",
			Help: "The HTTP request latencies in seconds.",
			// Declared for OpenMetrics, which exposes the unit separately
			// instead of leaving it implicit in the metric name.
			Unit:    "seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"status", "method", "path"}),
	}

	err := reg.Register(collector.reqs)
	if err != nil {
		return nil, err
	}

	err = reg.Register(collector.latency)
	if err != nil {
		return nil, err
	}

	return collector.middleware, nil
}

func (m *metrics) middleware(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrappedWriter := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrappedWriter, r)

		m.writeMetrics(r, start, strconv.Itoa(wrappedWriter.Status()))
	}

	return http.HandlerFunc(handlerFunc)
}

func (m *metrics) writeMetrics(req *http.Request, start time.Time, code string) {
	// RoutePattern collapses consecutive wildcards and trims the trailing
	// slash, so "/api/*/*/users/" and "/api/users" share one label value
	// instead of inflating the metric cardinality. It is nil-safe, which
	// keeps the middleware usable outside a chi router.
	routePattern := chi.RouteContext(req.Context()).RoutePattern()

	m.reqs.WithLabelValues(code, req.Method, routePattern).Inc()
	observer := m.latency.WithLabelValues(code, req.Method, routePattern)
	latencySeconds := time.Since(start).Seconds()

	spanCtx := trace.SpanContextFromContext(req.Context())
	if spanCtx.HasTraceID() && spanCtx.IsSampled() {
		if exemplarObserver, ok := observer.(prometheus.ExemplarObserver); ok {
			exemplarObserver.ObserveWithExemplar(latencySeconds, prometheus.Labels{"trace_id": spanCtx.TraceID().String()})

			return
		}
	}

	observer.Observe(latencySeconds)
}
