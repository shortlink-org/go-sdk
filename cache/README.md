# Cache

A small byte-level cache port and its Redis adapter.

The package exposes an **interface**, not a constructor around somebody else's
client. A consumer binds `cache.Cache` in its container, fakes it in a test, and
swaps in `cache.Noop` where a deployment runs without Redis — without importing
a Redis library of its own.

```go
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)          // ErrMiss on a miss
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}
```

## Getting started

```go
import "github.com/shortlink-org/go-sdk/cache"

client, err := cache.NewRedis(ctx, cfg)
if err != nil {
	return err
}
defer client.Close()
```

`NewRedis` opens the connection through `go-sdk/db/drivers/redis`, so the
address comes from the same variables every other store in the SDK uses.

The connection belongs to the returned `*Redis`: close it alongside the other
connections the service opened, rather than waiting for the context to end.

## Values are bytes

There is no reflective `Item` API and no imposed codec. What a value looks like
on the wire is the business of whoever stored it:

```go
err := client.Set(ctx, "session:"+token, payload, 15*time.Minute)

payload, err := client.Get(ctx, "session:"+token)
switch {
case errors.Is(err, cache.ErrMiss):
	// not cached — go to the source of truth
case err != nil:
	return err
}
```

A miss is `cache.ErrMiss`, not an empty value and not a failure. A stored empty
slice stays distinguishable from an absent key.

`Set` with a `ttl` of zero or less stores nothing: an entry without an expiry is
not expressible through this port, so no caller can leave one behind.

`Delete` accepts several keys and ignores the ones that are not there — the
write path usually has no idea whether anybody read what it just changed.

### Optional JSON sugar

If a consumer does want JSON, it is offered as free functions over the port
rather than as methods on it, built on `encoding/json/v2`. The port stays
usable, and fakeable, without them:

```go
err := cache.SetJSON(ctx, client, key, session, time.Minute)

session, err := cache.GetJSON[Session](ctx, client, key)
```

## No cache

```go
var client cache.Cache = cache.Noop{}
```

`Noop` always misses; `Set` and `Delete` do nothing. It is how "this deployment
has no cache" is expressed, so a consumer keeps one code path instead of testing
the cache for nil at every call site.

## Instrumentation is optional

Nothing is instrumented by default, and a nil provider is the same as not
passing the option — a service that exports nothing neither pays for the hooks
nor has to depend on `go-sdk/observability` in order to start:

```go
client, err := cache.NewRedis(ctx, cfg,
	cache.WithTracer(tracerProvider),
	cache.WithMeter(meterProvider),
)
```

## Client-side caching

`WithClientSideCache` keeps a local copy of every value read, for at most the
given TTL. It is **off by default**:

```go
client, err := cache.NewRedis(ctx, cfg, cache.WithClientSideCache(time.Minute))
```

The local layer is rueidis `DoCache` — client-side caching that Redis itself
invalidates. Redis tracks which keys the connection has cached and pushes an
invalidation when one of them changes, so a `Delete` on any replica drops the
copy held by **all** of them.

This is the reason there is no in-process LRU here under any flag. Nothing
invalidates a TinyLFU: every replica except the one that called `Delete` keeps
serving the stale value until its own local TTL runs out. For a catalog that is
tolerable; for revoking a session it is a hole.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `STORE_REDIS_URI` | `localhost:6379` | Redis hosts, read by the `db` redis driver |
| `STORE_REDIS_USERNAME` | — | Redis username |
| `STORE_REDIS_PASSWORD` | — | Redis password |
| `STORE_REDIS_CLIENT_CACHE_TTL` | `0` | Client-side cache window; `0` leaves the local layer off |

`WithClientSideCache` overrides `STORE_REDIS_CLIENT_CACHE_TTL`.

## Tests

The tests run against a Redis container through `testcontainers`, and skip
rather than fail where no Docker daemon is reachable:

```bash
go test -tags unit ./...
```
