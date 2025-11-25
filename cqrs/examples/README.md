# CQRS Basic Example

This example demonstrates the basic usage of the `cqrs` package with Watermill.

## What it does

- Registers command and event types in a type registry
- Creates command and event buses
- Sets up typed handlers for commands and events
- Publishes a command and an event
- Processes them through the router

## Running the example

```bash
cd cqrs/examples
go mod tidy
go run main.go
```

## Expected output

```
🚀 Publishing CreateOrderCommand...
📦 Processing command: CreateOrderCommand{OrderId: order-123, UserId: user-456, Amount: 99.99}
✅ Order created successfully: order-123

📢 Publishing OrderCreatedEvent...
📊 Projecting event: OrderCreatedEvent{OrderId: order-123, UserId: user-456, Amount: 99.99}
✅ Projection updated for order: order-123

✨ Example completed successfully!
```

## Key concepts

- **Type Registry**: Central registry for all commands and events
- **Namer**: Generates canonical names like `orders.command.create_order.v1`
- **Marshaler**: Serializes protobuf messages with metadata
- **CommandBus/EventBus**: Publish commands and events with automatic metadata enrichment
- **Handlers**: Typed handlers that receive unmarshaled protobuf messages
- **Router**: Watermill router with CQRS middleware (retry, timeout, etc.)

