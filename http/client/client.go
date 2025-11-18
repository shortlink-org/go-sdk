package http_client

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func New(opts ...Option) (*http.Client, error) {
	cfg := new(config)
	cfg.base = http.DefaultTransport

	// Apply options
	for _, opt := range opts {
		err := opt(cfg)
		if err != nil {
			return nil, err
		}
	}

	// Local Token Bucket (optional)
	var limiter Limiter

	if cfg.rate > 0 && cfg.burst > 0 {
		l, err := NewTokenBucketLimiter(cfg.rate, cfg.burst, cfg.jitter)
		if err != nil {
			return nil, err
		}

		limiter = l
	}

	// OTEL HTTP wrapping for propagation
	otelTransport := otelhttp.NewTransport(
		cfg.base,
		otelhttp.WithPropagators(nil), // uses global by default
	)

	// Build middleware chain
	chain := Chain(
		OTelWaitMiddleware(OTelWaitConfig{
			Client: cfg.clientName,
		}),
		DeadlineMiddleware(DeadlineConfig{
			Threshold: cfg.deadlineThreshold,
			Metrics:   cfg.metrics,
			Client:    cfg.clientName,
		}),
		ServerRateLimitMiddleware(ServerLimitConfig{
			JitterFraction: cfg.headerJitter,
			Metrics:        cfg.metrics,
			Client:         cfg.clientName,
		}),
		TokenBucketMiddleware(TokenBucketConfig{
			Limiter: limiter,
			Metrics: cfg.metrics,
			Client:  cfg.clientName,
		}),
		Metrics429Middleware(Metrics429Config{
			Metrics: cfg.metrics,
			Client:  cfg.clientName,
		}),
	)

	client := new(http.Client)
	client.Transport = chain(otelTransport)
	client.Timeout = 0 * time.Second // timeout should be per-request via ctx

	return client, nil
}
