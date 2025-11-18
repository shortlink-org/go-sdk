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

type Option func(*config) error

func WithClientName(name string) Option {
	return func(c *config) error {
		c.clientName = name
		return nil
	}
}

func WithRateLimit(rate float64, burst int) Option {
	return func(c *config) error {
		c.rate = rate
		c.burst = burst

		return nil
	}
}

func WithJitter(fraction float64) Option {
	return func(c *config) error {
		c.jitter = fraction
		return nil
	}
}

func WithServerHeaderJitter(fraction float64) Option {
	return func(c *config) error {
		c.headerJitter = fraction
		return nil
	}
}

func WithDeadlineThreshold(t time.Duration) Option {
	return func(c *config) error {
		c.deadlineThreshold = t
		return nil
	}
}

func WithMetrics(m *Metrics) Option {
	return func(c *config) error {
		c.metrics = m
		return nil
	}
}

func WithBaseTransport(rt http.RoundTripper) Option {
	return func(c *config) error {
		c.base = rt
		return nil
	}
}
