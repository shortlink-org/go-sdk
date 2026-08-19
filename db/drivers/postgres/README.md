# postgres

PostgreSQL driver for `db`, built on [pgx](https://github.com/jackc/pgx) v5
with `pgxpool` and OpenTelemetry tracing via
[otelpgx](https://github.com/exaring/otelpgx).

```go
import (
	"github.com/shortlink-org/go-sdk/db"
	_ "github.com/shortlink-org/go-sdk/db/drivers/postgres"
)

store, err := db.New(ctx, log, tracer, metrics, cfg)
pool, err := db.Conn[*pgxpool.Pool](store)
```

`STORE_TYPE=postgres` selects the driver; `STORE_POSTGRES_URI` is the DSN.

## Layout

```
postgres/            the driver: Store, Init, options, RouterFrom
  replica/           the router: routing decision, health gate, poller, watermarks
    wal/             WAL positions and cross-boundary tokens
    sqlclass/        statement classification
    metrics/         OpenTelemetry instruments
  middlewares/httpmw/  HTTP boundary middleware
  middlewares/grpcmw/  gRPC boundary interceptors
  ops/               Grafana dashboard, Prometheus Operator rules
  ADR/               architecture decision record
```

The dependency runs one way, `postgres → replica → {wal, sqlclass, metrics}`,
and the three leaves depend on nothing of ours. `metrics` in particular takes
strings and numbers rather than the router's own types, which is what keeps it
from having to know about routing at all.

Both boundary middlewares live here, under `middlewares/`, because carrying
this guarantee across a hop is the driver's concern whichever transport speaks
it. Neither imports `replica`: each declares the small interface it needs and
takes `router.Port()`, which is what keeps the transports out of the router's
types.

## Architecture decisions

- [ADR 0001 — Read-replica routing](./ADR/0001-read-replica-routing.md)

## Read replicas

Set `STORE_POSTGRES_REPLICA_URI` and reads move to a standby, without giving up
read-your-writes. Unset, nothing changes: no second pool, no poller, no
behavioural difference.

```go
router, err := postgres.RouterFrom(store)

// Reads route to a replica; writes go to the primary and pin the rest of
// this context there.
rows, err := router.Query(ctx, `SELECT ...`)

// Explicitly accept lag, or explicitly refuse it.
rows, err := router.Query(replica.Stale(ctx), `SELECT ...`)
rows, err := router.Query(replica.OnPrimary(ctx), `SELECT ...`)
```

See the ADR for the design, the failure modes, and what this deliberately does
not cover.

### Configuration

| key | default | meaning |
|---|---|---|
| `STORE_POSTGRES_URI` | `postgres://postgres:shortlink@localhost:5432/shortlink?sslmode=disable` | primary DSN |
| `STORE_POSTGRES_REPLICA_URI` | `""` | replica DSNs, comma separated. Empty disables routing |
| `STORE_POSTGRES_REPLICA_POLL_INTERVAL` | `250ms` | how often replicas are probed |
| `STORE_POSTGRES_REPLICA_POLL_JITTER` | `0.15` | fraction of the interval, so a fleet does not probe in lockstep |
| `STORE_POSTGRES_REPLICA_PROBE_TIMEOUT` | `500ms` | per-probe timeout |
| `STORE_POSTGRES_REPLICA_SAMPLE_STALE_AFTER` | `2s` | age past which a sample is not trusted |
| `STORE_POSTGRES_REPLICA_MAX_LAG_BYTES` | `8388608` | staleness budget for reads with no watermark |
| `STORE_POSTGRES_REPLICA_NO_TRACKER_POLICY` | `primary` | where an unscoped read goes |
| `STORE_POSTGRES_GATE_MAX_WAIT` | `250ms` | inline wait for a replica to catch up |

> **One pool must mean one endpoint.** A replica DSN pointing at a load
> balancer in front of several standbys breaks the guarantee: the probe
> describes one node and the read reaches another. Run one pool per standby.

### With DDD and CQRS

The router implements the same surface as `*pgxpool.Pool`, so it stays in the
infrastructure layer: one field in a repository adapter changes type and the
domain never sees it. Under CQRS the tracker belongs on the bus, which already
knows which side of the split it is on:

```go
// command bus — the write taints the context by itself
ctx = replica.WithTracker(ctx)

// query bus — a read model tolerates lag by definition
ctx = replica.Stale(ctx)
```

Two rules do not follow from the code, and the ADR explains why both are
load-bearing: [Fit with DDD and
CQRS](./ADR/0001-read-replica-routing.md#fit-with-ddd-and-cqrs), which also has
the table of what belongs on which pool.

Pin the write side to the primary — aggregate rehydration looks exactly like an
ordinary read, and the classifier cannot catch it:

```go
func (r *AggregateRepo) Load(ctx context.Context, id uuid.UUID) (*Order, error) {
	return r.load(replica.OnPrimary(ctx), id)
}
```

Wire the unit of work, so a repository called inside a transaction runs on that
transaction's connection:

```go
store, err := db.New(ctx, log, tracer, metrics, cfg,
	postgres.With(postgres.WithTxLookup(uow.FromContext)),
)
```

### Behind a load-balanced replica endpoint

A Kubernetes `Service` in front of several standbys — Crunchy PGO's
`<cluster>-replicas`, or any DNS round-robin — breaks the token path and only
the token path. kube-proxy balances per TCP connection, so the health probe and
the read it authorises can land on different pods, and the gate then reports
"caught up" about a node that did not serve the read.

What still holds, because it never consults the gate:

| mechanism | behind a multi-pod Service |
|---|---|
| taint — a read after a write in the same request | works; goes to the primary |
| stale reads within a lag budget | approximate; the sample may come from another pod |
| a token carried across a boundary | **not sound** |

So the working configuration is the carriers off, the scoping on:

```go
r.Use(httpmw.New(httpmw.Config{Router: router, Header: new(false)}))
```

That still scopes every request — without it, reads carry no tracker and the
default policy sends all of them to the primary, which is the whole benefit
gone. It just does not mint or trust tokens. The offload figures above were
measured in exactly this mode: the load harness used the taint and nothing else.

To get the token path as well, one pool must mean one endpoint: either run a
single replica, in which case the Service resolves to it and everything works,
or give each standby its own DSN. Pod-level discovery that survives a rollout is
not implemented.

Two more notes for Crunchy PGO specifically. Point the primary DSN at
`<cluster>-primary`: it follows failover, Patroni bumps the timeline, and the
poller notices and discards tokens from the previous one. And
`pg_control_system()` needs a superuser, which the application role is not — so
the system identifier stays unknown and lineage checks fall back to the
timeline, which still catches every failover.

### Behind a pooler

PgBouncer — including Crunchy PGO's `spec.proxy.pgBouncer` — does not parse
SQL and does not balance, so it pools without competing with the `Router`. Two
things to know. (A proxy that routes as well as pools, such as pgpool-II, is a
second router for the same decision and is not supported; see the ADR.)

PGO's generated `[databases]` section is a single wildcard pointing at
`<cluster>-primary`, so its PgBouncer fronts the primary only. The replica DSN
does not go through it — either add a `[databases]` entry for the standby or
point `STORE_POSTGRES_REPLICA_URI` at the standby directly, subject to the
one-pool-one-endpoint rule above.

PGO sets no `pool_mode`, so PgBouncer's default of `session` applies and
nothing here changes. Under `transaction` pooling it does: pgx's prepared
statement cache needs PgBouncer 1.21+ with `max_prepared_statements` above
zero, and the session-scoped statements the classifier routes to the primary
(`SET`, `PREPARE`, `DISCARD`) stop being reliable — a property of transaction
pooling, not of routing.

### Transactions

`Begin` and `BeginTx` return an ordinary `pgx.Tx` and behave as you would
expect: a read-write transaction goes to the primary and taints the context, a
`pgx.ReadOnly` one may go to a replica.

`InTx` owns the whole lifecycle instead — rollback on error, rollback on panic,
exactly one release — and earns its place when `WithSyncWatermark` is on:

```go
err := router.InTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `INSERT INTO orders (...) VALUES (...)`)

	return err
})
```

The WAL position has to be read **after** the commit — read inside the
transaction it excludes the commit record, and the gate then opens exactly one
WAL record too early. Through `pgx.Tx` that is necessarily a second round trip,
because `Commit` sends `COMMIT` by itself. `InTx` pipelines the two:

```sql
commit; select pg_current_wal_insert_lsn()::text
```

The tx handed to `fn` is valid only for that call — do not retain it, and do not
commit or roll it back yourself.

### Carrying the guarantee across a boundary

A `context.Context` dies with the request, so `POST /orders` followed by
`GET /orders` loses the guarantee exactly where a user notices. The boundary
middlewares carry it as a token instead:

```go
// HTTP: stamps X-Read-LSN on mutating responses, gates the requests that follow
r.Use(httpmw.New(httpmw.Config{Router: router}))

// Queue: stamps the token into message metadata, and the consumer waits for it
client.Publisher = watermill.NewConsistencyPublisher(client.Publisher, router.Port(), log, meter)
client.Router.AddMiddleware(watermill.NewConsistencyMiddleware(router.Port(), watermill.ConsistencyOptions{}, meter))

// gRPC: both directions — the caller's watermark rides out in metadata, the
// callee's comes back in the trailer
grpc.WithChainUnaryInterceptor(grpcmw.UnaryClientInterceptor(router.Port()))
grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(router.Port()))
```

All three are optional. Without them the routing still works — reads that never
write still reach a replica — but a read following a write stays on the primary.

They share one adapter, `router.Port()`, and none of them imports `replica`.
Each declares the small interface it needs — a method or two taking strings —
and Go's structural typing does the rest, so a transport never reaches into the
router's types and the `watermill` module never imports `db` at all.

## What it buys

Not latency — a replica read is not faster, and crosses one more hop. What it
buys is capacity on the primary: at a fixed offered load and a 90% read mix the
primary served 75.7% fewer transactions, with read latency unchanged. The
measurements, the arithmetic ceiling on that number, and what it does *not*
claim are in [the ADR](./ADR/0001-read-replica-routing.md#measured-outcome).

Whether there is anything to move is a question about your primary, not about
this driver:

```sql
SELECT calls, total_exec_time, query
FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 20;
```

If `SELECT`s without `FOR UPDATE` sit at the top, there is. At 20% CPU on the
primary you are buying headroom for a spike, not performance today.

Once it is running, the share that actually left the primary is a metric:

```promql
sum(rate(postgres_route_decisions_total{target="replica"}[5m]))
  / sum(rate(postgres_route_decisions_total[5m]))
```

and `postgres_route_fallbacks_total{reason="no_tracker"}` climbing means the
feature is enabled but not wired — reads are not being scoped, so they default
to the primary.

There is a dashboard in [ops/grafana/](./ops/grafana/) and Prometheus Operator
rules in [ops/prometheus/](./ops/prometheus/).

The routing decision itself costs 14–31 ns with no allocation, against a 255 µs
round trip — the full benchmark table is in the ADR. To rerun it:

```bash
go test -tags unit -run XXX -bench . ./drivers/postgres/
```

## Migrations

```go
import "github.com/shortlink-org/go-sdk/db/drivers/postgres/migrate"

err := migrate.Migration(ctx, store, migrationsFS, "my-service")
```

Each service gets its own `schema_migrations_<name>` table, so several services
can share a database without colliding.

## Tests

```bash
go test -mod=readonly -race -tags unit ./drivers/postgres/...
```

Integration against a real server, and against a real streaming replica:

```bash
go test -mod=readonly -tags "database postgres" ./drivers/postgres/...
go test -mod=readonly -tags "database postgres replica" ./drivers/postgres/...
```

The replica suite covers routing against real streaming replication, the
degradation paths (standby stopped, standby promoted, a standby delayed by
`recovery_min_apply_delay` and refused at startup), the batched commit, and the
load comparison above. It builds a primary and a `pg_basebackup` standby on
a private network, so it needs Docker and takes about a minute.
