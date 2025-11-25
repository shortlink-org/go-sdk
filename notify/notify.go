/*
Package notify provides a simple notification system.

Deprecated: This package is deprecated. Use github.com/shortlink-org/go-sdk/cqrs instead.
*/
package notify

import (
	"context"

	"go.uber.org/atomic"
)

var (
	subscribers = Notify[any]{
		subscriberMap: map[uint32][]Subscriber[any]{},
	}
	eventCounter atomic.Uint32
)

// NewEventID generates a new unique event ID.
//
// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
func NewEventID() uint32 {
	eventCounter.Inc()
	return eventCounter.Load()
}

// Subscribe adds a subscriber to the specified event.
//
// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
func Subscribe(event uint32, subscriber Subscriber[any]) {
	subscribers.mu.Lock()
	subscribers.subscriberMap[event] = append(subscribers.subscriberMap[event], subscriber)
	subscribers.mu.Unlock()
}

// UnSubscribe removes a subscriber from the specified event.
//
// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
func UnSubscribe(event uint32, subscriber Subscriber[any]) {
	subscribers.mu.Lock()
	defer subscribers.mu.Unlock()

	for _, v := range subscribers.subscriberMap[event] {
		if subscriber == v {
			delete(subscribers.subscriberMap, event)
			break
		}
	}
}

// Publish sends an event with a payload to all subscribers.
// If a callback is provided, it returns the first successful response that matches the response filter.
//
// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
func Publish(ctx context.Context, event uint32, payload any, cb *Callback) {
	responses := map[string]Response[any]{}

	subscribers.mu.RLock()
	defer subscribers.mu.RUnlock()

	if len(subscribers.subscriberMap[event]) == 0 && cb != nil {
		cb.CB <- nil
	}

	for _, s := range subscribers.subscriberMap[event] {
		response := s.Notify(ctx, event, payload)

		if response.Error != nil && cb != nil {
			cb.CB <- response
			return
		}

		if response.Name != "" {
			responses[response.Name] = response
		}
	}

	if cb != nil && responses[cb.ResponseFilter].Name != "" {
		cb.CB <- responses[cb.ResponseFilter]
	}
}

// Clean resets the subscriber map.
//
// Deprecated: Use github.com/shortlink-org/go-sdk/cqrs instead.
func Clean() {
	subscribers = Notify[any]{
		subscriberMap: map[uint32][]Subscriber[any]{},
	}
}
