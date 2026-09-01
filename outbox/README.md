# Outbox

Transactional outbox for services built on [`db`](../db/README.md) and
[`watermill`](../watermill/README.md): the message is written inside the
transaction that produced it, and delivered afterwards through a Watermill
router.

Two halves, neither useful alone:

- **`Publisher`** appends a message to the outbox table using the transaction
  the application already has open, so the message and the aggregate commit or
  roll back together.
- **`Relay`** reads the table and hands each message to a router, which brings
  retry, the poison queue, correlation ids, metrics and tracing with it.

What stays with the service is the **meaning**: topic names, the wire shape of
a domain event, and who subscribes to what. The SDK owns the mechanism, which
is the same everywhere.

## Guarantees

Read these before wiring anything. Each of them shows up as an incident when
assumed rather than read.

### NO ORDERING

**None.** Not global, not per aggregate, not per transaction.

The relay claims a batch with `FOR UPDATE SKIP LOCKED` and the router
dispatches that batch concurrently, one goroutine per message. Two messages
written by the same transaction can arrive in either order, and a message that
failed once arrives after messages written later.

A consumer that needs order has to carry it in the payload — a version, a
sequence number, a timestamp — and reject what it has already seen. There is no
option to turn ordering on: it would mean a single serialized reader, and the
`Relay` is designed to run in several replicas at once.

### At least once

A handler can see the same message twice. The relay marks a row delivered
*after* the handler returns, so a crash in between replays it. **Handlers must
be idempotent.**

### One cursor per topic

A delivered row is delivered for everyone. The relays of a topic are competing
consumers sharing one cursor, not independent readers: each message reaches
exactly one handler, which is what lets the relay scale horizontally.

Consequences worth stating plainly:

- A second consumer of the same events needs **its own topic**, or a broker
  downstream of the relay.
- A consumer deployed later sees **nothing written before it existed**. There is
  no replay.

### Retention

Delivered rows are removed once they are older than `WithRetention`, seven days
by default — long enough to answer "did we send it?" during an incident, short
enough that the outbox does not become the largest table in the database. The
reaper runs inside `Relay.Run`. `WithoutRetention()` turns it off for a service
that cleans the table itself.

## Usage

### Schema

The table is created by a migration like any other. Nothing in this package
creates tables at start-up behind the migration's back.

```go
import (
    "github.com/shortlink-org/go-sdk/db/drivers/postgres/migrate"
    "github.com/shortlink-org/go-sdk/outbox"
)

err := migrate.Migration(ctx, store, outbox.Migrations, "outbox")
```

One table, `outbox`, with a `topic` column — not one table per topic. A
per-topic name would have to be derived from a topic string, and a topic like
`auth.user` becomes the quoted identifier `"outbox_auth.user"`: something that
reads as schema-qualified and is not. There is no name to derive here.

### Writing

`Publisher` needs the transaction the service manages. `uow.FromContext`
satisfies the lookup, and it is the same hook `postgres.WithTxLookup` takes:

```go
publisher, err := outbox.NewPublisher(uow.FromContext)

// ...inside the transaction that changes the aggregate:
err = publisher.Publish(ctx, "orders", message.NewMessage(uuid.NewString(), payload))
```

Publishing without a transaction in the context returns `ErrNoTransaction`. It
is an error rather than a fallback to a pooled connection on purpose: a message
written outside the transaction survives the rollback of the aggregate that
produced it, and surfaces later as an event about something that never
happened. The usual cause is publishing after the commit rather than inside it.

### Delivering

The relay takes the router from `watermill.New`, so delivery inherits the
middleware the service already configured:

```go
client, err := watermill.New(ctx, log, cfg, backend, meter, tracer,
    watermill.WithPoisonQueue(client.Publisher, "orders.dlq"),
)

relay, err := outbox.NewRelay(store, log, client.Router)

err = relay.Handle("orders", func(ctx context.Context, msg *message.Message) error {
    return deliver(ctx, msg)
})

// Instead of client.Router.Run: the relay's handlers and the service's own
// live on the same router, and one Run drives both — plus the reaper.
err = relay.Run(ctx)
```

Returning an error from a handler nacks the message, which hands it to the
router's middleware: retry first, then the poison queue. **This depends on the
poison queue wrapping retry** — see [middleware order](../watermill/README.md#middleware-order).
Poison installed inside retry clears the error before retry can see it, and
every message that fails once lands in the DLQ.

A topic with no handler is never claimed. A message written by a newer build,
carrying a topic this build knows nothing about, waits for the consumer that
will be deployed later; it does not block the topics that do have handlers and
is not consumed by them.

## Configuration

| Option | Default | Description |
|--------|---------|-------------|
| `WithBatchSize` | `100` | rows claimed by one read; bounds both dispatch concurrency and how long the claiming transaction stays open |
| `WithPollInterval` | `500ms` | wait after a read that found nothing |
| `WithRetention` | `168h` | how long a delivered row is kept |
| `WithoutRetention` | — | keep delivered rows forever, stop the reaper |
| `WithReapInterval` | `1h` | how often the reaper runs |
| `WithReapBatchSize` | `1000` | rows removed by one reaper pass |

The claiming transaction is held open until the whole batch is acknowledged, so
`BatchSize × handler time` is roughly how long it lives. Retries happen inside
the handler, so a batch containing one message that exhausts its retries holds
the transaction for the length of that backoff.

## Why not watermill-sql

`watermill-sql` implements all of this. It also creates its own tables when the
subscriber starts, by its own names, outside the migrations. In a service where
every table belongs to an aggregate and is migrated, that is the single
exception, and it appears by magic.

So the table and the read loop are written here, and Watermill is used only for
delivery — the router, retry, poison, correlation id, metrics — where it gives
more than we would write.

## Schema

```sql
CREATE TABLE outbox (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid         TEXT        NOT NULL,
    topic        TEXT        NOT NULL,
    payload      BYTEA       NOT NULL,
    metadata     JSONB       NOT NULL DEFAULT '{}'::JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ
);
```

Two partial indexes: one over undelivered rows per topic for the relay, one
over delivered rows for the reaper. Both stay the size of the working set
rather than the size of the history.

## Related Packages

- **[`db`](../db/README.md)** — the store and `postgres.WithTxLookup`
- **[`watermill`](../watermill/README.md)** — the router and its middleware stack
- **[`uow`](../uow)** — the transaction in the context
