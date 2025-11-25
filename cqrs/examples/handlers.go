package main

import (
	"context"
	"fmt"
	"time"
)

// CreateOrderHandler processes CreateOrderCommand
type CreateOrderHandler struct{}

func (h *CreateOrderHandler) Handle(ctx context.Context, cmd *CreateOrderCommand) error {
	fmt.Printf("📦 Processing command: CreateOrderCommand{OrderId: %s, UserId: %s, Amount: %.2f}\n",
		cmd.OrderId, cmd.UserId, cmd.Amount)

	// Simulate business logic
	time.Sleep(100 * time.Millisecond)

	// In real app, you would publish events here
	fmt.Printf("✅ Order created successfully: %s\n", cmd.OrderId)
	return nil
}

// OrderCreatedProjector processes OrderCreatedEvent
type OrderCreatedProjector struct{}

func (p *OrderCreatedProjector) Handle(ctx context.Context, evt *OrderCreatedEvent) error {
	fmt.Printf("📊 Projecting event: OrderCreatedEvent{OrderId: %s, UserId: %s, Amount: %.2f}\n",
		evt.OrderId, evt.UserId, evt.Amount)

	// Simulate projection logic (e.g., update read model)
	time.Sleep(50 * time.Millisecond)

	fmt.Printf("✅ Projection updated for order: %s\n", evt.OrderId)
	return nil
}
