package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/shortlink-org/go-sdk/cqrs/bus"
	"github.com/shortlink-org/go-sdk/cqrs/handlers"
	cqrsmessage "github.com/shortlink-org/go-sdk/cqrs/message"
	"github.com/shortlink-org/go-sdk/cqrs/router"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup Watermill pub/sub (using in-memory channel for demo)
	pubsub := gochannel.NewGoChannel(
		gochannel.Config{},
		watermill.NewStdLogger(false, false),
	)
	defer pubsub.Close()

	// Create type registry and register commands/events
	registry := bus.NewTypeRegistry()
	if err := registry.RegisterCommand(&CreateOrderCommand{}); err != nil {
		log.Fatalf("Failed to register command: %v", err)
	}
	if err := registry.RegisterEvent(&OrderCreatedEvent{}); err != nil {
		log.Fatalf("Failed to register event: %v", err)
	}

	// Create namer and JSON marshaler
	namer := cqrsmessage.NewShortlinkNamer("orders")
	marshaler := cqrsmessage.NewJSONMarshaler(namer)

	// Create command and event buses
	commandBus := bus.NewCommandBus(pubsub, marshaler, namer)
	eventBus := bus.NewEventBus(pubsub, marshaler, namer)

	// Create handlers
	createOrderHandler := handlers.NewCommandHandler(&CreateOrderHandler{}, registry, marshaler)
	orderCreatedHandler := handlers.NewEventHandler(&OrderCreatedProjector{}, registry, marshaler)

	// Setup router
	routerCfg := router.RouterConfig{
		ServiceName: "orders",
		Handlers: []router.HandlerRegistration{
			{
				Name:    "create_order_command",
				Topic:   cqrsmessage.TopicForCommand(namer.CommandName(&CreateOrderCommand{})),
				Handler: createOrderHandler,
			},
			{
				Name:    "order_created_event",
				Topic:   cqrsmessage.TopicForEvent(namer.EventName(&OrderCreatedEvent{})),
				Handler: orderCreatedHandler,
			},
		},
		Middlewares: router.RouterMiddlewareConfig{
			Timeout:               5 * time.Second,
			RetryMax:              3,
			CircuitBreakerEnabled: false, // Disabled for demo
		},
	}

	rt, err := router.NewRouter(
		watermill.NewStdLogger(false, false),
		pubsub,
		pubsub,
		routerCfg,
	)
	if err != nil {
		log.Fatalf("Failed to create router: %v", err)
	}

	// Start router
	go func() {
		if err := rt.Run(ctx); err != nil {
			log.Fatalf("Router error: %v", err)
		}
	}()

	// Wait for router to start
	time.Sleep(100 * time.Millisecond)

	// Publish a command
	fmt.Println("\n🚀 Publishing CreateOrderCommand...")
	cmd := &CreateOrderCommand{
		OrderId:   "order-123",
		UserId:    "user-456",
		ProductId: "product-789",
		Amount:    99.99,
	}

	if err := commandBus.Send(ctx, cmd); err != nil {
		log.Fatalf("Failed to send command: %v", err)
	}

	// Publish an event
	fmt.Println("\n📢 Publishing OrderCreatedEvent...")
	evt := &OrderCreatedEvent{
		OrderId:   "order-123",
		UserId:    "user-456",
		ProductId: "product-789",
		Amount:    99.99,
		CreatedAt: time.Now().Unix(),
	}

	if err := eventBus.Publish(ctx, evt); err != nil {
		log.Fatalf("Failed to publish event: %v", err)
	}

	// Wait for handlers to process
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n✨ Example completed successfully!")
	cancel()
}
