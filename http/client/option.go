package http_client

import (
	"net/http"
	"time"
)

type config struct {
	clientName        string
	rate              float64
	burst             int
	jitter            float64
	headerJitter      float64
	deadlineThreshold time.Duration
	metrics           *Metrics
	base              http.RoundTripper
}

// Option configures an HTTP client during construction.
type Option func(*config) error

// WithClientName sets the client name used for metrics and tracing.
func WithClientName(name string) Option {
	return func(c *config) error {
		c.clientName = name
		return nil
	}
}

// WithRateLimit configures the local token bucket rate limiter.
// Rate is in requests per second, burst is the maximum number of tokens.
// Both must be positive for the limiter to be created.
func WithRateLimit(rate float64, burst int) Option {
	return func(c *config) error {
		c.rate = rate
		c.burst = burst

		return nil
	}
}

// WithJitter sets the jitter fraction for the token bucket limiter.
// Fraction should be between 0 and 1. Negative values are normalized to 0,
// values greater than 1 are normalized to 1.
func WithJitter(fraction float64) Option {
	return func(c *config) error {
		c.jitter = fraction
		return nil
	}
}

// WithServerHeaderJitter sets the jitter fraction for server rate limit waits.
// Fraction should be between 0 and 1. Negative values are normalized to 0,
// values greater than 1 are normalized to 1.
func WithServerHeaderJitter(fraction float64) Option {
	return func(c *config) error {
		c.headerJitter = fraction
		return nil
	}
}

// WithDeadlineThreshold sets the minimum time before a context deadline
// at which requests will be rejected early. If a request's deadline is
// closer than this threshold, it will be cancelled immediately.
func WithDeadlineThreshold(t time.Duration) Option {
	return func(c *config) error {
		c.deadlineThreshold = t
		return nil
	}
}

// WithMetrics sets the Prometheus metrics collector.
func WithMetrics(m *Metrics) Option {
	return func(c *config) error {
		c.metrics = m
		return nil
	}
}

// WithBaseTransport sets the base HTTP transport to wrap with middleware.
// If nil, http.DefaultTransport is used.
func WithBaseTransport(rt http.RoundTripper) Option {
	return func(c *config) error {
		if rt == nil {
			c.base = http.DefaultTransport
		} else {
			c.base = rt
		}

		return nil
	}
}
