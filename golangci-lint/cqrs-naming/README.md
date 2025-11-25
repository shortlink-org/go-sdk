# cqrs-naming plugin

> [!NOTE]
> This `golangci-lint` plugin ensures that CQRS commands and events follow naming conventions
> and are used correctly with their respective buses.
> 
> **Compatible with golangci-lint v2.0+**

## Rules

1. **Command types** (ending with `Command`) must be used with `CommandBus.Send`
2. **Event types** (ending with `Event`) must be used with `EventBus.Publish`
3. Commands cannot be published via `EventBus.Publish`
4. Events cannot be sent via `CommandBus.Send`

## Getting Started

We use Makefile for build and deploy.

```bash
make help # show help message with all commands and targets
make build # build the plugin
```

## Example Usage

### Bad Code

```go
package example

import (
    "github.com/shortlink-org/go-sdk/cqrs/bus"
)

type CreateOrderCommand struct {
    OrderID string
}

type OrderCreatedEvent struct {
    OrderID string
}

func badExample(commandBus *bus.CommandBus, eventBus *bus.EventBus) {
    cmd := &CreateOrderCommand{OrderID: "123"}
    evt := &OrderCreatedEvent{OrderID: "123"}
    
    // ❌ Error: Event type used with CommandBus
    commandBus.Send(ctx, evt)
    
    // ❌ Error: Command type used with EventBus
    eventBus.Publish(ctx, cmd)
}
```

### Good Code

```go
package example

import (
    "github.com/shortlink-org/go-sdk/cqrs/bus"
)

type CreateOrderCommand struct {
    OrderID string
}

type OrderCreatedEvent struct {
    OrderID string
}

func goodExample(commandBus *bus.CommandBus, eventBus *bus.EventBus) {
    cmd := &CreateOrderCommand{OrderID: "123"}
    evt := &OrderCreatedEvent{OrderID: "123"}
    
    // ✅ Correct: Command used with CommandBus
    commandBus.Send(ctx, cmd)
    
    // ✅ Correct: Event used with EventBus
    eventBus.Publish(ctx, evt)
}
```

## Integration with golangci-lint

Add the plugin to your `.golangci.yml`:

```yaml
linters-settings:
  custom:
    cqrsnaming:
      path: ./golangci-lint/cqrs-naming/bin/cqrs-naming.so
      description: Validates CQRS naming conventions

linters:
  enable:
    - cqrsnaming
```
