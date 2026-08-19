# grpcmw

Carries a database read-your-writes guarantee across a gRPC hop.

Without it, a service that writes and then calls another service hands over
nothing, and the callee's reads may be served by a replica that has not replayed
those writes. A `context.Context` does not cross a wire, so the guarantee
travels as metadata under `wal-watermark`.

```go
router, _ := postgres.RouterFrom(store)

// client
grpc.NewClient(target,
	grpc.WithChainUnaryInterceptor(grpcmw.UnaryClientInterceptor(router.Port())),
	grpc.WithChainStreamInterceptor(grpcmw.StreamClientInterceptor(router.Port())),
)

// server
grpc.NewServer(
	grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(router.Port())),
	grpc.ChainStreamInterceptor(grpcmw.StreamServerInterceptor(router.Port())),
)
```

## Both directions, and they are different cases

**Forward** — the caller wrote, then called. Its watermark rides out in
metadata, and the callee's reads are gated on it: a replica once it has replayed
past that position, the primary until then.

**Return** — the callee wrote on the caller's behalf. Its watermark comes back
in the trailer, and the caller's own later reads are gated on it. This has no
equivalent in the HTTP or queue paths, and it is why the interface has an
`Observe` that the others do not: by the time a reply arrives, the caller
already holds its context, so the guarantee has to be raised in place rather
than wrapped around.

The trailer is read even when the call returned an error — an RPC can fail after
part of its work has committed, and forgetting that write is precisely what this
exists to prevent.

## Notes

- A request arriving without a watermark is still scoped, so its reads may use a
  replica. Unscoped, they all default to the primary.
- Health and reflection RPCs are skipped.
- A `nil` `Consistency` makes every interceptor a transparent pass-through, so
  wiring it into a service with no replicas costs nothing.
- A failure to read the watermark never fails the call. Trading a consistency
  optimisation for availability is the wrong way round; the callee simply reads
  from the primary.
- Streaming clients send the watermark but do not read the trailer back: a
  stream's trailer only exists once the caller's own loop is done. For a
  streaming RPC that writes on the caller's behalf, return the watermark in a
  response message and pass it to `Observe` yourself.

It sits with the driver, next to `httpmw`, because carrying this guarantee
across a hop is the driver's concern whichever transport speaks it. It does not
import `replica`: `Consistency` is declared here and satisfied by
`*postgres.TextPort`, so the interceptors never touch the router's types.

See [ADR 0001 — Read-replica routing](../../ADR/0001-read-replica-routing.md).
