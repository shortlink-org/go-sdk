package handlers

import (
	"context"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"

	"github.com/shortlink-org/go-sdk/cqrs/bus"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

// EventHandler processes immutable events.
type EventHandler[T any] interface {
	Handle(ctx context.Context, evt T) error
}

// NewEventHandler adapts typed handler to Watermill handler function.
func NewEventHandler[T any](
	logic EventHandler[T],
	registry *bus.TypeRegistry,
	marshaler cqrsmessage.Marshaler,
) wmmessage.HandlerFunc {
	var handle func(context.Context, T) error
	if logic != nil {
		handle = logic.Handle
	}

	return newWatermillTypedHandler(handle, registry, marshaler, (*bus.TypeRegistry).ResolveEvent, errEventNotRegistered, errNilEventLogic, "event")
}
