//go:build unit || (database && redis)

package cache_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/cache"
)

// errSentinel stands in for whatever the driver failed with.
var errSentinel = errors.New("connection refused")

func TestErrorUnwrap(t *testing.T) {
	t.Parallel()

	t.Run("errors.Is reaches the cause through a cache error", func(t *testing.T) {
		t.Parallel()

		err := cache.NewCacheError("get", errSentinel)

		// Without Unwrap the operation name cost the caller every detail
		// behind it, and a timeout could not be told from a closed client.
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "get")
	})

	t.Run("errors.AsType finds the cache error through a wrap", func(t *testing.T) {
		t.Parallel()

		wrapped := fmt.Errorf("session lookup: %w", cache.NewCacheError("set", errSentinel))

		base, ok := errors.AsType[*cache.BaseError](wrapped)
		require.True(t, ok)
		require.Equal(t, "set", base.Op())
		require.ErrorIs(t, base, errSentinel)
	})
}
