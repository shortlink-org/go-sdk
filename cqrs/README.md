# CQRS layer for Watermill-powered services

The `cqrs` package provides a thin, protobuf-first CQRS abstraction on top of [Watermill](../watermill/README.md).

It standardizes Shortlink message naming, metadata, tracing, and handler registration without introducing aggregates or event-sourcing concerns.

## Architecture

```
   [Your Service Code]

           |

        handlers  ← typed business handlers

           |

   cqrs.{bus,marshaler,namer}

           |

        [Watermill](../watermill/README.md)    ← publisher/subscriber/router

           |

        Kafka/NATS
```

## ✨ Key Features

- **Canonical message identity** (`billing.command.create_invoice.v1`)
- **Proto-first marshaling** with typed registry
- **Trace & metadata propagation** (OTEL)
- **CommandBus / EventBus** with middleware
- **Automatic [Watermill](../watermill/README.md) router integration**
- **Handler Decorators:** retry, timeout, CB, recover

## Quick Start

Publish a command:

```go
_ = commandBus.Send(ctx, &billingv1.CreateInvoiceCommand{
    OrderId: "123",
})
```

Handle an event:

```go
func (h *InvoiceCreatedProjector) Handle(ctx context.Context, evt *billingv1.InvoiceCreatedEvent) error {
    // process event
    return nil
}
```

## Metadata layout

Every [Watermill](../watermill/README.md) message produced by this layer carries:

| Key | Description |
| --- | --- |
| `shortlink.service_name` | service emitting the message (comes from `Namer` / router config) |
| `shortlink.message_kind` | either `command` or `event` |
| `shortlink.type_name` | canonical name without version (`billing.command.create_order`) |
| `shortlink.type_version` | semantic version (`v1` by default) |
| `shortlink.content_type` | media type (`application/x-protobuf`) |
| `shortlink.trace_id` / `shortlink.span_id` | OTel trace context |
| `shortlink.occurred_at` | RFC3339 timestamp of emission |

Example [Watermill](../watermill/README.md) message metadata:

```json
{
  "shortlink.service_name": "billing",
  "shortlink.message_kind": "event",
  "shortlink.type_name": "billing.event.invoice_created",
  "shortlink.type_version": "v1",
  "shortlink.content_type": "application/x-protobuf",
  "shortlink.trace_id": "cc50e...",
  "shortlink.span_id": "97bf...",
  "shortlink.occurred_at": "2025-11-25T21:52:02Z"
}
```

These keys are automatically consumed by the namer (`message.NameOf`, `TopicForCommand`, `TopicForEvent`) ensuring commands/events can be resolved from metadata alone.

### Override Namespace

The `shortlink.` namespace is the default. Override it globally via environment variable:

```bash
export SHORTLINK_METADATA_NAMESPACE="mycorp"
```

The SDK recalculates all metadata keys automatically (e.g., `mycorp.service_name`, `mycorp.trace_id`). This is useful for multi-tenant clusters or when integrating with existing systems that use different naming conventions.

## Complete Example

```go
import (
    wmmessage "github.com/ThreeDotsLabs/watermill/message"
    "github.com/shortlink-org/go-sdk/cqrs/bus"
    "github.com/shortlink-org/go-sdk/cqrs/handlers"
    cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
    "github.com/shortlink-org/go-sdk/cqrs/router"
)

// bootstrap shared components
registry := bus.NewTypeRegistry()
if err := registry.RegisterCommand(&billingv1.CreateInvoiceCommand{}); err != nil {
    panic(err)
}
if err := registry.RegisterEvent(&billingv1.InvoiceCreatedEvent{}); err != nil {
    panic(err)
}

namer := cqrsmessage.NewShortlinkNamer("billing")
marshaler := cqrsmessage.NewProtoMarshaler(namer)

commandBus := bus.NewCommandBus(watermillPublisher, marshaler, namer)
eventBus := bus.NewEventBus(watermillPublisher, marshaler, namer)

createHandler := handlers.NewCommandHandler(&CreateInvoiceHandler{}, registry, marshaler)

builderCfg := router.RouterConfig{
    ServiceName: "billing",
    Handlers: []router.HandlerRegistration{
        {
            Name:    "create_invoice_command",
            Topic:   cqrsmessage.TopicForCommand(namer.CommandName(&billingv1.CreateInvoiceCommand{})),
            Handler: createHandler,
        },
        {
            Name:    "invoice_created_event",
            Topic:   cqrsmessage.TopicForEvent(namer.EventName(&billingv1.InvoiceCreatedEvent{})),
            Handler: handlers.NewEventHandler(&InvoiceCreatedProjector{}, registry, marshaler),
        },
    },
    Middlewares: router.RouterMiddlewareConfig{
        Timeout:               10 * time.Second,
        RetryMax:              5,
        CircuitBreakerEnabled: true,
    },
}

rt, err := router.NewRouter(wmLogger, watermillSubscriber, watermillPublisher, builderCfg)
if err != nil {
    panic(err)
}
```

The buses publish protobuf payloads with tracing metadata, the router validates subscribed topics against the registry, and typed handlers can focus on business code.

## Topic naming

Topics reuse canonical names (e.g. `billing.command.create_invoice.v1`). Helper functions `TopicForCommand` and `TopicForEvent` can be used everywhere to keep publishers/subscribers aligned with Kafka settings declared in [`go-sdk/watermill`](../watermill/README.md).
