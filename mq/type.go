package mq

import (
	"context"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"github.com/shortlink-org/go-sdk/mq/query"
)

// MQ - common interface of DataBus
//
// Deprecated: Use github.com/shortlink-org/go-sdk/watermill instead.
type MQ interface {
	Init(ctx context.Context, log logger.Logger) error

	// Pub/Sub a pattern
	Publish(ctx context.Context, target string, routingKey, payload []byte) error
	Subscribe(ctx context.Context, target string, message query.Response) error
	UnSubscribe(target string) error
}

// DataBus abstract type
//
// Deprecated: Use github.com/shortlink-org/go-sdk/watermill instead.
type DataBus struct {
	log    logger.Logger
	mq     MQ
	typeMQ string
	cfg    *config.Config
}

// defaultMQType is used when MQ_TYPE is unset or empty.
const defaultMQType = "rabbitmq"

// Options holds the options collected for each driver.
type Options struct {
	// driverOptions maps a driver name to the options addressed to it. They
	// stay untyped here so that this package needs no import of any driver.
	driverOptions map[string][]any
}

// Option is a functional option for configuring the data bus.
type Option func(*Options)
