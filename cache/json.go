package cache

import (
	"context"
	"encoding/json/v2"
	"time"
)

// GetJSON reads a key and decodes it as T.
//
// It is a convenience over the port, not part of it: the port stays bytes so
// that the codec belongs to whoever owns the value, and a consumer that
// serializes with anything else — protobuf, msgpack, a hand-rolled encoding —
// ignores this function and loses nothing. A miss is ErrMiss, as it is on Get.
//
//nolint:ireturn // T is the caller's own concrete value type
func GetJSON[T any](ctx context.Context, client Cache, key string) (T, error) {
	var value T

	raw, err := client.Get(ctx, key)
	if err != nil {
		return value, err
	}

	err = json.Unmarshal(raw, &value)
	if err != nil {
		var zero T

		return zero, NewCacheError("unmarshal", err)
	}

	return value, nil
}

// SetJSON encodes value as JSON and stores it for ttl. As on Set, a ttl of zero or
// less stores nothing.
func SetJSON[T any](ctx context.Context, client Cache, key string, value T, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return NewCacheError("marshal", err)
	}

	return client.Set(ctx, key, raw, ttl)
}
