package cache

import (
	"context"
	"time"
)

// Noop is a Cache that stores nothing and always misses.
//
// It is how "this deployment has no cache" is expressed: a consumer binds Noop
// instead of Redis and keeps one code path, rather than testing the cache for
// nil at every call site.
type Noop struct{}

// Get always reports ErrMiss.
func (Noop) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, ErrMiss
}

// Set discards the value.
func (Noop) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

// Delete does nothing.
func (Noop) Delete(_ context.Context, _ ...string) error {
	return nil
}
