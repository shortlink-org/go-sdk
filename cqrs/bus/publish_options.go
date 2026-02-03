package bus

import (
	wmmessage "github.com/ThreeDotsLabs/watermill/message"
)

// PublishOption configures a single Publish call (e.g. use a tx-scoped publisher).
type PublishOption func(*publishOptions)

type publishOptions struct {
	publisher wmmessage.Publisher
}

// WithPublisher uses the given publisher for this call only.
// Use it for transactional outbox: create a tx-scoped publisher (e.g. watermill-sql
// from pgx.Tx), optionally wrap with forwarder.NewPublisher, and pass it so the event
// is written in the same transaction as your data.
func WithPublisher(pub wmmessage.Publisher) PublishOption {
	return func(o *publishOptions) {
		o.publisher = pub
	}
}
