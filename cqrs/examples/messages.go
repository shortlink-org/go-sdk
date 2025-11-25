package main

// CreateOrderCommand represents a command to create an order
type CreateOrderCommand struct {
	OrderId   string  `json:"order_id"`
	UserId    string  `json:"user_id"`
	ProductId string  `json:"product_id"`
	Amount    float64 `json:"amount"`
}

// OrderCreatedEvent represents an event that an order was created
type OrderCreatedEvent struct {
	OrderId   string  `json:"order_id"`
	UserId    string  `json:"user_id"`
	ProductId string  `json:"product_id"`
	Amount    float64 `json:"amount"`
	CreatedAt int64   `json:"created_at"`
}
