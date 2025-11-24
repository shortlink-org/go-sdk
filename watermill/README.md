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
