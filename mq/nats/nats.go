package nats

import (
	"context"
	"errors"
	"log/slog"
	"net/url"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/mq/query"
)

func New(cfg *config.Config) *MQ {
	return &MQ{
		subscribes: make(map[string]*subscription),
		cfg:        cfg,
	}
}

func (mq *MQ) Init(ctx context.Context, log *slog.Logger) error {
	// Set configuration
	err := mq.setConfig()
	if err != nil {
		return err
	}

	// Connect to a server
	mq.client, err = nats.Connect(mq.config.URI.String())
	if err != nil {
		return err
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		errClose := mq.close()
		if errClose != nil {
			log.Error("NATS close",
				slog.String("error", errClose.Error()),
			)
		}
	}()

	return err
}

// close - stop the subscriptions and drain the connection
func (mq *MQ) close() error {
	var errs error

	mq.mu.Lock()
	subs := make([]*subscription, 0, len(mq.subscribes))

	for name, sub := range mq.subscribes {
		subs = append(subs, sub)
		delete(mq.subscribes, name)
	}
	mq.mu.Unlock()

	for _, sub := range subs {
		errs = errors.Join(errs, sub.stop())
	}

	return errors.Join(errs, mq.client.Drain())
}

// Publish - publish a message
//
// The subject is the target, matching the subject Subscribe listens on. Core NATS has
// no notion of a message key, so routingKey is not used.
func (mq *MQ) Publish(ctx context.Context, target string, _, payload []byte) error {
	msg := &nats.Msg{
		Subject: target,
		Data:    payload,
	}

	// Carry the trace context in the message headers. Headers require a NATS server
	// 2.2 or newer, hence the capability check.
	if mq.client.HeadersSupported() {
		msg.Header = make(nats.Header)
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))
	}

	err := mq.client.PublishMsg(msg)
	if err != nil {
		return err
	}

	return nil
}

// Subscribe - subscribe to message
func (mq *MQ) Subscribe(ctx context.Context, target string, message query.Response) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if _, exists := mq.subscribes[target]; exists {
		return nil
	}

	ch := make(chan *nats.Msg, mq.config.ChannelSize)

	sub, err := mq.client.ChanSubscribe(target, ch)
	if err != nil {
		return err
	}

	subCtx, cancel := context.WithCancel(ctx)

	entry := &subscription{
		sub:    sub,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	mq.subscribes[target] = entry

	go mq.consume(subCtx, entry, ch, message)

	return nil
}

// consume - forward messages to the subscriber until the subscription is stopped.
func (mq *MQ) consume(ctx context.Context, sub *subscription, ch <-chan *nats.Msg, message query.Response) {
	defer close(sub.done)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}

			// we can get a nil message if we close the connection
			if msg == nil {
				continue
			}

			// Restore the trace context injected by the publisher, so that the subscriber
			// continues the original trace instead of starting a detached one.
			msgCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Header))

			// A subscriber that stopped reading must not block the shutdown.
			select {
			case message.Chan <- query.ResponseMessage{
				Context: msgCtx,
				Body:    msg.Data,
			}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// UnSubscribe - unsubscribe from message
func (mq *MQ) UnSubscribe(name string) error {
	mq.mu.Lock()

	sub, exists := mq.subscribes[name]
	if !exists {
		mq.mu.Unlock()

		return nil
	}

	delete(mq.subscribes, name)
	mq.mu.Unlock()

	// Stopping waits for the draining goroutine, so it must not hold the lock.
	return sub.stop()
}

// setConfig - set configuration
func (mq *MQ) setConfig() error {
	mq.cfg.SetDefault("MQ_NATS_URI", "nats://localhost:4222") // NATS_URI
	//nolint:revive,mnd // ignore magics numbers
	mq.cfg.SetDefault("MQ_NATS_CHANNEL_SIZE", 64) // NATS_CHANNEL_SIZE

	// parse uri
	uri, err := url.Parse(mq.cfg.GetString("MQ_NATS_URI"))
	if err != nil {
		return err
	}

	// set config
	mq.config = &Config{
		URI:         uri,
		ChannelSize: mq.cfg.GetInt("MQ_NATS_CHANNEL_SIZE"),
	}

	return nil
}
