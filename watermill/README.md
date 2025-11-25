# Watermill SDK wrapper

A lightweight wrapper around [ThreeDotsLabs/watermill](https://watermill.io) that integrates observability and is ready for production use in Shortlink services.

## Features

- **Ready-to-use `Client`**: internally sets up `message.Router`, configures logger, global middleware (panic/retry/correlation), metrics, and OTEL tracing.
- **Metrics + exemplars**: publish/consume counters and histograms with automatic `topic`, `trace_id`, `span_id` attributes.
- **Tracing**: middleware extracts context from Watermill metadata, creates `watermill.consume` span and propagates context. On publish, creates `watermill.publish` span and writes TraceID/SpanID to message metadata.
- **DLQ**: optional configuration sends messages after N errors to `<topic>.DLQ` with `DLQMessage` body (payload + metadata + error text + retry counter).
- **Kafka backend**: `backends/kafka` contains a slight-fork wrapper of Watermill Kafka (publisher/subscriber + OTEL tracer). RabbitMQ is not yet implemented (stub).

## Installation

Add the module as a dependency:

```bash
go get github.com/shortlink-org/go-sdk/watermill
```

## Quick Start

```go
ctx := context.Background()
cfg := config.New()                      // github.com/shortlink-org/go-sdk/config
log := logger.New(ctx)                   // github.com/shortlink-org/go-sdk/logger
meter := monitoring.Metrics             // see observability/metrics
tracer := tracing.TracerProvider        // see observability/tracing

backend, _ := kafka.NewSubscriber(...)
client, err := watermill.New(ctx, log, cfg, backend, meter, tracer)
if err != nil {
    log.Fatal(err)
}

handler := client.Router.AddHandler(
    "example",
    "input.topic",
    client.Subscriber,
    "output.topic",
    client.Publisher,
    func(msg *message.Message) ([]*message.Message, error) {
        // business logic
        return nil, nil
    },
)

go client.Router.Run(ctx)
defer client.Close()
```

## Dependency Injection (google/wire)

`watermill.New` and `backends/kafka.New` are ready-to-use provider functions, so they can be plugged straight into `google/wire` graphs. Если сервису нужны только публикация или только потребление, используйте `kafka.NewPublisherFromConfig` или `kafka.NewSubscriberFromConfig`.

```go
var WatermillSet = wire.NewSet(
    config.New,          // returns *config.Config
    logger.New,          // returns logger.Logger
    kafka.New,           // github.com/shortlink-org/go-sdk/watermill/backends/kafka
    wire.Bind(new(watermill.Backend), new(*kafka.Backend)),
    watermill.New,
)

var KafkaPublisherSet = wire.NewSet(
    config.New,
    logger.New,
    kafka.NewPublisherFromConfig,
)

var KafkaSubscriberSet = wire.NewSet(
    config.New,
    logger.New,
    kafka.NewSubscriberFromConfig,
)
```

`kafka.New` configures Sarama clients from `config.Config`, enabling OTEL, retries, idempotent producer and other production defaults. Override any key via env vars or `.env` to adapt behaviour per service.

## Configuration

Values are read from `github.com/shortlink-org/go-sdk/config.Config` (Viper). Important keys:

| Key | Default | Description |
|-----|---------|-------------|
| `WATERMILL_RETRY_MAX_RETRIES` | `3` | number of retries in retry middleware |
| `WATERMILL_RETRY_INITIAL_INTERVAL` | `150ms` | initial retry interval |
| `WATERMILL_RETRY_MAX_INTERVAL` | `2s` | maximum interval between retries |
| `WATERMILL_RETRY_MULTIPLIER` | `2.0` | exponential backoff multiplier |
| `WATERMILL_DLQ_ENABLED` | `false` | enable DLQ middleware |
| `WATERMILL_DLQ_MAX_RETRIES` | `5` | number of errors allowed before sending to `<topic>.DLQ` |

### Kafka backend configuration

| Key | Default | Description |
|-----|---------|-------------|
| `WATERMILL_KAFKA_BROKERS` | `localhost:9092` | comma-separated broker list or string slice |
| `WATERMILL_KAFKA_CONSUMER_GROUP` | `SERVICE_NAME` | consumer group name |
| `WATERMILL_KAFKA_CONSUMER_INITIAL_OFFSET` | `latest` | `latest` or `oldest` subscription offset |
| `WATERMILL_KAFKA_REBALANCE_STRATEGY` | `range` | `range`, `roundrobin`, or `sticky` |
| `WATERMILL_KAFKA_SUBSCRIBER_NACK_SLEEP` | `100ms` | delay before redelivering Nacked messages |
| `WATERMILL_KAFKA_SUBSCRIBER_RECONNECT_SLEEP` | `1s` | delay before retrying failed connections |
| `WATERMILL_KAFKA_WAIT_FOR_TOPIC_TIMEOUT` | `10s` | wait timeout when creating topics |
| `WATERMILL_KAFKA_SKIP_TOPIC_INIT` | `false` | do not wait for topic readiness after creation |
| `WATERMILL_KAFKA_OTEL_ENABLED` | `true` | wrap publisher/subscriber with OTEL instrumentation |
| `WATERMILL_KAFKA_SARAMA_VERSION` | `max` | Sarama protocol version (`max`, `default`, or concrete) |
| `WATERMILL_KAFKA_PRODUCER_RETRY_MAX` | `10` | max producer retries before failing publish |
| `WATERMILL_KAFKA_PRODUCER_COMPRESSION` | `snappy` | compression codec (`none`, `gzip`, `lz4`, `snappy`, `zstd`) |
| `WATERMILL_KAFKA_PRODUCER_IDEMPOTENT` | `true` | enable idempotent producer with `max.in.flight=1` |
| `WATERMILL_KAFKA_CLIENT_ID` | `SERVICE_NAME` | Sarama client ID used for producer and consumer |

## DLQ Message

After exceeding `maxRetries`, a JSON message is sent to `<topic>.DLQ`:

```json
{
  "topic": "my-topic",
  "payload": "... base64 ...",
  "metadata": {"received_topic": "my-topic", "watermill_dlq_retry_count": "5"},
  "error": "handler error string",
  "retry_count": 5,
  "original_uuid": "<source message uuid>"
}
```

TraceID/SpanID are preserved in metadata, so downstream consumers can continue the trace.

## Observability

- **Metrics** — published via the provided `metric.MeterProvider`. Names:
  - `watermill_messages_published_total`
  - `watermill_messages_consumed_total`
  - `watermill_messages_failed_total`
  - `watermill_publish_latency_seconds`
  - `watermill_consume_latency_seconds`
  All metrics have `topic`, `trace_id`, `span_id` attributes. Errors are additionally tagged with `stage=publish|consume` and `error` (truncated to 128 characters).

- **Tracing** — requires `trace.TracerProvider`. Middleware automatically extracts/injects context in Watermill metadata (`otel_trace_id`, `otel_span_id`).

## Kafka Backend

The `backends/kafka` directory contains a driver copied from Watermill with several improvements:

- OTEL support via `otelsarama`
- helper functions for contexts (partition/offset/timestamp)

Usage is similar to upstream Watermill. See tests in `backends/kafka/pubsub_test.go` and configuration via `SubscriberConfig`/`PublisherConfig`.

## Limitations

- RabbitMQ backend is not yet implemented (file `backends/rabbit/rabbit.go`).
- Limited automated tests in the package (use Watermill upstream tests for validation). Before release, run `go test ./watermill/...`.
