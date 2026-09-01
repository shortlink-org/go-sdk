//go:build unit || (database && redis)

package cache_test

import (
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"
	"uuid"

	"github.com/redis/rueidis"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/shortlink-org/go-sdk/cache"
	"github.com/shortlink-org/go-sdk/config"
)

// key returns a name no other test in this package uses. The tests share one
// container, so a fixed name would let one case observe another's writes.
func key(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("%s:%s", t.Name(), uuid.NewV7())
}

// startRedis brings up the container the whole file shares and returns its
// host:port. Without a Docker daemon the tests skip: the module still has to
// build and vet on a machine that has none.
func startRedis(t *testing.T) string {
	t.Helper()

	ctx := t.Context()

	container, err := tcredis.Run(ctx, "redis:latest")
	if err != nil {
		t.Skipf("redis container unavailable, skipping: %v", err)
	}

	testcontainers.CleanupContainer(t, container)

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	parsed, err := url.Parse(uri)
	require.NoError(t, err)

	return parsed.Host
}

// newCache opens the adapter against addr. Nothing is instrumented here on
// purpose: a nil tracer and a nil meter have to be accepted.
func newCache(t *testing.T, addr string, opts ...cache.Option) *cache.Redis {
	t.Helper()

	cfg, err := config.New()
	require.NoError(t, err)
	cfg.Set("STORE_REDIS_URI", addr)

	client, err := cache.NewRedis(t.Context(), cfg, opts...)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, client.Close()) })

	return client
}

const (
	// Short enough to observe an expiry without slowing the run.
	expiryTTL = 200 * time.Millisecond

	// These bound the polls that wait on Redis: key expiry and the
	// invalidation push both land on the server's schedule, not the test's.
	waitFor = 10 * time.Second
	tick    = 50 * time.Millisecond
)

func TestRedis(t *testing.T) {
	addr := startRedis(t)

	// The container is shared: every case keys off its own name, so the cases
	// are independent without paying for a container each.
	t.Run("round trip", func(t *testing.T) { testRoundTrip(t, addr) })
	t.Run("a miss is ErrMiss", func(t *testing.T) { testMiss(t, addr) })
	t.Run("an empty value is not a miss", func(t *testing.T) { testEmptyValue(t, addr) })
	t.Run("delete is idempotent", func(t *testing.T) { testDeleteIdempotent(t, addr) })
	t.Run("delete removes several keys at once", func(t *testing.T) { testDeleteMany(t, addr) })
	t.Run("an entry expires with its ttl", func(t *testing.T) { testExpiry(t, addr) })
	t.Run("set with a non-positive ttl stores nothing", func(t *testing.T) { testNonPositiveTTL(t, addr) })
	t.Run("a non-positive ttl does not overwrite a live entry", func(t *testing.T) { testTTLKeepsEntry(t, addr) })
	t.Run("json sugar round trips", func(t *testing.T) { testJSON(t, addr) })
}

func testRoundTrip(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)
	name := key(t)

	// Bytes go in and come back byte for byte, including the ones no string
	// codec would survive: a NUL, a byte that is not valid UTF-8, a newline.
	value := []byte("\x00\xffv\n")

	require.NoError(t, client.Set(t.Context(), name, value, time.Minute))

	got, err := client.Get(t.Context(), name)
	require.NoError(t, err)
	require.Equal(t, value, got)
}

func testMiss(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)

	got, err := client.Get(t.Context(), key(t))
	require.ErrorIs(t, err, cache.ErrMiss)
	require.Nil(t, got)
}

func testEmptyValue(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)
	name := key(t)

	// A stored empty value is not an absent key: that is the distinction the
	// old string-returning client could not make.
	require.NoError(t, client.Set(t.Context(), name, []byte{}, time.Minute))

	got, err := client.Get(t.Context(), name)
	require.NoError(t, err)
	require.Empty(t, got)
}

func testDeleteIdempotent(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)
	name := key(t)

	require.NoError(t, client.Set(t.Context(), name, []byte("v"), time.Minute))
	require.NoError(t, client.Delete(t.Context(), name))

	// The write path usually does not know whether anybody read the key, so
	// deleting an absent one — or nothing at all — has to stay quiet.
	require.NoError(t, client.Delete(t.Context(), name))
	require.NoError(t, client.Delete(t.Context()))

	_, err := client.Get(t.Context(), name)
	require.ErrorIs(t, err, cache.ErrMiss)
}

func testDeleteMany(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)
	first, second := key(t), key(t)

	require.NoError(t, client.Set(t.Context(), first, []byte("a"), time.Minute))
	require.NoError(t, client.Set(t.Context(), second, []byte("b"), time.Minute))
	require.NoError(t, client.Delete(t.Context(), first, second, key(t)))

	for _, name := range []string{first, second} {
		_, err := client.Get(t.Context(), name)
		require.ErrorIs(t, err, cache.ErrMiss)
	}
}

func testExpiry(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)
	name := key(t)

	// Redis expires keys on its own clock, so this waits for real time — a
	// testing/synctest bubble would only fake the Go side of it.
	require.NoError(t, client.Set(t.Context(), name, []byte("v"), expiryTTL))

	require.Eventually(t, func() bool {
		_, err := client.Get(t.Context(), name)

		return errors.Is(err, cache.ErrMiss)
	}, waitFor, tick, "entry outlived its ttl")
}

func testNonPositiveTTL(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)

	for _, ttl := range []time.Duration{0, -time.Second} {
		name := key(t)

		require.NoError(t, client.Set(t.Context(), name, []byte("v"), ttl))

		_, err := client.Get(t.Context(), name)
		require.ErrorIs(t, err, cache.ErrMiss, "ttl %s left an entry behind", ttl)
	}
}

func testTTLKeepsEntry(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)
	name := key(t)

	require.NoError(t, client.Set(t.Context(), name, []byte("kept"), time.Minute))
	require.NoError(t, client.Set(t.Context(), name, []byte("dropped"), 0))

	got, err := client.Get(t.Context(), name)
	require.NoError(t, err)
	require.Equal(t, []byte("kept"), got)
}

func testJSON(t *testing.T, addr string) {
	t.Helper()

	client := newCache(t, addr)
	name := key(t)

	type session struct {
		User  string `json:"user"`
		Admin bool   `json:"admin"`
	}

	want := session{User: "alice", Admin: true}
	require.NoError(t, cache.SetJSON(t.Context(), client, name, want, time.Minute))

	got, err := cache.GetJSON[session](t.Context(), client, name)
	require.NoError(t, err)
	require.Equal(t, want, got)

	_, err = cache.GetJSON[session](t.Context(), client, key(t))
	require.ErrorIs(t, err, cache.ErrMiss)
}

// TestClientSideCache covers the reason the local layer is DoCache and not an
// in-process LRU: Redis tracks what this connection cached and invalidates it,
// so a delete made elsewhere is not served stale.
func TestClientSideCache(t *testing.T) {
	addr := startRedis(t)

	client := newCache(t, addr, cache.WithClientSideCache(time.Hour))
	name := key(t)

	require.NoError(t, client.Set(t.Context(), name, []byte("v"), time.Hour))

	// Read once so the value is held locally; without this the next read would
	// go to the server anyway and prove nothing.
	got, err := client.Get(t.Context(), name)
	require.NoError(t, err)
	require.Equal(t, []byte("v"), got)

	// Delete from a second connection, which the local copy cannot observe on
	// its own — only the invalidation Redis pushes drops it.
	other, err := rueidis.NewClient(rueidis.ClientOption{InitAddress: []string{addr}})
	require.NoError(t, err)

	t.Cleanup(other.Close)

	require.NoError(t, other.Do(t.Context(), other.B().Del().Key(name).Build()).Error())

	// The invalidation arrives out of band, so the read is retried rather than
	// assumed to be immediate. The local TTL is an hour: nothing but the push
	// can make this pass.
	require.Eventually(t, func() bool {
		_, errGet := client.Get(t.Context(), name)

		return errors.Is(errGet, cache.ErrMiss)
	}, waitFor, tick, "stale copy survived a delete from another client")
}
