package handlers

import (
	"context"
	"fmt"

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
	expectedType := handlerTypeOf[T]()

	return func(msg *wmmessage.Message) ([]*wmmessage.Message, error) {
		if msg == nil {
			return nil, errNilMessage
		}
		if logic == nil {
			return nil, errNilCommandLogic
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

		cmdType, ok := registry.ResolveCommand(name)
		if !ok {
			return nil, fmt.Errorf("%w: %s", errCommandNotRegistered, name)
		}

		instance := newValue(cmdType)
		if err := marshaler.Unmarshal(msg, instance); err != nil {
			return nil, fmt.Errorf("unmarshal command %s: %w", name, err)
		}

		typedCmd, err := typedPayload[T](instance, expectedType, cmdType)
		if err != nil {
			return nil, fmt.Errorf("command %s: %w", name, err)
		}

		ctx := msg.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		if err := logic.Handle(ctx, typedCmd); err != nil {
			return nil, fmt.Errorf("handle command %s: %w", name, err)
		}

		return nil, nil
	}
}
