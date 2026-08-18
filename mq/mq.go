/*
Message Queue

A driver becomes selectable by being imported for its side effect:

	import _ "github.com/shortlink-org/go-sdk/mq/kafka"

Only the drivers actually imported are linked into the binary.

Deprecated: This package is deprecated. Use github.com/shortlink-org/go-sdk/watermill instead.
*/
package mq

import (
	"context"
	"log/slog"
	"sort"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/mq/query"
)

// New creates a new MQ instance
//
// When MQ_ENABLED is false it reports ErrDisabled, so a switched-off bus
// cannot be mistaken for a usable one:
//
//	bus, err := mq.New(ctx, log, cfg)
//	if errors.Is(err, mq.ErrDisabled) {
//		return nil
//	}
//
// Deprecated: Use github.com/shortlink-org/go-sdk/watermill instead.
func New(ctx context.Context, log logger.Logger, cfg *config.Config, opts ...Option) (*DataBus, error) {
	cfg.SetDefault("MQ_ENABLED", "false") // Enabled MQ

	if !cfg.GetBool("MQ_ENABLED") {
		return nil, ErrDisabled
	}

	options := &Options{driverOptions: make(map[string][]any)}
	for _, opt := range opts {
		opt(options)
	}

	err := checkOptionTargets(options)
	if err != nil {
		return nil, err
	}

	typeMQ := mqType(cfg)

	factory, ok := lookup(typeMQ)
	if !ok {
		return nil, &UnknownMQTypeError{
			MQType:     typeMQ,
			Registered: Drivers(),
		}
	}

	driver, err := factory(Deps{
		Log:     log,
		Cfg:     cfg,
		driver:  typeMQ,
		options: options.driverOptions[typeMQ],
	})
	if err != nil {
		return nil, err
	}

	dataBus := &DataBus{
		log:    log,
		mq:     driver,
		typeMQ: typeMQ,
		cfg:    cfg,
	}

	err = dataBus.Init(ctx, log)
	if err != nil {
		return nil, err
	}

	return dataBus, nil
}

// Init - init connection
func (mq *DataBus) Init(ctx context.Context, log logger.Logger) error {
	err := mq.mq.Init(ctx, log)
	if err != nil {
		return err
	}

	mq.log.Info("run MQ", slog.String("mq", mq.typeMQ))

	return nil
}

// Subscribe - subscribe to a topic
func (mq *DataBus) Subscribe(ctx context.Context, target string, message query.Response) error {
	mq.log.Info("subscribe to topic",
		slog.String("topic", target),
	)

	return mq.mq.Subscribe(ctx, target, message)
}

// UnSubscribe - unsubscribe to a topic
func (mq *DataBus) UnSubscribe(target string) error {
	mq.log.Info("unsubscribe to topic",
		slog.String("topic", target),
	)

	return mq.mq.UnSubscribe(target)
}

// Publish - publish to a topic
func (mq *DataBus) Publish(ctx context.Context, target string, key, payload []byte) error {
	mq.log.Info("publish to topic",
		slog.String("topic", target),
	)

	return mq.mq.Publish(ctx, target, key, payload)
}

// mqType - resolve the configured MQ type
func mqType(cfg *config.Config) string {
	cfg.SetDefault("MQ_TYPE", defaultMQType) // Select: kafka, rabbitmq, nats, redis

	typeMQ := cfg.GetString("MQ_TYPE")
	if typeMQ == "" {
		return defaultMQType
	}

	return typeMQ
}

// checkOptionTargets - reject options addressed to a driver that is not
// registered. Options for a registered but unselected driver are legitimate
// and stay silent; a name nothing answers to is a wiring mistake.
func checkOptionTargets(options *Options) error {
	targets := make([]string, 0, len(options.driverOptions))
	for driver := range options.driverOptions {
		targets = append(targets, driver)
	}

	sort.Strings(targets)

	for _, driver := range targets {
		if _, ok := lookup(driver); !ok {
			return &UnknownOptionTargetError{
				Driver:     driver,
				Registered: Drivers(),
			}
		}
	}

	return nil
}
