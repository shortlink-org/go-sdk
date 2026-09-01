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

// typedHandlerDeps bundles the non-generic wiring shared by command and event
// handlers, so the constructor stays within the argument limit.
type typedHandlerDeps struct {
	registry         *bus.TypeRegistry
	marshaler        cqrsmessage.Marshaler
	resolve          resolveRegistered
	errNotRegistered error
	errNilLogic      error
	kind             string
}

func newWatermillTypedHandler[T any](
	handle func(ctx context.Context, payload T) error,
	deps *typedHandlerDeps,
) wmmessage.HandlerFunc {
	expectedType := handlerTypeOf[T]()

	return func(msg *wmmessage.Message) ([]*wmmessage.Message, error) {
		if msg == nil {
			return nil, errNilMessage
		}

		if handle == nil {
			return nil, deps.errNilLogic
		}

		if deps.registry == nil {
			return nil, errNilRegistry
		}

		if deps.marshaler == nil {
			return nil, errNilMarshaler
		}

		name := deps.marshaler.NameFromMessage(msg)
		if name == "" {
			name = cqrsmessage.NameOf(msg)
		}

		payloadType, ok := deps.resolve(deps.registry, name)
		if !ok {
			return nil, fmt.Errorf("%w: %s", deps.errNotRegistered, name)
		}

		instance := newValue(payloadType)

		err := deps.marshaler.Unmarshal(msg, instance)
		if err != nil {
			return nil, fmt.Errorf("unmarshal %s %s: %w", deps.kind, name, err)
		}

		typed, err := typedPayload[T](instance, expectedType, payloadType)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", deps.kind, name, err)
		}

		msgCtx := msg.Context()
		if msgCtx == nil {
			//nolint:contextcheck // Watermill may deliver synthetic messages without context.
			msgCtx = context.Background()
		}

		err = handle(msgCtx, typed)
		if err != nil {
			return nil, fmt.Errorf("handle %s %s: %w", deps.kind, name, err)
		}

		return nil, nil
	}
}
