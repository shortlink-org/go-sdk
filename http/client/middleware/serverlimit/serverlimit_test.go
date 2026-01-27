package serverlimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHostLimiterStateForPerHost(t *testing.T) {
	t.Parallel()

	limiter := newHostLimiter()

	t.Cleanup(func() {
		require.NoError(t, limiter.Close())
	})

	stateA1 := limiter.stateFor("api-a.example")
	stateA2 := limiter.stateFor("api-a.example")
	require.Same(t, stateA1, stateA2, "same host must reuse state")

	stateB := limiter.stateFor("api-b.example")
	require.NotSame(t, stateA1, stateB, "different hosts must have independent states")

	stateA1.mu.Lock()
	oldLastUsed := stateA1.lastUsed
	stateA1.mu.Unlock()

	time.Sleep(5 * time.Millisecond)

	stateA3 := limiter.stateFor("api-a.example")
	require.Same(t, stateA1, stateA3, "state should still be shared for host")

	stateA1.mu.Lock()
	newLastUsed := stateA1.lastUsed
	stateA1.mu.Unlock()

	require.False(t, newLastUsed.Before(oldLastUsed), "lastUsed must not go backwards")
}

func TestHostLimiterCleanupTTL(t *testing.T) {
	t.Parallel()

	limiter := newHostLimiter()

	t.Cleanup(func() {
		require.NoError(t, limiter.Close())
	})

	stale := limiter.stateFor("stale.example")
	stale.mu.Lock()
	stale.lastUsed = time.Now().Add(-2 * hostStateTTL)
	stale.t = time.Time{}
	stale.mu.Unlock()

	active := limiter.stateFor("active.example")
	active.mu.Lock()
	active.lastUsed = time.Now().Add(-2 * hostStateTTL)
	active.t = time.Now().Add(5 * time.Minute)
	active.mu.Unlock()

	limiter.cleanup()

	_, ok := limiter.states.Load("stale.example")
	require.False(t, ok, "stale host should be removed")

	_, ok = limiter.states.Load("active.example")
	require.True(t, ok, "active host should be retained even if lastUsed is old")
}

func TestNextFromHeaders(t *testing.T) {
	t.Parallel()

	now := time.Now()
	resp := new(http.Response)
	resp.Header = make(http.Header)

	resp.Header.Set("Retry-After", "10")
	next := nextFromHeaders(resp, now)
	require.Equal(t, now.Add(10*time.Second), next)

	resp.Header.Set("Retry-After", now.Add(30*time.Second).UTC().Format(http.TimeFormat))
	next = nextFromHeaders(resp, now)
	require.Equal(t, now.Add(30*time.Second).UTC().Truncate(time.Second), next.UTC().Truncate(time.Second))

	resp.Header.Del("Retry-After")
	resp.Header.Set("Ratelimit-Reset", "15")
	next = nextFromHeaders(resp, now)
	require.Equal(t, now.Add(15*time.Second), next)
}
