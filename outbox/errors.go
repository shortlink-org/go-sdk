package outbox

import (
	"errors"
	"fmt"
)

// ErrNoTransaction reports a publish with no transaction in the context.
//
// It is deliberately an error and not a fallback to a pooled connection. A
// message written outside the transaction survives the rollback of the
// aggregate that produced it, which is the exact failure the outbox exists to
// prevent — and it surfaces later, as an event about something that never
// happened. The usual cause is publishing after the commit rather than inside
// it.
var ErrNoTransaction = errors.New("outbox: publish outside a transaction")

// ErrNoTopic reports a publish with an empty topic.
var ErrNoTopic = errors.New("outbox: publish without a topic")

// ErrNilStore reports a relay built without a database.
var ErrNilStore = errors.New("outbox: relay requires a store")

// ErrNilLogger reports a relay built without a logger.
var ErrNilLogger = errors.New("outbox: relay requires a logger")

// ErrNilRouter reports a relay built without a router.
//
// There is no default: the router carries the middleware stack — retry, the
// poison queue, correlation ids, metrics — and a relay that quietly built its
// own would deliver messages under a policy the service never chose.
var ErrNilRouter = errors.New("outbox: relay requires a router")

// ErrNilTxLookup reports a publisher built without a way to find the
// transaction.
var ErrNilTxLookup = errors.New("outbox: publisher requires a transaction lookup")

// ErrRelayClosed reports use of a relay whose Run has already returned.
var ErrRelayClosed = errors.New("outbox: relay is closed")

// DuplicateTopicError reports a second handler for a topic that already has
// one. Two handlers on one topic would each be a competing consumer of the
// same cursor, so each message would reach only one of them — almost never
// what the caller meant.
type DuplicateTopicError struct {
	Topic string
}

func (e *DuplicateTopicError) Error() string {
	return fmt.Sprintf("outbox: topic %q already has a handler", e.Topic)
}
