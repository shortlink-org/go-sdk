### event-sourcing

Event sourcing for Go, with PostgreSQL as the store.

- A generic repository for aggregates
- Atomic append with optimistic concurrency
- Snapshots with a policy and schema versions
- Event metadata and upcasters
- A global stream for projections
- OpenTelemetry traces and metrics, declared in
  [`semconv/registry/eventsourcing.yaml`](../semconv/registry/eventsourcing.yaml)

The root package is the domain and its ports (`Store`, `GlobalReader`,
`SnapshotScheduler`). [`store/postgres`](./store/postgres) implements them.

There is no event bus: publish through a transactional outbox, as below.

##### Aggregates

An aggregate embeds `Root` and defines `Apply`, which runs both when an event is raised
and when it is replayed:

```go
type Order struct {
	eventsourcing.Root

	total int
}

func (o *Order) Apply(event *eventsourcing.Event) error {
	switch event.Type {
	case "item_added":
		var item ItemAdded
		if err := json.Unmarshal(event.Payload, &item); err != nil {
			return err
		}

		o.total += item.Price
	}

	return nil
}

func (o *Order) AddItem(price int) error {
	event, err := eventsourcing.NewEvent("item_added", ItemAdded{Price: price})
	if err != nil {
		return err
	}

	return eventsourcing.Raise(o, event)
}
```

##### Repository

```go
store, err := postgres.New(ctx, db)
if err != nil {
	return err
}

orders, err := eventsourcing.NewRepository(store, "order", func() *Order { return &Order{} })
if err != nil {
	return err
}

order, err := orders.Load(ctx, id) // or orders.New() for a new one
if err != nil {
	return err
}

if err := order.AddItem(100); err != nil {
	return err
}

err = orders.Save(ctx, order)
if errors.Is(err, eventsourcing.ErrConcurrencyConflict) {
	// load again and retry the command
}
```

`Load` returns `ErrAggregateNotFound` when the aggregate does not exist.

##### Appending events

`Store.Append` is the stream API under the repository. It writes all its events or none,
as the versions after `expectedVersion` (0 for a new aggregate):

```go
err := store.Append(ctx, id, 4, events)
if errors.Is(err, eventsourcing.ErrConcurrencyConflict) {
	// the aggregate is not at version 4: reload and retry
}
```

Aggregate ids and event ids must be UUIDs. The store fills in `ID` (UUIDv7), `CreatedAt`
and `SchemaVersion` (1) when they are not set; events are ordered by `Version`.

##### Metadata and upcasting

An event carries `CorrelationID`, `CausationID`, `SchemaVersion` and free-form `Metadata`,
where the store adds the W3C `traceparent` of the write when there is a span. Old events
are never rewritten; bring them up to date on load:

```go
upcast := func(event *eventsourcing.Event) error {
	if event.Type == "item_added" && event.SchemaVersion == 1 {
		// rewrite event.Payload to the current schema
	}

	return nil
}

orders, err := eventsourcing.NewRepository(store, "order", newOrder, eventsourcing.WithUpcasters(upcast))
```

##### Transactions

When `ctx` carries a transaction (`uow.WithTx`), `Append`, `ReadStream`, `LoadSnapshot` and
`SaveSnapshot` run in it, and committing is up to the caller. This is how events and an
[`outbox`](../outbox) message are written atomically. A conflict aborts that transaction,
so roll it back before retrying.

##### Snapshots

An aggregate implementing `Snapshotter` is restored from its snapshot. There are two ways
to take them:

- `WithSnapshotEvery(n)` makes `Repository.Save` take one whenever the version crosses a
  multiple of `n`. It never fails the save, and is skipped inside a transaction.
- A background job calls `AggregatesDueForSnapshot` and `Repository.Snapshot` for each
  aggregate it returns.

`SnapshotSchemaVersion` versions the format. `Load` ignores a snapshot of another version
or one that fails to restore, and replays the events. `SaveSnapshot` keeps the latest
schema version, and then the latest aggregate version.

##### Projections

`ReadAll` reads the events of all aggregates in a stable order. Persist the `Cursor` of the
last event handled:

```go
events, err := store.ReadAll(ctx, cursor, 500)
if err != nil {
	return err
}

for _, event := range events {
	// handle the event, then persist event.Cursor
}
```

Sequence values are taken before commit, so reading by position alone would skip a slow
transaction. The cursor is a pair of transaction id and position, and an event becomes
visible once every older transaction has ended: nothing lands behind the cursor, and one
long-running transaction anywhere in the database delays the stream. Delivery is
at-least-once when the cursor is persisted after handling.

##### Migrations

`postgres.New` applies the schema, in the `eventsourcing` database schema. With
`WithoutMigrations` it does not, for applications that run without DDL privileges and apply
`postgres.Migrations()` themselves. PostgreSQL 13 or newer is required.

##### Tests

The integration tests start PostgreSQL in a container:

```bash
make test MODULE=eventsourcing TAGS=unit,database
```

##### References

+ [Pattern: Event sourcing](https://microservices.io/patterns/data/event-sourcing.html)
+ [Go - CQRS / Event Sourcing made easy - Go](https://github.com/mishudark/eventhus)
+ [Examples and Tutorials of Event Sourcing in .NET](https://github.com/oskardudycz/EventSourcing.NetCore)
