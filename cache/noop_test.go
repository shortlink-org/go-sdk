//go:build unit || (database && redis)

package cache_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/cache"
)

// answer is any value at all: what matters is that Noop refuses to remember it.
const answer = 42

func TestNoop(t *testing.T) {
	t.Parallel()

	// Noop is bound where Redis would be, so what is under test is that it
	// satisfies the port and never fails.
	var client cache.Cache = cache.Noop{}

	name := uuid.New().String()

	require.NoError(t, client.Set(t.Context(), name, []byte("v"), time.Minute))
	require.NoError(t, client.Set(t.Context(), name, []byte("v"), 0))

	got, err := client.Get(t.Context(), name)
	require.ErrorIs(t, err, cache.ErrMiss)
	require.Nil(t, got)

	require.NoError(t, client.Delete(t.Context(), name))
	require.NoError(t, client.Delete(t.Context()))

	require.NoError(t, cache.SetJSON(t.Context(), client, name, answer, time.Minute))

	_, err = cache.GetJSON[int](t.Context(), client, name)
	require.ErrorIs(t, err, cache.ErrMiss)
}
