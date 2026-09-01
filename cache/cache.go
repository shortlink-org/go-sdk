// Package cache is a small byte-level cache port and its Redis adapter.
//
// The package deliberately exposes an interface rather than a concrete client:
// a consumer binds Cache in its container, fakes it in a test, and swaps in
// Noop where a deployment runs without Redis. Nothing here leaks the Redis
// client library into the consumer's imports.
//
// Values are bytes. What a value looks like on the wire is the business of
// whoever stored it, so this package imposes no codec — see GetJSON and SetJSON
// for optional sugar over encoding/json/v2.
package cache

import (
	"context"
	"errors"
	"time"
)

// ErrMiss reports that the key is not in the cache.
//
// A miss is an ordinary outcome, not a failure: callers compare with errors.Is
// and fall through to the source of truth. It is returned bare, never wrapped
// in a value, so a stored empty slice stays distinguishable from an absent key.
var ErrMiss = errors.New("cache: miss")

// Cache is the port. Everything in this package either implements it or is a
// free function over it.
type Cache interface {
	// Get returns the stored bytes, or ErrMiss when the key is absent.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores value under key for ttl. A ttl of zero or less stores
	// nothing: an entry without an expiry is not expressible through this
	// port, so no caller can leave one behind by accident.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes the given keys. Removing a key that is not there is not
	// an error — the write path usually has no idea whether anybody read the
	// thing it just changed.
	Delete(ctx context.Context, keys ...string) error
}
