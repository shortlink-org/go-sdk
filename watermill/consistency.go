package watermill

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/shortlink-org/go-sdk/logger"
)

// MetaWALWatermark carries the database watermark a handler must observe
// before it processes a message. The name follows the otel_trace_id convention
// already used by InjectTrace.
const MetaWALWatermark = "wal_watermark"

// unresolvedWatermark is intentionally not a valid database token. It says a
// write happened but its exact position could not be captured; the database
// adapter therefore pins the consumer to the primary when Apply sees it.
const unresolvedWatermark = "unresolved"

// defaultGateMaxWait bounds how long a handler waits inline for the read
// replica to catch up before giving up and letting the retry middleware take
// over.
const defaultGateMaxWait = 250 * time.Millisecond

// ErrReplicaNotCaughtUp reports that the read replica has not yet replayed the
// writes that produced this message.
//
// It is a distinct error so that the poison-queue filter can tell it apart
// from a message that is actually broken: a message that is merely early must
// not end up in the dead-letter queue.
var ErrReplicaNotCaughtUp = errors.New("watermill: read replica has not caught up with this message")

// Watermarker produces a token describing the writes made while handling the
// current context.
//
// It is declared here, rather than imported, so that this module never depends
// on the db module — Go interfaces are structural, and *postgres.Router
// satisfies this one as it stands.
type Watermarker interface {
	// Capture returns the token. Empty means nothing was written; an error means
	// a write happened but its position could not be resolved.
	Capture(ctx context.Context) (string, error)
}

// ReplicaGate reports whether a read replica has replayed a given token, and
// waits briefly if it has not.
//
// Also declared locally, for the same reason.
type ReplicaGate interface {
	// Await returns whether a replica is ready to serve reads that must
	// observe the token, waiting up to maxWait for it to become so.
	Await(ctx context.Context, token string, maxWait time.Duration) (bool, error)

	// Apply returns a context carrying the guarantee the token describes, so
	// that reads made by the handler are routed accordingly.
	Apply(ctx context.Context, token string) context.Context

	// Scope returns a context whose reads may use a replica. A message that
	// carries no watermark still has to be scoped, or every read its handler
	// makes falls back to the primary and the replica stays idle.
	Scope(ctx context.Context) context.Context
}

// NewConsistencyPublisher stamps outgoing messages with the watermark of the
// writes that produced them.
//
// Wrap the publisher your handlers use:
//
//	pub := watermill.NewConsistencyPublisher(client.Publisher, router, log, meter)
//
// A failure to read the watermark is counted and swallowed. Failing a publish
// because a consistency optimization did not work would trade correctness for
// availability in the wrong direction: the consumer simply falls back to the
// primary, which is what it did before this existed.
//
//nolint:ireturn // it decorates a message.Publisher, so it must be one
func NewConsistencyPublisher(
	publisher message.Publisher,
	watermarker Watermarker,
	log logger.Logger,
	provider metric.MeterProvider,
) message.Publisher {
	return &consistencyPublisher{
		Publisher:   publisher,
		watermarker: watermarker,
		log:         log,
		stamps:      consistencyCounter(provider, "watermill_consistency_stamps_total", "Messages stamped with a database watermark."),
	}
}

type consistencyPublisher struct {
	message.Publisher

	watermarker Watermarker
	log         logger.Logger
	stamps      metric.Int64Counter
}

type publicationWatermarkState uint8

const (
	publicationWatermarkAbsent publicationWatermarkState = iota
	publicationWatermarkResolved
	publicationWatermarkUnresolved
)

// publicationWatermark is the publisher-side consistency state. It converts
// the structural Watermarker triple once, then owns both wire encoding and
// metrics outcome so those concerns cannot interpret the flags differently.
type publicationWatermark struct {
	state publicationWatermarkState
	token string
	err   error
}

func capturePublicationWatermark(ctx context.Context, watermarker Watermarker) publicationWatermark {
	token, err := watermarker.Capture(ctx)
	if err != nil {
		return publicationWatermark{state: publicationWatermarkUnresolved, err: err}
	}

	if token == "" {
		return publicationWatermark{state: publicationWatermarkAbsent}
	}

	return publicationWatermark{state: publicationWatermarkResolved, token: token}
}

func (w publicationWatermark) wireValue() string {
	if w.state == publicationWatermarkResolved {
		return w.token
	}

	if w.state == publicationWatermarkUnresolved {
		return unresolvedWatermark
	}

	return ""
}

func (w publicationWatermark) outcome() string {
	switch {
	case w.err != nil:
		return "error"
	case w.state == publicationWatermarkAbsent:
		return "nothing_written"
	case w.state == publicationWatermarkUnresolved:
		return "unresolved"
	default:
		return "stamped"
	}
}

func (w publicationWatermark) stamp(messages []*message.Message) {
	if raw := w.wireValue(); raw != "" {
		for _, msg := range messages {
			msg.Metadata.Set(MetaWALWatermark, raw)
		}
	}
}

// Publish implements message.Publisher.
func (p *consistencyPublisher) Publish(topic string, messages ...*message.Message) error {
	if p.watermarker != nil && len(messages) > 0 {
		p.stamp(messages)
	}

	return p.Publisher.Publish(topic, messages...)
}

func (p *consistencyPublisher) stamp(messages []*message.Message) {
	ctx := ensureContext(messages[0].Context())
	watermark := capturePublicationWatermark(ctx, p.watermarker)

	watermark.stamp(messages)
	p.count(ctx, watermark.outcome())

	if watermark.err != nil {
		if p.log != nil {
			p.log.Debug("watermill: could not read the database watermark",
				slog.String("error", watermark.err.Error()),
			)
		}
	}
}

func (p *consistencyPublisher) count(ctx context.Context, outcome string) {
	if p.stamps == nil {
		return
	}

	p.stamps.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// ConsistencyOptions configure the read-your-writes gate for handlers.
type ConsistencyOptions struct {
	// MaxWait bounds the inline wait before the handler gives up and returns
	// ErrReplicaNotCaughtUp. Zero selects 250ms.
	MaxWait time.Duration
}

// NewConsistencyMiddleware makes a handler wait, briefly, for the read replica
// to replay the writes that produced the message it is about to process.
//
// Add it after the retry middleware, so the error path is retried, and after
// the tracing middleware, so the wait is attributed to the consume span.
//
// It waits rather than nacking immediately, for three reasons that all point
// the same way. A nack feeds the retry middleware, and with the default three
// retries a message that is early by tens of milliseconds can exhaust them
// during a one-second replica hiccup and land in the dead-letter queue as
// though it were malformed — a consistency feature manufacturing dead letters.
// On Kafka the nack buys nothing anyway: the same message is redelivered after
// a sleep without the offset advancing, so the partition is blocked either
// way, and the retry counter moves for nothing. And an inline wait keeps
// ordering trivially, where a nack under concurrent handlers does not.
//
// The wait is bounded because a standby resolving a query conflict can freeze
// replay for as long as max_standby_streaming_delay, and blocking that long
// would collide with the handler timeout and produce a worse error than the
// honest one.
func NewConsistencyMiddleware(gate ReplicaGate, opts ConsistencyOptions, provider metric.MeterProvider) message.HandlerMiddleware {
	if opts.MaxWait <= 0 {
		opts.MaxWait = defaultGateMaxWait
	}

	results := consistencyCounter(provider, "watermill_consistency_gate_results_total",
		"Outcomes of waiting for a read replica to replay a message's watermark.")

	return func(next message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			token := msg.Metadata.Get(MetaWALWatermark)
			if token == "" || gate == nil {
				count(msg.Context(), results, "no_watermark")

				if gate != nil {
					msg.SetContext(gate.Scope(ensureContext(msg.Context())))
				}

				return next(msg)
			}

			ctx := ensureContext(msg.Context())

			ready, err := gate.Await(ctx, token, opts.MaxWait)
			if err != nil {
				count(ctx, results, "gate_error")

				return nil, err
			}

			if !ready {
				count(ctx, results, "not_caught_up")

				return nil, ErrReplicaNotCaughtUp
			}

			count(ctx, results, "ready")

			// Apply the guarantee even once the gate has opened: between this
			// check and the handler's own reads, the replica it picks may not
			// be the one that satisfied the check.
			msg.SetContext(gate.Apply(ctx, token))

			return next(msg)
		}
	}
}

// IsConsistencyError reports whether err is a message that arrived before its
// writes did, rather than a message that is broken.
//
// Pass it to the poison-queue filter so that an early message is retried
// instead of dead-lettered:
//
//	NewShortlinkPoisonMiddlewareWithFilter(pub, topic, func(err error) bool {
//		return !watermill.IsConsistencyError(err)
//	})
func IsConsistencyError(err error) bool {
	return errors.Is(err, ErrReplicaNotCaughtUp)
}

//nolint:ireturn // metric.Int64Counter is the otel API's own type
func consistencyCounter(provider metric.MeterProvider, name, description string) metric.Int64Counter {
	if provider == nil {
		return nil
	}

	counter, err := provider.Meter("watermill").Int64Counter(
		name,
		metric.WithDescription(description),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil
	}

	return counter
}

func count(ctx context.Context, counter metric.Int64Counter, outcome string) {
	if counter == nil {
		return
	}

	counter.Add(ensureContext(ctx), 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}
