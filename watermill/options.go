package watermill

import (
	"strings"
	"time"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/sony/gobreaker"
)

// Option configures Watermill client behavior.
type Option func(*Options)

// Options describe middleware configuration that can be tweaked via functional options.
type Options struct {
	Retry          RetryOptions
	Timeout        TimeoutOptions
	CircuitBreaker CircuitBreakerOptions
}

// RetryOptions configure retry middleware behavior.
type RetryOptions struct {
	Enabled             bool
	MaxRetries          int
	InitialInterval     time.Duration
	MaxInterval         time.Duration
	Multiplier          float64
	Jitter              float64
	MaxElapsedTime      time.Duration
	ResetContextOnRetry bool
}

// TimeoutOptions configure handler timeout middleware.
type TimeoutOptions struct {
	Enabled  bool
	Duration time.Duration
}

// CircuitBreakerOptions configure the circuit breaker middleware.
type CircuitBreakerOptions struct {
	Enabled  bool
	Settings gobreaker.Settings
}

func defaultOptions(cfg *config.Config) Options {
	cfg.SetDefault("WATERMILL_RETRY_MAX_RETRIES", 3)
	cfg.SetDefault("WATERMILL_RETRY_INITIAL_INTERVAL", "150ms")
	cfg.SetDefault("WATERMILL_RETRY_MAX_INTERVAL", "2s")
	cfg.SetDefault("WATERMILL_RETRY_MULTIPLIER", 2.0)
	cfg.SetDefault("WATERMILL_RETRY_JITTER", 0.15)
	cfg.SetDefault("WATERMILL_RETRY_MAX_ELAPSED", "0s")
	cfg.SetDefault("WATERMILL_RETRY_RESET_CONTEXT", false)

	cfg.SetDefault("WATERMILL_HANDLER_TIMEOUT_ENABLED", true)
	cfg.SetDefault("WATERMILL_HANDLER_TIMEOUT", "20s")

	cfg.SetDefault("WATERMILL_CB_ENABLED", true)
	cfg.SetDefault("WATERMILL_CB_TIMEOUT", "30s")
	cfg.SetDefault("WATERMILL_CB_INTERVAL", "0s")
	cfg.SetDefault("WATERMILL_CB_FAILURE_THRESHOLD", 5)
	cfg.SetDefault("WATERMILL_CB_HALFOPEN_MAX_REQUESTS", 1)

	retry := RetryOptions{
		Enabled:             true,
		MaxRetries:          cfg.GetInt("WATERMILL_RETRY_MAX_RETRIES"),
		InitialInterval:     cfg.GetDuration("WATERMILL_RETRY_INITIAL_INTERVAL"),
		MaxInterval:         cfg.GetDuration("WATERMILL_RETRY_MAX_INTERVAL"),
		Multiplier:          cfg.GetFloat64("WATERMILL_RETRY_MULTIPLIER"),
		Jitter:              cfg.GetFloat64("WATERMILL_RETRY_JITTER"),
		MaxElapsedTime:      cfg.GetDuration("WATERMILL_RETRY_MAX_ELAPSED"),
		ResetContextOnRetry: cfg.GetBool("WATERMILL_RETRY_RESET_CONTEXT"),
	}
	if retry.MaxRetries < 0 {
		retry.MaxRetries = 0
	}

	timeout := TimeoutOptions{
		Enabled:  cfg.GetBool("WATERMILL_HANDLER_TIMEOUT_ENABLED"),
		Duration: cfg.GetDuration("WATERMILL_HANDLER_TIMEOUT"),
	}
	if timeout.Duration <= 0 {
		timeout.Duration = 20 * time.Second
	}

	failureThreshold := cfg.GetInt("WATERMILL_CB_FAILURE_THRESHOLD")
	if failureThreshold <= 0 {
		failureThreshold = 5
	}

	serviceName := strings.TrimSpace(cfg.GetString("SERVICE_NAME"))
	cbName := "watermill_handler"
	if serviceName != "" {
		cbName = serviceName + "_watermill_handler"
	}

	cbSettings := gobreaker.Settings{
		Name:        cbName,
		Timeout:     cfg.GetDuration("WATERMILL_CB_TIMEOUT"),
		Interval:    cfg.GetDuration("WATERMILL_CB_INTERVAL"),
		MaxRequests: uint32(cfg.GetInt("WATERMILL_CB_HALFOPEN_MAX_REQUESTS")),
	}
	if cbSettings.Timeout <= 0 {
		cbSettings.Timeout = 30 * time.Second
	}
	if cbSettings.MaxRequests == 0 {
		cbSettings.MaxRequests = 1
	}
	cbSettings.ReadyToTrip = func(counts gobreaker.Counts) bool {
		return counts.ConsecutiveFailures >= uint32(failureThreshold)
	}

	cb := CircuitBreakerOptions{
		Enabled:  cfg.GetBool("WATERMILL_CB_ENABLED"),
		Settings: cbSettings,
	}

	return Options{
		Retry:          retry,
		Timeout:        timeout,
		CircuitBreaker: cb,
	}
}

// WithRetryOptions overrides retry middleware configuration.
func WithRetryOptions(opts RetryOptions) Option {
	return func(o *Options) {
		o.Retry = opts
	}
}

// WithTimeout enables timeout middleware with the provided duration.
func WithTimeout(duration time.Duration) Option {
	return func(o *Options) {
		o.Timeout.Enabled = duration > 0
		o.Timeout.Duration = duration
	}
}

// WithTimeoutOptions overrides timeout middleware configuration.
func WithTimeoutOptions(opts TimeoutOptions) Option {
	return func(o *Options) {
		o.Timeout = opts
	}
}

// WithCircuitBreakerOptions overrides circuit breaker settings.
func WithCircuitBreakerOptions(opts CircuitBreakerOptions) Option {
	return func(o *Options) {
		o.CircuitBreaker = opts
	}
}

// DisableRetry disables retry middleware entirely.
func DisableRetry() Option {
	return func(o *Options) {
		o.Retry.Enabled = false
	}
}

// DisableTimeout disables the timeout middleware.
func DisableTimeout() Option {
	return func(o *Options) {
		o.Timeout.Enabled = false
	}
}

// DisableCircuitBreaker disables the circuit breaker middleware.
func DisableCircuitBreaker() Option {
	return func(o *Options) {
		o.CircuitBreaker.Enabled = false
	}
}
