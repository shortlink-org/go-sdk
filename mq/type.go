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
	Publish(ctx context.Context, target string, routingKey []byte, payload []byte) error
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
