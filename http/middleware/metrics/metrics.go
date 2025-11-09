package metrics_middleware

import (
	"net/http"
	"strconv"
	"strings"
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

func NewMetrics() (func(next http.Handler) http.Handler, error) {
	m := metrics{
		reqs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests made.",
		}, []string{"status", "method", "path"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "The HTTP request latencies in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"status", "method", "path"}),
	}

	err := prometheus.Register(m.reqs)
	if err != nil {
		return nil, err
	}

	err = prometheus.Register(m.latency)
	if err != nil {
		return nil, err
	}

	return m.middleware, nil
}

func (m metrics) middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		m.writeMetrics(r, start, strconv.Itoa(ww.Status()))
	}

	return http.HandlerFunc(fn)
}

func (m *metrics) writeMetrics(r *http.Request, start time.Time, code string) {
	rctx := chi.RouteContext(r.Context())
	routePattern := strings.Join(rctx.RoutePatterns, "")
	routePattern = strings.ReplaceAll(routePattern, "/*/", "/")

	m.reqs.WithLabelValues(code, r.Method, routePattern).Inc()
	observer := m.latency.WithLabelValues(code, r.Method, routePattern)
	latencySeconds := time.Since(start).Seconds()

	spanCtx := trace.SpanContextFromContext(r.Context())
	if spanCtx.HasTraceID() && spanCtx.IsSampled() {
		if exemplarObserver, ok := observer.(prometheus.ExemplarObserver); ok {
			exemplarObserver.ObserveWithExemplar(latencySeconds, prometheus.Labels{"trace_id": spanCtx.TraceID().String()})

			return
		}
	}

	observer.Observe(latencySeconds)
}
