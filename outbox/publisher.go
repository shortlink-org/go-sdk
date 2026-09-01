package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5"
)

// TxLookup finds the transaction the application has in flight, or returns nil
// when there is none. It is the same hook the postgres driver takes as
// postgres.WithTxLookup, and uow.FromContext satisfies it:
//
//	outbox.NewPublisher(uow.FromContext)
type TxLookup func(ctx context.Context) pgx.Tx

// insertSQL appends one message. The table name is a constant, not a caller's
// string, so there is no identifier to quote or validate here.
//
//nolint:gosec // tableName is a package constant, not user input
var insertSQL = fmt.Sprintf(
	`INSERT INTO %s (uuid, topic, payload, metadata) VALUES ($1, $2, $3, $4)`,
	tableName,
)

// Publisher appends messages to the outbox inside the caller's transaction.
type Publisher struct {
	lookup TxLookup
}

// NewPublisher returns a Publisher that writes through the transaction lookup
// returns.
func NewPublisher(lookup TxLookup) (*Publisher, error) {
	if lookup == nil {
		return nil, ErrNilTxLookup
	}

	return &Publisher{lookup: lookup}, nil
}

// Publish appends msgs to the outbox on topic.
//
// It requires a transaction in ctx and fails with ErrNoTransaction without
// one. The rows become visible to the relay only when that transaction
// commits, which is the whole point: the message and the state change it
// describes land together or not at all.
func (p *Publisher) Publish(ctx context.Context, topic string, msgs ...*message.Message) error {
	if topic == "" {
		return ErrNoTopic
	}

	if len(msgs) == 0 {
		return nil
	}

	transaction := p.lookup(ctx)
	if transaction == nil {
		return ErrNoTransaction
	}

	var batch pgx.Batch

	for _, msg := range msgs {
		// A map[string]string always marshals; there is no error to handle.
		metadata, _ := json.Marshal(map[string]string(msg.Metadata)) //nolint:errchkjson,errcheck // map[string]string cannot fail

		batch.Queue(insertSQL, msg.UUID, topic, msg.Payload, metadata)
	}

	err := transaction.SendBatch(ctx, &batch).Close()
	if err != nil {
		return fmt.Errorf("outbox: append to %s: %w", tableName, err)
	}

	return nil
}
