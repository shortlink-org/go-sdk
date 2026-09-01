package rabbit

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/shortlink-org/go-sdk/config"
)

// ErrNotImplemented reports that the RabbitMQ backend is still a stub.
var ErrNotImplemented = errors.New("RabbitMQ backend is not implemented yet")

type Backend struct{}

func New(ctx context.Context, log *slog.Logger, cfg *config.Config) (*Backend, error) {
	// TODO: implement real AMQP adapter
	//nolint:ireturn // the interface is the library's own contract
	return nil, ErrNotImplemented
	//nolint:ireturn // the interface is the library's own contract
}

//nolint:ireturn // message.Publisher is the Backend contract
func (b *Backend) Publisher() message.Publisher { return nil }

//nolint:ireturn // message.Subscriber is the Backend contract
func (b *Backend) Subscriber() message.Subscriber { return nil }
func (b *Backend) Close() error                   { return nil }
