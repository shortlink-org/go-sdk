package handlers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/cqrs/bus"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
)

type createOrder struct {
	OrderID string `json:"orderId"`
}

type orderCreated struct {
	OrderID string `json:"orderId"`
}

// recordingHandler captures the payload it was handed and can be told to fail.
type recordingHandler[T any] struct {
	got      T
	calls    atomic.Int32
	failWith error
}

func (h *recordingHandler[T]) Handle(_ context.Context, payload T) error {
	h.calls.Add(1)
	h.got = payload

	return h.failWith
}

//nolint:ireturn // Marshaler is the contract the handlers take.
func newMarshaler() cqrsmessage.Marshaler {
	return cqrsmessage.NewJSONMarshaler(cqrsmessage.NewShortlinkNamer("billing"))
}

// encode produces the Watermill message the dispatcher would receive for v.
func encode(t *testing.T, marshaler cqrsmessage.Marshaler, v any) *wmmessage.Message {
	t.Helper()

	msg, err := marshaler.Marshal(context.Background(), v)
	require.NoError(t, err)

	return msg
}

func TestNewCommandHandlerDispatchesTypedPayload(t *testing.T) {
	registry := bus.NewTypeRegistry()
	require.NoError(t, registry.RegisterCommand(&createOrder{}))

	marshaler := newMarshaler()
	logic := &recordingHandler[*createOrder]{}

	handler := NewCommandHandler[*createOrder](logic, registry, marshaler)

	produced, err := handler(encode(t, marshaler, &createOrder{OrderID: "order-1"}))

	require.NoError(t, err)
	require.Nil(t, produced)
	require.Equal(t, int32(1), logic.calls.Load())
	require.Equal(t, "order-1", logic.got.OrderID)
}

func TestNewEventHandlerDispatchesTypedPayload(t *testing.T) {
	registry := bus.NewTypeRegistry()
	require.NoError(t, registry.RegisterEvent(&orderCreated{}))

	marshaler := newMarshaler()
	logic := &recordingHandler[*orderCreated]{}

	handler := NewEventHandler[*orderCreated](logic, registry, marshaler)

	_, err := handler(encode(t, marshaler, &orderCreated{OrderID: "order-2"}))

	require.NoError(t, err)
	require.Equal(t, "order-2", logic.got.OrderID)
}

// A command registered on the registry must not be resolvable as an event, and
// the other way round — otherwise a handler could be fed the wrong payload.
func TestCommandAndEventRegistriesAreSeparate(t *testing.T) {
	registry := bus.NewTypeRegistry()
	require.NoError(t, registry.RegisterCommand(&createOrder{}))

	marshaler := newMarshaler()
	handler := NewEventHandler[*createOrder](&recordingHandler[*createOrder]{}, registry, marshaler)

	_, err := handler(encode(t, marshaler, &createOrder{OrderID: "order-3"}))

	require.ErrorIs(t, err, errEventNotRegistered)
}

func TestCommandHandlerErrorPaths(t *testing.T) {
	marshaler := newMarshaler()

	registry := bus.NewTypeRegistry()
	require.NoError(t, registry.RegisterCommand(&createOrder{}))

	msg := encode(t, marshaler, &createOrder{OrderID: "order-4"})

	tests := []struct {
		name      string
		logic     CommandHandler[*createOrder]
		registry  *bus.TypeRegistry
		marshaler cqrsmessage.Marshaler
		msg       *wmmessage.Message
		wantErr   error
	}{
		{
			name:      "nil message",
			logic:     &recordingHandler[*createOrder]{},
			registry:  registry,
			marshaler: marshaler,
			msg:       nil,
			wantErr:   errNilMessage,
		},
		{
			name:      "nil logic",
			logic:     nil,
			registry:  registry,
			marshaler: marshaler,
			msg:       msg,
			wantErr:   errNilCommandLogic,
		},
		{
			name:      "nil registry",
			logic:     &recordingHandler[*createOrder]{},
			registry:  nil,
			marshaler: marshaler,
			msg:       msg,
			wantErr:   errNilRegistry,
		},
		{
			name:      "nil marshaler",
			logic:     &recordingHandler[*createOrder]{},
			registry:  registry,
			marshaler: nil,
			msg:       msg,
			wantErr:   errNilMarshaler,
		},
		{
			name:      "command not registered",
			logic:     &recordingHandler[*createOrder]{},
			registry:  bus.NewTypeRegistry(),
			marshaler: marshaler,
			msg:       msg,
			wantErr:   errCommandNotRegistered,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewCommandHandler(tt.logic, tt.registry, tt.marshaler)

			_, err := handler(tt.msg)

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// A failure inside the business logic reaches the caller wrapped, not swallowed.
func TestCommandHandlerPropagatesLogicError(t *testing.T) {
	registry := bus.NewTypeRegistry()
	require.NoError(t, registry.RegisterCommand(&createOrder{}))

	marshaler := newMarshaler()
	sentinel := errors.New("insufficient funds")
	logic := &recordingHandler[*createOrder]{failWith: sentinel}

	handler := NewCommandHandler[*createOrder](logic, registry, marshaler)

	_, err := handler(encode(t, marshaler, &createOrder{OrderID: "order-5"}))

	require.ErrorIs(t, err, sentinel)
}

// The registry holds *createOrder while the handler expects *orderCreated, so
// the payload must be rejected rather than silently mis-typed.
func TestCommandHandlerRejectsTypeMismatch(t *testing.T) {
	registry := bus.NewTypeRegistry()
	require.NoError(t, registry.RegisterCommand(&createOrder{}))

	marshaler := newMarshaler()
	handler := NewCommandHandler[*orderCreated](&recordingHandler[*orderCreated]{}, registry, marshaler)

	_, err := handler(encode(t, marshaler, &createOrder{OrderID: "order-6"}))

	require.ErrorIs(t, err, errHandlerTypeMismatch)
}

func TestDecorateHandlerNilStaysNil(t *testing.T) {
	require.Nil(t, DecorateHandler(nil, DecoratorConfig{}))
}

// Recoverer is always installed, so a panicking handler becomes an error
// instead of taking the process down.
func TestDecorateHandlerRecoversPanic(t *testing.T) {
	panicking := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		panic("boom")
	}

	decorated := DecorateHandler(panicking, DecoratorConfig{})

	_, err := decorated(wmmessage.NewMessage("id", nil))

	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestDecorateHandlerRetriesUntilSuccess(t *testing.T) {
	var calls atomic.Int32

	flaky := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		if calls.Add(1) < 3 {
			return nil, errors.New("transient")
		}

		return nil, nil
	}

	decorated := DecorateHandler(flaky, DecoratorConfig{RetryMax: 5})

	_, err := decorated(wmmessage.NewMessage("id", nil))

	require.NoError(t, err)
	require.Equal(t, int32(3), calls.Load())
}

func TestDecorateHandlerGivesUpAfterRetryMax(t *testing.T) {
	var calls atomic.Int32

	always := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		calls.Add(1)

		return nil, errors.New("permanent")
	}

	decorated := DecorateHandler(always, DecoratorConfig{RetryMax: 2})

	_, err := decorated(wmmessage.NewMessage("id", nil))

	require.Error(t, err)
	// The first attempt is not a retry, so RetryMax=2 means three calls.
	require.Equal(t, int32(3), calls.Load())
}

func TestDecorateHandlerAppliesTimeout(t *testing.T) {
	slow := func(msg *wmmessage.Message) ([]*wmmessage.Message, error) {
		<-msg.Context().Done()

		return nil, msg.Context().Err()
	}

	decorated := DecorateHandler(slow, DecoratorConfig{Timeout: 10 * time.Millisecond})

	msg := wmmessage.NewMessage("id", nil)
	msg.SetContext(context.Background())

	_, err := decorated(msg)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// With the breaker open the handler is no longer called; the error comes from
// gobreaker instead of the business logic.
func TestDecorateHandlerOpensCircuitBreaker(t *testing.T) {
	var calls atomic.Int32

	failing := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		calls.Add(1)

		return nil, errors.New("down")
	}

	settings := gobreaker.Settings{
		Name: "test",
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	}

	decorated := DecorateHandler(failing, DecoratorConfig{
		CircuitBreakerEnabled:  true,
		CircuitBreakerSettings: &settings,
	})

	msg := wmmessage.NewMessage("id", nil)

	_, firstErr := decorated(msg)
	require.Error(t, firstErr)

	_, secondErr := decorated(msg)
	require.ErrorIs(t, secondErr, gobreaker.ErrOpenState)
	require.Equal(t, int32(1), calls.Load(), "handler must not be called while the breaker is open")
}

func TestDecorateHandlerUsesDefaultBreakerSettings(t *testing.T) {
	handler := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		return nil, nil
	}

	decorated := DecorateHandler(handler, DecoratorConfig{CircuitBreakerEnabled: true})

	_, err := decorated(wmmessage.NewMessage("id", nil))
	require.NoError(t, err)

	settings := defaultCircuitBreakerSettings()
	require.Equal(t, circuitBreakerTimeout, settings.Timeout)
	require.Equal(t, uint32(circuitBreakerMaxRequests), settings.MaxRequests)
	require.True(t, settings.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: circuitBreakerFailureThreshold}))
	require.False(t, settings.ReadyToTrip(gobreaker.Counts{ConsecutiveFailures: circuitBreakerFailureThreshold - 1}))
}

func TestAsMiddlewareDecorates(t *testing.T) {
	panicking := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		panic("boom")
	}

	mw := AsMiddleware(DecoratorConfig{})

	_, err := mw(panicking)(wmmessage.NewMessage("id", nil))

	require.Error(t, err)
}

func TestChainAppliesInOrderAndSkipsNil(t *testing.T) {
	var order []string

	record := func(tag string) wmmessage.HandlerMiddleware {
		return func(next wmmessage.HandlerFunc) wmmessage.HandlerFunc {
			return func(msg *wmmessage.Message) ([]*wmmessage.Message, error) {
				order = append(order, tag)

				return next(msg)
			}
		}
	}

	base := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		order = append(order, "handler")

		return nil, nil
	}

	chained := Chain(base, record("first"), nil, record("second"))

	_, err := chained(wmmessage.NewMessage("id", nil))

	require.NoError(t, err)
	// Chain wraps left to right, so the last middleware ends up outermost.
	require.Equal(t, []string{"second", "first", "handler"}, order)
}

func TestChainWithoutMiddlewaresReturnsHandler(t *testing.T) {
	var called bool

	base := func(*wmmessage.Message) ([]*wmmessage.Message, error) {
		called = true

		return nil, nil
	}

	_, err := Chain(base)(wmmessage.NewMessage("id", nil))

	require.NoError(t, err)
	require.True(t, called)
}
