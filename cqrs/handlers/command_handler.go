package handlers

import (
	"context"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"

	"github.com/shortlink-org/go-sdk/cqrs/bus"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

// CommandHandler describes business logic for specific command type.
type CommandHandler[T any] interface {
	Handle(ctx context.Context, cmd T) error
}

// NewCommandHandler adapts typed handler to Watermill handler function.
func NewCommandHandler[T any](
	logic CommandHandler[T],
	registry *bus.TypeRegistry,
	marshaler cqrsmessage.Marshaler,
) wmmessage.HandlerFunc {
	var handle func(context.Context, T) error
	if logic != nil {
		handle = logic.Handle
	}

	return newWatermillTypedHandler(handle, registry, marshaler, (*bus.TypeRegistry).ResolveCommand, errCommandNotRegistered, errNilCommandLogic, "command")
}
