### event-sourcing

This is a simple event sourcing library for Go.

### Feature

- Atomic append with optimistic concurrency
- Snapshots
- OpenTelemetry traces and metrics, declared in
  [`semconv/registry/eventsourcing.yaml`](../semconv/registry/eventsourcing.yaml)
- Store engine
  - PostgreSQL

There is no event bus: publish through a transactional outbox, as below.

##### Saving events

`Save` writes all its events or none of them. When the aggregate moved past the
version the events were built on, the error matches `ErrConcurrencyConflict`:
reload the aggregate and retry the command.

```go
err := store.Save(ctx, aggregate.Uncommitted())
if errors.Is(err, eventsourcing.ErrConcurrencyConflict) {
	// reload and retry
}
```

Aggregate ids and event ids must be UUIDs. `Save` fills in `Id` (UUIDv7) and
`CreatedAt` when they are not set. Timestamps are stored in UTC with a precision
of one second; events are ordered by `version`.

##### Transactions

When `ctx` carries a transaction (`uow.WithTx`), `Save`, `Load` and `SaveSnapshot`
run in it, and committing is up to the caller. This is how events and an
[`outbox`](../outbox) message are written atomically. A conflict aborts that
transaction, so roll it back before retrying.

##### Snapshots

`GetAggregateWithoutSnapshot` returns the next batch of aggregates that are due
for a snapshot. Tune it with `WithSnapshotBatchSize` and `WithSnapshotThreshold`.
`SaveSnapshot` ignores a snapshot older than the stored one.

##### Migrations

`New` applies the schema. With `WithoutMigrations` it does not, for applications
that run without DDL privileges and apply `es_postgres.Migrations()` themselves.

##### Tests

The integration tests start PostgreSQL in a container:

```bash
make test MODULE=eventsourcing TAGS=unit,database
```

##### Tracing

![Tracing](./docs/opentracing.png)

##### References

+ [Pattern: Event sourcing](https://microservices.io/patterns/data/event-sourcing.html)
+ [Go - CQRS / Event Sourcing made easy - Go](https://github.com/mishudark/eventhus)
+ [Examples and Tutorials of Event Sourcing in .NET](https://github.com/oskardudycz/EventSourcing.NetCore)
