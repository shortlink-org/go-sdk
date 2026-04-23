package handlers

import (
	"context"
	"fmt"
	"reflect"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"

	"github.com/shortlink-org/go-sdk/cqrs/bus"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

type resolveRegistered func(reg *bus.TypeRegistry, name string) (reflect.Type, bool)

func newWatermillTypedHandler[T any](
	handle func(ctx context.Context, payload T) error,
	registry *bus.TypeRegistry,
	marshaler cqrsmessage.Marshaler,
	resolve resolveRegistered,
	errNotRegistered error,
	errNilLogic error,
	kind string,
) wmmessage.HandlerFunc {
	expectedType := handlerTypeOf[T]()

	return func(msg *wmmessage.Message) ([]*wmmessage.Message, error) {
		if msg == nil {
			return nil, errNilMessage
		}

		if handle == nil {
			return nil, errNilLogic
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

		payloadType, ok := resolve(registry, name)
		if !ok {
			return nil, fmt.Errorf("%w: %s", errNotRegistered, name)
		}

		instance := newValue(payloadType)
		if err := marshaler.Unmarshal(msg, instance); err != nil {
			return nil, fmt.Errorf("unmarshal %s %s: %w", kind, name, err)
		}

		typed, err := typedPayload[T](instance, expectedType, payloadType)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", kind, name, err)
		}

		msgCtx := msg.Context()
		if msgCtx == nil {
			//nolint:contextcheck // Watermill may deliver synthetic messages without context.
			msgCtx = context.Background()
		}

		if err := handle(msgCtx, typed); err != nil {
			return nil, fmt.Errorf("handle %s %s: %w", kind, name, err)
		}

		return nil, nil
	}
}
