## Observability

This package provides observability primitives for the application:

- `monitoring` - provides a Prometheus metrics registry and a HTTP handler for exposing metrics
- `tracing` - provides a Tracing provider for OpenTelemetry (see below)
- `logging` - provides a structured logger

### Tracing

`tracing.New` builds an OTLP/gRPC exporter and installs it as the global
OpenTelemetry provider together with the W3C TraceContext, Baggage and B3
propagators. Tracing is optional: **an empty `TRACER_URI` turns it off**.
`New` then installs nothing, leaves the propagators alone and returns a nil
provider with a no-op cleanup, at no cost (no exporter, no retries, nothing
through the otel error handler).

| Variable                   | Default | Description                                                                              |
|----------------------------|---------|------------------------------------------------------------------------------------------|
| `TRACER_URI`               | *(empty)* | Collector address (`host:port`) for OTLP over gRPC. Empty disables tracing.            |
| `SERVICE_NAME`             |         | `service.name` resource attribute.                                                       |
| `SERVICE_VERSION`          |         | `service.version` resource attribute.                                                    |
| `TRACING_INITIAL_INTERVAL` | `2s`    | First retry delay for a failed export; also the batch timeout.                           |
| `TRACING_MAX_INTERVAL`     | `30s`   | Upper bound of the retry delay.                                                          |
| `TRACING_MAX_ELAPSED_TIME` | `1m`    | How long a single export keeps retrying before it is dropped.                            |
| `TRACING_SHUTDOWN_TIMEOUT` | `10s`   | Budget for the cleanup returned by `New` to flush pending spans and stop the exporter.   |

The cleanup runs on a fresh context bounded by `TRACING_SHUTDOWN_TIMEOUT`, not on
the context `New` was started with, and is safe to call more than once. The
timeout is deliberately not tied to `TRACING_MAX_ELAPSED_TIME`: a service told
to stop should stop, not wait a minute on an unreachable collector.

### References

- [uptrace](https://uptrace.dev/opentelemetry/) - more articles and tips
