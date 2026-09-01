package watermill

import (
	"strings"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/sony/gobreaker"

	"github.com/shortlink-org/go-sdk/config"
)

// Defaults for the middleware chain.
const (
	defaultRetryMultiplier   = 2.0
	defaultRetryJitter       = 0.15
	defaultHandlerTimeoutSec = 20
	defaultBreakerTimeoutSec = 30
)

// Option configures Watermill client behavior.
type Option func(*Options)

// Options describe middleware configuration that can be tweaked via functional options.
type Options struct {
	Retry          RetryOptions
	Timeout        TimeoutOptions
	CircuitBreaker CircuitBreakerOptions
	Poison         PoisonOptions
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

// PoisonOptions configure the dead-letter (poison queue) middleware.
//
// Where the middleware sits is not a caller decision: the poison queue
// publishes the dead letter and then reports success, so under the retry
// middleware it would swallow every failure before retry could see one and
// nothing would ever be retried. Configuring it here is what lets the SDK put
// it outside retry, where a message reaches the DLQ only once its retries are
// spent.
type PoisonOptions struct {
	// Enabled turns the dead-letter middleware on.
	Enabled bool

	// Publisher receives the dead letters. Nil means the client's own
	// publisher — the instrumented one New returns.
	Publisher message.Publisher

	// Topic is the DLQ topic. Empty routes each message to
	// "<received_topic>.DLQ".
	Topic string

	// Filter reports whether a failure deserves the DLQ. Nil dead-letters
	// every failure that outlives retry. See
	// NewShortlinkPoisonMiddlewareWithFilter for what belongs here.
	Filter func(err error) bool

	// ServiceName is stamped on every DLQ event. Empty falls back to the
	// SERVICE_NAME environment variable, read at publication time.
	ServiceName string
}

func defaultOptions(cfg *config.Config) Options {
	cfg.SetDefault("WATERMILL_RETRY_MAX_RETRIES", 3)
	cfg.SetDefault("WATERMILL_RETRY_INITIAL_INTERVAL", "150ms")
	cfg.SetDefault("WATERMILL_RETRY_MAX_INTERVAL", "2s")
	cfg.SetDefault("WATERMILL_RETRY_MULTIPLIER", defaultRetryMultiplier)
	cfg.SetDefault("WATERMILL_RETRY_JITTER", defaultRetryJitter)
	cfg.SetDefault("WATERMILL_RETRY_MAX_ELAPSED", "0s")
	cfg.SetDefault("WATERMILL_RETRY_RESET_CONTEXT", false)

	cfg.SetDefault("WATERMILL_HANDLER_TIMEOUT_ENABLED", true)
	cfg.SetDefault("WATERMILL_HANDLER_TIMEOUT", "20s")

	cfg.SetDefault("WATERMILL_CB_ENABLED", true)
	cfg.SetDefault("WATERMILL_CB_TIMEOUT", "30s")
	cfg.SetDefault("WATERMILL_CB_INTERVAL", "0s")
	cfg.SetDefault("WATERMILL_CB_FAILURE_THRESHOLD", 5)
	cfg.SetDefault("WATERMILL_CB_HALFOPEN_MAX_REQUESTS", 1)

	cfg.SetDefault("WATERMILL_DLQ_ENABLED", false)
	cfg.SetDefault("WATERMILL_DLQ_TOPIC", "")

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
		timeout.Duration = defaultHandlerTimeoutSec * time.Second
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
		MaxRequests: uint32(cfg.GetInt("WATERMILL_CB_HALFOPEN_MAX_REQUESTS")), //nolint:gosec // the value comes from configuration and is range-checked above
	}
	if cbSettings.Timeout <= 0 {
		cbSettings.Timeout = defaultBreakerTimeoutSec * time.Second
	}

	if cbSettings.MaxRequests == 0 {
		cbSettings.MaxRequests = 1
	}

	cbSettings.ReadyToTrip = func(counts gobreaker.Counts) bool {
		return counts.ConsecutiveFailures >= uint32(failureThreshold) //nolint:gosec // the value comes from configuration and is range-checked above
	}

	breaker := CircuitBreakerOptions{
		Enabled:  cfg.GetBool("WATERMILL_CB_ENABLED"),
		Settings: cbSettings,
	}

	// Publisher stays nil: the client's own publisher does not exist yet, and
	// New fills it in.
	poison := PoisonOptions{
		Enabled:     cfg.GetBool("WATERMILL_DLQ_ENABLED"),
		Topic:       cfg.GetString("WATERMILL_DLQ_TOPIC"),
		ServiceName: serviceName,
	}

	return Options{
		Retry:          retry,
		Timeout:        timeout,
		CircuitBreaker: breaker,
		Poison:         poison,
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

// WithPoisonQueue enables the dead-letter queue and sends dead letters to
// publisher on topic. An empty topic routes each message to
// "<received_topic>.DLQ"; a nil publisher means the client's own publisher.
//
// Use this instead of adding NewShortlinkPoisonMiddleware to the router by
// hand: the option lets New place the middleware outside retry, so a message
// is dead-lettered only after its retries are spent. Added by hand after New,
// the middleware lands inside retry and disables it.
//
//	client, err := watermill.New(ctx, log, cfg, backend, meter, tracer,
//		watermill.WithPoisonQueue(publisher, "shortlink.dlq"),
//	)
func WithPoisonQueue(publisher message.Publisher, topic string) Option {
	return func(o *Options) {
		o.Poison.Enabled = true
		o.Poison.Publisher = publisher
		o.Poison.Topic = topic
	}
}

// WithPoisonQueueFilter is WithPoisonQueue with a say in what counts as
// poison. The filter reports whether a failure that outlived retry should be
// dead-lettered; see NewShortlinkPoisonMiddlewareWithFilter for the motivating
// case.
func WithPoisonQueueFilter(publisher message.Publisher, topic string, shouldGoToPoisonQueue func(err error) bool) Option {
	return func(o *Options) {
		o.Poison.Enabled = true
		o.Poison.Publisher = publisher
		o.Poison.Topic = topic
		o.Poison.Filter = shouldGoToPoisonQueue
	}
}

// WithPoisonOptions overrides the dead-letter middleware configuration.
func WithPoisonOptions(opts PoisonOptions) Option {
	return func(o *Options) {
		o.Poison = opts
	}
}

// DisablePoisonQueue disables the dead-letter middleware, overriding
// WATERMILL_DLQ_ENABLED.
func DisablePoisonQueue() Option {
	return func(o *Options) {
		o.Poison.Enabled = false
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
