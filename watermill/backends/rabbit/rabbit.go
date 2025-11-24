package rabbit

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

type Backend struct{}

func New(ctx context.Context, log logger.Logger, cfg *config.Config) (*Backend, error) {
	// TODO: implement real AMQP adapter
	return nil, fmt.Errorf("RabbitMQ backend is not implemented yet")
}

func (b *Backend) Publisher() message.Publisher   { return nil }
func (b *Backend) Subscriber() message.Subscriber { return nil }
func (b *Backend) Close() error                   { return nil }
