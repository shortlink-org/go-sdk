# Temporal

Dependency injection package for [Temporal](https://temporal.io/) client using the official
[Temporal Go SDK](https://github.com/temporalio/sdk-go).

Integrates with `go-sdk/grpc` for consistent gRPC configuration and `go-sdk/observability` 
for unified OpenTelemetry metrics and tracing.

## Features

Based on [Temporal Go SDK Observability Guide](https://docs.temporal.io/develop/go/observability):

- **Tracing**: OpenTelemetry tracing for workflows, activities, and child workflows
- **Metrics**: Temporal SDK metrics via OpenTelemetry (workflow tasks, activities, polls)
- **Logging**: Logger adapter for go-sdk logger
- **gRPC**: Integration with go-sdk/grpc (auth forwarding, TLS, timeouts)

## Installation

```bash
go get github.com/shortlink-org/go-sdk/temporal
```

## Usage

### With Wire (DI)

```go
package di

import (
    "github.com/google/wire"
    "github.com/shortlink-org/go-sdk/temporal"
)

var Set = wire.NewSet(
    // ... other providers (logger, config, tracer, monitoring)
    temporal.New,
)
```

### Standalone

```go
package main

import (
    "context"
    "github.com/shortlink-org/go-sdk/config"
    "github.com/shortlink-org/go-sdk/logger"
    "github.com/shortlink-org/go-sdk/observability/metrics"
    "github.com/shortlink-org/go-sdk/observability/tracing"
    "github.com/shortlink-org/go-sdk/temporal"
)

func main() {
    ctx := context.Background()
    cfg, _ := config.New()
    log, _, _ := logger.NewDefault(ctx, cfg)
    tracer, _, _ := tracing.New(ctx, log, cfg)
    monitor, _, _ := metrics.New(ctx, log, tracer, cfg)
    
    client, err := temporal.New(log, cfg, tracer, monitor)
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    // Health check
    if err := temporal.CheckHealth(ctx, client); err != nil {
        log.Error("Temporal health check failed")
    }
}
```

## Configuration

### Temporal

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TEMPORAL_HOST` | `temporal-frontend.temporal.svc.cluster.local:7233` | Temporal server address |
| `TEMPORAL_NAMESPACE` | `default` | Temporal namespace |
| `TEMPORAL_IDENTITY` | (empty) | Worker identity (optional) |

### gRPC Client (from go-sdk/grpc)

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `GRPC_CLIENT_TLS_ENABLED` | `false` | Enable TLS |
| `GRPC_CLIENT_CERT_PATH` | `ops/cert/intermediate_ca.pem` | TLS certificate path |
| `GRPC_CLIENT_TIMEOUT` | `10s` | Request timeout |

## Observability

All observability is unified through OpenTelemetry:

```
┌─────────────────────────────────────────────────────────────┐
│                    Temporal Client                          │
├─────────────────────────────────────────────────────────────┤
│  Tracing (OpenTelemetry)                                    │
│    └── Workflow spans, Activity spans, Child Workflow spans │
├─────────────────────────────────────────────────────────────┤
│  Metrics (OpenTelemetry via MetricsHandler)                 │
│    ├── temporal_workflow_task_*                             │
│    ├── temporal_activity_*                                  │
│    └── temporal_*_poll_*                                    │
├─────────────────────────────────────────────────────────────┤
│  gRPC Metrics (go-sdk/grpc)                                 │
│    └── grpc_client_* (latency, errors, etc.)                │
└─────────────────────────────────────────────────────────────┘
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Service (OMS)                          │
├─────────────────────────────────────────────────────────────┤
│  temporal.New(logger, config, tracer, monitoring)           │
│       │                                                     │
│       ├── Temporal Interceptors:                            │
│       │    └── OpenTelemetry tracing                        │
│       │                                                     │
│       ├── Temporal MetricsHandler:                          │
│       │    └── OpenTelemetry metrics                        │
│       │                                                     │
│       └── go-sdk/grpc options:                              │
│            ├── WithAuthForward() ─ Istio/Oathkeeper JWT     │
│            ├── WithTracer()      ─ gRPC tracing             │
│            ├── WithMetrics()     ─ gRPC metrics             │
│            └── WithTimeout()     ─ 10s default              │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│               Istio Sidecar (mTLS)                          │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│    temporal-frontend.temporal.svc.cluster.local:7233        │
└─────────────────────────────────────────────────────────────┘
```

## References

- [Temporal Go SDK](https://github.com/temporalio/sdk-go)
- [Temporal Go SDK Observability](https://docs.temporal.io/develop/go/observability)
- [Temporal Client Documentation](https://docs.temporal.io/develop/go/temporal-client)
