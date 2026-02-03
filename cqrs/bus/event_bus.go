package bus

import (
	"context"
	"errors"
	"fmt"

	wmmessage "github.com/ThreeDotsLabs/watermill/message"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
	"github.com/shortlink-org/go-sdk/uow"
)

var (
	errEventBusUninitialized  = errors.New("cqrs/bus: event bus is not initialized")
	errEventPublisherNil      = errors.New("cqrs/bus: publisher is required")
	errEventMarshalerNil      = errors.New("cqrs/bus: marshaler is required")
	errEventPayloadNil        = errors.New("cqrs/bus: event is nil")
	errEventPublishRequiresTx = errors.New("cqrs/bus: event publishing requires UoW transaction (use Publish inside UnitOfWork)")
)

// EventBus publishes domain events.
type EventBus struct {
	publisher wmmessage.Publisher
	marshaler cqrsmessage.Marshaler
	namer     cqrsmessage.Namer
	forwarder *forwarderState
	txOutbox  *txOutboxConfig
}

// NewEventBus builds EventBus with required dependencies.
func NewEventBus(pub wmmessage.Publisher, marshaler cqrsmessage.Marshaler, namer cqrsmessage.Namer, opts ...Option) *EventBus {
	bus, err := NewEventBusWithOptions(pub, marshaler, namer, opts...)
	if err != nil {
		panic(err)
	}

	return bus
}

// NewEventBusWithOptions builds EventBus and exposes configuration errors.
func NewEventBusWithOptions(
	pub wmmessage.Publisher,
	marshaler cqrsmessage.Marshaler,
	namer cqrsmessage.Namer,
	opts ...Option,
) (*EventBus, error) {
	cfg, err := applyOptions(opts)
	if err != nil {
		return nil, err
	}

	bus := &EventBus{
		publisher: pub,
		marshaler: marshaler,
		namer:     namer,
		txOutbox:  cfg.txOutbox,
	}

	if cfg.outbox != nil {
		bus.forwarder = newForwarderState(cfg.outbox)
		bus.publisher = bus.forwarder.wrapPublisher(pub)
	}
	// When only WithTxAwareOutbox is set, pub may be nil; Publish without tx will return errEventPublishRequiresTx.

	return bus, nil
}

// validate checks that the event bus and its dependencies are properly initialized.
// When only WithTxAwareOutbox is used (no forwarder), publisher may be nil.
func (b *EventBus) validate(evt any) error {
	if b == nil {
		return errEventBusUninitialized
	}
	if b.publisher == nil && (b.txOutbox == nil || b.forwarder != nil) {
		return errEventPublisherNil
	}
	if b.marshaler == nil {
		return errEventMarshalerNil
	}
	if evt == nil {
		return errEventPayloadNil
	}
	return nil
}

// Publish sends event using canonical topic name.
// Optional PublishOption(s) apply to this call only; e.g. WithPublisher(txPublisher) for transactional outbox.
func (b *EventBus) Publish(ctx context.Context, evt any, opts ...PublishOption) error {
	if err := b.validate(evt); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var po publishOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&po)
		}
	}
	publisher := b.publisher
	var txPub wmmessage.Publisher
	if po.publisher != nil {
		publisher = po.publisher
	} else if b.txOutbox != nil && uow.HasTx(ctx) {
		var err error
		txPub, err = newTxPublisher(uow.FromContext(ctx), b.txOutbox)
		if err != nil {
			return fmt.Errorf("tx-scoped publisher: %w", err)
		}
		publisher = txPub
	}
	if publisher == nil {
		return errEventPublishRequiresTx
	}
	defer func() {
		if txPub != nil {
			if c, ok := txPub.(interface{ Close() error }); ok {
				_ = c.Close()
			}
		}
	}()

	var (
		name    string
		topic   string
		service string
	)

	if b.namer != nil {
		name = b.namer.EventName(evt)
		topic = b.namer.TopicForEvent(name)
		service = b.namer.ServiceName()
		ctx = cqrsmessage.WithServiceName(ctx, service)
	} else {
		name = cqrsmessage.NameOf(evt)
		topic = cqrsmessage.TopicForEvent(name)
	}

	msg, err := b.marshaler.Marshal(ctx, evt)
	if err != nil {
		return fmt.Errorf("marshal event %T: %w", evt, err)
	}

	if msg.Metadata.Get(cqrsmessage.MetadataServiceName) == "" && service != "" {
		msg.Metadata.Set(cqrsmessage.MetadataServiceName, service)
	}
	msg.Metadata.Set(cqrsmessage.MetadataMessageKind, string(cqrsmessage.KindEvent))

	cqrsmessage.SetTrace(ctx, msg)

	return publisher.Publish(topic, msg)
}

// RunForwarder starts the optional outbox forwarder when configured.
func (b *EventBus) RunForwarder(ctx context.Context) error {
	if b == nil || b.forwarder == nil {
		return nil
	}

	return b.forwarder.Run(ctx)
}

// CloseForwarder stops the optional forwarder if it was started.
func (b *EventBus) CloseForwarder(ctx context.Context) error {
	if b == nil || b.forwarder == nil {
		return nil
	}

	return b.forwarder.Close(ctx)
}

// EventPublisher exposes EventBus as Publish(ctx, event) for dependency injection.
// Use it as ports.EventPublisher so apps (e.g. OMS) need no local adapter.
type EventPublisher struct{ Bus *EventBus }

// NewEventPublisher wraps EventBus for use as an EventPublisher interface.
func NewEventPublisher(bus *EventBus) *EventPublisher {
	return &EventPublisher{Bus: bus}
}

// Publish publishes the event; when ctx has a transaction (go-sdk/uow), writes to outbox in the same tx.
func (p *EventPublisher) Publish(ctx context.Context, event any) error {
	if p == nil || p.Bus == nil {
		return nil
	}
	return p.Bus.Publish(ctx, event)
}
