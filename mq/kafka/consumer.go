package kafka

import (
	"sync"

	"github.com/IBM/sarama"
	"github.com/dnwe/otelsarama"
	"go.opentelemetry.io/otel"

	"github.com/shortlink-org/go-sdk/mq/query"
)

// Consumer represents a Sarama consumer group consumer
type Consumer struct {
	// response channel
	ch query.Response

	// ready is closed once the first session has been set up. Setup runs again
	// after every rebalance, so the close is guarded by readyOnce.
	ready     chan struct{}
	readyOnce sync.Once
}

// newConsumer creates a consumer group handler delivering messages to ch.
func newConsumer(ch query.Response) *Consumer {
	return &Consumer{
		ch:    ch,
		ready: make(chan struct{}),
	}
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (consumer *Consumer) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	consumer.readyOnce.Do(func() {
		close(consumer.ready)
	})

	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (consumer *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
// Once the Messages() channel is closed, the Handler must finish its processing
// loop and exit.
//
//nolint:unparam // ignore unused parameter
func (consumer *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// NOTE:
	// Do not move the code below to a goroutine.
	// The `ConsumeClaim` itself is called within a goroutine, see:
	// https://github.com/IBM/sarama/blob/main/consumer_group.go#L27-L29
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			session.MarkMessage(message, "")

			// Restore the trace context injected by the producer, so that the subscriber
			// continues the original trace instead of starting a detached one.
			ctx := otel.GetTextMapPropagator().Extract(session.Context(), otelsarama.NewConsumerMessageCarrier(message))

			// A subscriber that stops reading must not pin the session: a blocked send
			// here would outlive the rebalance timeout and get the member evicted.
			select {
			case consumer.ch.Chan <- query.ResponseMessage{
				Context: ctx,
				Body:    message.Value,
			}:
			case <-session.Context().Done():
				return nil
			}
		// Should return when `session.Context()` is done.
		// If not, will raise `ErrRebalanceInProgress` or `read tcp <ip>:<port>: i/o timeout` when kafka rebalance. See:
		// https://github.com/IBM/sarama/issues/1192
		case <-session.Context().Done():
			return nil
		}
	}
}
