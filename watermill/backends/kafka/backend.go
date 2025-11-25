package kafka

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/hashicorp/go-multierror"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	sdkwatermill "github.com/shortlink-org/go-sdk/watermill"
)

// Backend aggregates Kafka publisher/subscriber and satisfies watermill.Backend.
type Backend struct {
	publisher  *Publisher
	subscriber *Subscriber
}

// New wires Kafka publisher and subscriber using config-driven defaults.
func New(_ context.Context, log logger.Logger, cfg *config.Config) (*Backend, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger is nil")
	}

	settings, err := loadBackendSettings(cfg)
	if err != nil {
		return nil, err
	}

	wmLogger := sdkwatermill.NewWatermillLogger(log)

	publisher, err := NewPublisher(settings.publisherConfig(), wmLogger)
	if err != nil {
		return nil, fmt.Errorf("create kafka publisher: %w", err)
	}

	subscriber, err := NewSubscriber(settings.subscriberConfig(), wmLogger)
	if err != nil {
		_ = publisher.Close()
		return nil, fmt.Errorf("create kafka subscriber: %w", err)
	}

	return &Backend{
		publisher:  publisher,
		subscriber: subscriber,
	}, nil
}

// Publisher returns the configured Kafka publisher.
func (b *Backend) Publisher() message.Publisher {
	if b == nil {
		return nil
	}
	return b.publisher
}

// Subscriber returns the configured Kafka subscriber.
func (b *Backend) Subscriber() message.Subscriber {
	if b == nil {
		return nil
	}
	return b.subscriber
}

// Close stops publisher and subscriber, joining all errors.
func (b *Backend) Close() error {
	if b == nil {
		return nil
	}

	var errs *multierror.Error

	if b.publisher != nil {
		if err := b.publisher.Close(); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("close publisher: %w", err))
		}
	}

	if b.subscriber != nil {
		if err := b.subscriber.Close(); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("close subscriber: %w", err))
		}
	}

	return errs.ErrorOrNil()
}

// NewPublisherFromConfig wires only Kafka publisher from config/log.
func NewPublisherFromConfig(log logger.Logger, cfg *config.Config) (*Publisher, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger is nil")
	}

	settings, err := loadBackendSettings(cfg)
	if err != nil {
		return nil, err
	}

	return NewPublisher(settings.publisherConfig(), sdkwatermill.NewWatermillLogger(log))
}

// NewSubscriberFromConfig wires only Kafka subscriber from config/log.
func NewSubscriberFromConfig(log logger.Logger, cfg *config.Config) (*Subscriber, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger is nil")
	}

	settings, err := loadBackendSettings(cfg)
	if err != nil {
		return nil, err
	}

	return NewSubscriber(settings.subscriberConfig(), sdkwatermill.NewWatermillLogger(log))
}
