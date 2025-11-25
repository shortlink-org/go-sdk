package handlers

import (
	"context"
	"fmt"

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
	expectedType := handlerTypeOf[T]()

	return func(msg *wmmessage.Message) ([]*wmmessage.Message, error) {
		if msg == nil {
			return nil, errNilMessage
		}
		if logic == nil {
			return nil, errNilEventLogic
		}
		if registry == nil {
			return nil, errNilRegistry
		}
		if marshaler == nil {
			return nil, errNilMarshaler
		}

		name := marshaler.NameFromMessage(msg)
		if name == "" {
			name = cqrsmessage.NameOf(msg)
		}

		evtType, ok := registry.ResolveEvent(name)
		if !ok {
			return nil, fmt.Errorf("%w: %s", errEventNotRegistered, name)
		}

		instance := newValue(evtType)
		if err := marshaler.Unmarshal(msg, instance); err != nil {
			return nil, fmt.Errorf("unmarshal event %s: %w", name, err)
		}

		typedEvt, err := typedPayload[T](instance, expectedType, evtType)
		if err != nil {
			return nil, fmt.Errorf("event %s: %w", name, err)
		}

		ctx := msg.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		if err := logic.Handle(ctx, typedEvt); err != nil {
			return nil, fmt.Errorf("handle event %s: %w", name, err)
		}

		return nil, nil
	}
}
