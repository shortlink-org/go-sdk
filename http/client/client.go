package http_client

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
	"github.com/shortlink-org/go-sdk/http/client/middleware/deadline"
	"github.com/shortlink-org/go-sdk/http/client/middleware/metrics429"
	"github.com/shortlink-org/go-sdk/http/client/middleware/otelwait"
	"github.com/shortlink-org/go-sdk/http/client/middleware/serverlimit"
	"github.com/shortlink-org/go-sdk/http/client/middleware/tokenbucket"
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
	var limiter types.Limiter

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
		otelwait.Middleware(otelwait.Config{
			Client: cfg.clientName,
		}),
		deadline.Middleware(deadline.Config{
			Threshold: cfg.deadlineThreshold,
			Metrics:   cfg.metrics,
			Client:    cfg.clientName,
		}),
		serverlimit.Middleware(serverlimit.Config{
			JitterFraction: cfg.headerJitter,
			Metrics:        cfg.metrics,
			Client:         cfg.clientName,
		}),
		tokenbucket.Middleware(tokenbucket.Config{
			Limiter: limiter,
			Metrics: cfg.metrics,
			Client:  cfg.clientName,
		}),
		metrics429.Middleware(metrics429.Config{
			Metrics: cfg.metrics,
			Client:  cfg.clientName,
		}),
	)

	client := new(http.Client)
	client.Transport = chain(otelTransport)
	client.Timeout = 0 * time.Second // timeout should be per-request via ctx

	return client, nil
}
