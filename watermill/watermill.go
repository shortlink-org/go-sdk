package watermill

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/hashicorp/go-multierror"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	watermilldlq "github.com/shortlink-org/go-sdk/watermill/dlq"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Backend interface — реализация MQ backend (Kafka, RabbitMQ, NATS…)
type Backend interface {
	Publisher() message.Publisher
	Subscriber() message.Subscriber
	Close() error
}

// Client — основной объект Watermill для сервисов.
type Client struct {
	Router     *message.Router
	Publisher  message.Publisher
	Subscriber message.Subscriber
	backend    Backend
}

// New — создаёт Watermill Router + middleware + logger + OTEL + metrics.
// backend должен быть создан "снаружи" (например, через kafka.New()).
// meterProvider и tracerProvider должны быть переданы явно (например, из observability/metrics и observability/tracing).
func New(
	ctx context.Context,
	log logger.Logger,
	cfg *config.Config,
	backend Backend,
	meterProvider metric.MeterProvider,
	tracerProvider trace.TracerProvider,
	options ...Option,
) (*Client, error) {
	if backend == nil {
		return nil, fmt.Errorf("backend is nil — must be provided explicitly")
	}

	wmLogger := NewWatermillLogger(log)
	watermilldlq.SetLogger(log)

	router, err := message.NewRouter(message.RouterConfig{}, wmLogger)
	if err != nil {
		return nil, err
	}

	optsCfg := defaultOptions(cfg)
	for _, opt := range options {
		if opt == nil {
			continue
		}
		opt(&optsCfg)
	}

	// Global middleware (panic, retry, correlation, timeout, circuit breaker)
	configureBaseMiddlewares(router, log, wmLogger, optsCfg)
	cfg.SetDefault("WATERMILL_DLQ_ENABLED", false)
	cfg.SetDefault("WATERMILL_DLQ_TOPIC", "")

	// OTEL tracing middleware
	otelMW := NewOTELMiddleware(tracerProvider)
	router.AddMiddleware(otelMW.HandlerMiddleware())

	// OTEL metrics / exemplars middleware
	metricsMW, err := NewMetricsMiddleware(log, meterProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics middleware: %w", err)
	}

	publisher := metricsMW.PublisherWrapper(backend.Publisher(), otelMW)

	if cfg.GetBool("WATERMILL_DLQ_ENABLED") {
		dlqTopic := cfg.GetString("WATERMILL_DLQ_TOPIC")
		router.AddMiddleware(NewShortlinkPoisonMiddleware(publisher, dlqTopic))
	}

	router.AddMiddleware(metricsMW.HandlerMiddleware())

	client := &Client{
		Router:     router,
		Publisher:  publisher,
		Subscriber: backend.Subscriber(),
		backend:    backend,
	}

	return client, nil
}

// Close gracefully closes all resources and collects all errors.
func (c *Client) Close() error {
	var errs *multierror.Error

	if c.Router != nil {
		if err := c.Router.Close(); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to close router: %w", err))
		}
	}

	if c.backend != nil {
		if err := c.backend.Close(); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to close backend: %w", err))
		}
	}

	return errs.ErrorOrNil()
}
