package http_client

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

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
	err := reg.Register(m.RateLimitWaitSeconds)
	if err != nil {
		return fmt.Errorf("register wait_seconds: %w", err)
	}

	err = reg.Register(m.RateLimit429Total)
	if err != nil {
		return fmt.Errorf("register 429: %w", err)
	}

	err = reg.Register(m.DeadlineCancelledTotal)
	if err != nil {
		return fmt.Errorf("register deadline_canceled: %w", err)
	}

	return nil
}
