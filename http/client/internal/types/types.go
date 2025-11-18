package types

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Middleware wraps a RoundTripper to add functionality.
// It receives the next RoundTripper in the chain and returns a new RoundTripper
// that may intercept, modify, or observe requests and responses.
type Middleware func(next http.RoundTripper) http.RoundTripper

// RoundTripperFunc allows using a function as a RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Limiter defines the interface for rate limiting.
// Wait blocks until a request can be made according to the rate limit,
// or until the context is cancelled. It returns the total time spent waiting
// and any error that occurred (typically context.Canceled or context.DeadlineExceeded).
type Limiter interface {
	Wait(ctx context.Context) (time.Duration, error)
}

const (
	LabelClient = "client"
	LabelHost   = "host"
	LabelMethod = "method"
	LabelSource = "source"
)

type Metrics struct {
	RateLimitWaitSeconds   *prometheus.HistogramVec
	RateLimit429Total      *prometheus.CounterVec
	DeadlineCancelledTotal *prometheus.CounterVec
}

func NewMetrics(namespace, subsystem string) *Metrics {
	return &Metrics{
		RateLimitWaitSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{ //nolint:exhaustruct // Prometheus options have many optional fields
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "rate_limit_wait_seconds",
				Help:      "Time spent waiting due to rate limiting (server headers or local bucket).",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{LabelClient, LabelHost, LabelMethod, LabelSource},
		),
		RateLimit429Total: prometheus.NewCounterVec(
			prometheus.CounterOpts{ //nolint:exhaustruct // Prometheus options have many optional fields
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "rate_limit_429_total",
				Help:      "Total number of HTTP 429 responses.",
			},
			[]string{LabelClient, LabelHost, LabelMethod},
		),
		DeadlineCancelledTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{ //nolint:exhaustruct // Prometheus options have many optional fields
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "deadline_canceled_total",
				Help:      "Requests canceled early due to short deadline.",
			},
			[]string{LabelClient, LabelHost, LabelMethod},
		),
	}
}

func (m *Metrics) Register(reg prometheus.Registerer) error {
	if err := reg.Register(m.RateLimitWaitSeconds); err != nil {
		return fmt.Errorf("register wait_seconds: %w", err)
	}

	if err := reg.Register(m.RateLimit429Total); err != nil {
		return fmt.Errorf("register 429: %w", err)
	}

	if err := reg.Register(m.DeadlineCancelledTotal); err != nil {
		return fmt.Errorf("register deadline_canceled: %w", err)
	}

	return nil
}
