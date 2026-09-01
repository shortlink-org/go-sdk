package outbox

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db"
)

// Handler processes one message from the outbox. Returning an error nacks the
// message, which puts it in the hands of whatever middleware the router
// carries — retry first, then the poison queue.
type Handler func(ctx context.Context, msg *message.Message) error

// Relay reads the outbox and delivers it through a Watermill router.
//
// The router is the caller's, and so is its middleware: retry, the poison
// queue, correlation ids, metrics and tracing all apply to outbox delivery
// exactly as they apply to anything else the service consumes. Pass the router
// from watermill.New and call Relay.Run instead of Router.Run — the relay's
// handlers and the service's own live on the same router, and one Run drives
// both.
type Relay struct {
	pool   *pgxpool.Pool
	log    *slog.Logger
	router *message.Router
	sub    *subscriber
	opts   Options

	topics map[string]struct{}
}

// NewRelay returns a relay reading store and delivering through router.
func NewRelay(store db.DB, log *slog.Logger, router *message.Router, opts ...Option) (*Relay, error) {
	if store == nil {
		return nil, ErrNilStore
	}

	if log == nil {
		return nil, ErrNilLogger
	}

	if router == nil {
		return nil, ErrNilRouter
	}

	pool, err := db.Conn[*pgxpool.Pool](store)
	if err != nil {
		return nil, err
	}

	options := applyOptions(opts)

	return &Relay{
		pool:   pool,
		log:    log,
		router: router,
		sub:    newSubscriber(pool, log, options),
		opts:   options,
		topics: make(map[string]struct{}),
	}, nil
}

// Handle registers h as the consumer of topic.
//
// One handler per topic: the relays of a topic share a single cursor, so two
// handlers would split its messages between them rather than each seeing all
// of them.
func (r *Relay) Handle(topic string, handler Handler) error {
	if topic == "" {
		return ErrNoTopic
	}

	if _, exists := r.topics[topic]; exists {
		return &DuplicateTopicError{Topic: topic}
	}

	r.topics[topic] = struct{}{}

	r.router.AddConsumerHandler(
		"outbox."+topic,
		topic,
		r.sub,
		func(msg *message.Message) error {
			return handler(msg.Context(), msg)
		},
	)

	return nil
}

// Run delivers until ctx is done. It starts the reaper and then runs the
// router, so a service that shares its router with the relay calls this
// instead of Router.Run.
func (r *Relay) Run(ctx context.Context) error {
	if len(r.topics) == 0 {
		r.log.Warn("outbox: relay is running with no handlers, nothing will be delivered")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	reaped := make(chan error, 1)

	go func() {
		reaped <- r.reap(ctx)
	}()

	err := r.router.Run(ctx)

	// Stop the reaper before reporting, so that Run returning means the relay
	// has actually stopped touching the database.
	cancel()

	return errors.Join(err, <-reaped)
}

// Close releases the relay's read loops. Run already does this on its way out;
// Close is for a relay that was built and never run.
func (r *Relay) Close() error {
	return r.sub.Close()
}

// Subscriber exposes the outbox as a Watermill subscriber, for a service that
// would rather wire the handlers itself.
//
// The registration Handle performs is the supported path; this is the escape
// hatch, and it comes with the same rule — one subscription per topic, because
// the topic has one cursor.
//
//nolint:ireturn // message.Subscriber is Watermill's own contract
func (r *Relay) Subscriber() message.Subscriber {
	return r.sub
}
