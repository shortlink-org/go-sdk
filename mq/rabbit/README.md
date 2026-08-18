## RabbitMQ Provider

This package provides a RabbitMQ implementation of the `mq.Provider` interface.

### Features

- ✅ Publish
- ✅ Subscribe
- ✅ Reconnect
- ✅ OpenTelemetry
- ✅ Logging

### Recovery

Reconnection is delegated to the automatic recovery built into `amqp091-go` (v1.12+).
After a connection loss the library reopens the connection and its channels,
re-declares the tracked topology (exchanges, queues, bindings) and re-subscribes
consumers onto the delivery channels they already handed out — `Subscribe` callers
keep receiving on the same channel and need no intervention.

Recovery is bounded: once `MQ_RECONNECT_MAX_RETRIES` attempts are exhausted the
connection is closed for good and `Check` reports the failure so the health probe
can restart the service. `Check` also reports an error while a recovery is in
flight, since publishing fails until it completes.

### Configuration

| Variable                     | Default                   | Description                                                        |
|------------------------------|---------------------------|--------------------------------------------------------------------|
| `MQ_RABBIT_URI`              | `amqp://localhost:5672`   | RabbitMQ URI                                                        |
| `MQ_RECONNECT_DELAY_SECONDS` | `3`                       | Delay between recovery attempts                                     |
| `MQ_RECONNECT_MAX_RETRIES`   | `60`                      | Recovery attempts before giving up; `0` disables recovery entirely  |
