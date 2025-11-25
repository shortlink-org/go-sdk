package notify

import (
	"context"
	"sync"
)

// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
type Publisher[T any] interface {
	Subscribe(event *int, subscriber Subscriber[T])
	UnSubscribe(subscriber Subscriber[T])
}

// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
type Subscriber[T any] interface {
	Notify(ctx context.Context, event uint32, payload T) Response[T]
}

// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
type Notify[T any] struct {
	mu            sync.RWMutex
	subscriberMap map[uint32][]Subscriber[T]
}

// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
type Response[T any] struct {
	Payload T
	Error   error
	Name    string
}

// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
type Callback struct {
	CB             chan<- any
	ResponseFilter string
}
