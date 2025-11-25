package handlers

import (
	"time"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	wmmid "github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/sony/gobreaker"
)

// DecoratorConfig controls CQRS handler middleware behavior.
type DecoratorConfig struct {
	Timeout                time.Duration
	RetryMax               int
	CircuitBreakerEnabled  bool
	CircuitBreakerSettings *gobreaker.Settings
}

// DecorateHandler wraps handler with standard CQRS middlewares:
// Recoverer -> CircuitBreaker -> Timeout -> Retry.
func DecorateHandler(h wmmessage.HandlerFunc, cfg DecoratorConfig) wmmessage.HandlerFunc {
	if h == nil {
		return nil
	}

	decorated := wmmid.Recoverer(h)

	if cfg.CircuitBreakerEnabled {
		settings := cfg.CircuitBreakerSettings
		if settings == nil {
			defaultCfg := defaultCircuitBreakerSettings()
			settings = &defaultCfg
		}
		cb := wmmid.NewCircuitBreaker(*settings)
		decorated = cb.Middleware(decorated)
	}

	if cfg.Timeout > 0 {
		decorated = wmmid.Timeout(cfg.Timeout)(decorated)
	}

	if cfg.RetryMax > 0 {
		retry := wmmid.Retry{
			MaxRetries: cfg.RetryMax,
		}
		decorated = retry.Middleware(decorated)
	}

	return decorated
}

func defaultCircuitBreakerSettings() gobreaker.Settings {
	return gobreaker.Settings{
		Name:        "shortlink_cqrs_handler",
		Timeout:     30 * time.Second,
		Interval:    0,
		MaxRequests: 1,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}
}
