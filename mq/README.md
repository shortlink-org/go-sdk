## MQ Adapter

This package provides a simple abstraction for a message queue.

> **Deprecated.** Use `github.com/shortlink-org/go-sdk/watermill` instead. It takes
> the transport as a `Backend` interface, so no driver is linked unless you build one.

### Selecting a driver

A driver is selected at runtime through `MQ_TYPE`, and is made available at
compile time by importing its package for the side effect:

```go
import (
    "github.com/shortlink-org/go-sdk/mq"
    _ "github.com/shortlink-org/go-sdk/mq/rabbit"
)

bus, err := mq.New(ctx, log, cfg)
if errors.Is(err, mq.ErrDisabled) {
    return nil // MQ_ENABLED is false
}
```

Only the drivers you import are linked. Importing all four costs ~33 MB of
binary; a bus with RabbitMQ alone is ~10 MB.

An unrecognised `MQ_TYPE` fails `mq.New` with `UnknownMQTypeError` and lists
the registered drivers — it does **not** fall back to another transport.
`MQ_ENABLED=false` reports `ErrDisabled` rather than returning a nil bus.

A driver that cannot register — a duplicate name or a nil factory — is recorded
and reported by `mq.New` as `RegisterError`. Registration happens in `init`,
where there is no caller to return an error to, so nothing panics: the failure
waits until there is someone to hand it to.

### Support MQ

- [x] RabbitMQ
- [x] Kafka/RedPanda
- [x] NATS
- [x] Redis
- [ ] NSQ
- [ ] Apache Pulsar
- [ ] AWS SQS
