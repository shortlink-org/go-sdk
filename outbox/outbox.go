/*
Package outbox writes messages inside the caller's database transaction and
delivers them afterwards.

The write and the delivery are two halves of the same pattern and neither is
useful alone. Publisher appends a message to the outbox table using the
transaction the application already has open, so the message and the aggregate
that produced it commit or roll back together. Relay reads the table and hands
each message to a Watermill router, which brings retry, the poison queue,
correlation ids and metrics with it.

What this package does not decide is what the messages mean. Topic names, the
wire shape of a domain event, and who subscribes to what belong to the service.

# Delivery guarantees

  - AT LEAST ONCE. A handler can see the same message twice: the relay marks a
    row delivered after the handler returns, so a crash in between replays it.
    Handlers must be idempotent.

  - NO ORDERING. None. Not global, not per aggregate. The relay reads a batch
    with FOR UPDATE SKIP LOCKED and the router dispatches the batch
    concurrently, so two messages written by the same transaction can arrive in
    either order, and a message retried after a failure arrives after messages
    written later. If a consumer needs order, it has to carry it in the payload
    — a version, a sequence number, a timestamp — and reject what it has
    already seen.

  - ONE CURSOR PER TOPIC. A delivered row is delivered for everyone: the relays
    of one topic are competing consumers, not independent readers. A second
    consumer of the same events needs its own topic, or a broker downstream of
    the relay. A consumer deployed later sees nothing that was written before
    it existed.

Rows are deleted once they have been delivered for longer than the retention
window; see WithRetention.
*/
package outbox
