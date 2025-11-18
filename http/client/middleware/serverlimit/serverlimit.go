package serverlimit

import (
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
	"github.com/shortlink-org/go-sdk/http/client/middleware/otelwait"
)

const (
	hostStateTTL        = 5 * time.Minute
	hostCleanupInterval = time.Minute
)

type Config struct {
	JitterFraction float64
	Metrics        *types.Metrics
	Client         string
}

func Middleware(cfg Config) types.Middleware {
	jitterFraction := cfg.JitterFraction
	if jitterFraction < 0 {
		jitterFraction = 0
	}

	if jitterFraction > 1 {
		jitterFraction = 1
	}

	limiter := newHostLimiter()

	return func(next http.RoundTripper) http.RoundTripper {
		return types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			state := limiter.stateFor(req.URL.Host)

			var wait time.Duration

			state.mu.Lock()

			currentTime := time.Now()
			if state.t.After(currentTime) {
				wait = time.Until(state.t)
			}

			state.mu.Unlock()

			if wait > 0 {
				wait = limiter.addJitter(wait, jitterFraction)

				timer := time.NewTimer(wait)
				select {
				case <-req.Context().Done():
					timer.Stop()
					return nil, req.Context().Err()
				case <-timer.C:
					timer.Stop()
				}

				otelwait.RecordWait(req.Context(), "server", wait)

				if cfg.Metrics != nil {
					cfg.Metrics.RateLimitWaitSeconds.
						WithLabelValues(cfg.Client, req.URL.Host, req.Method, "server").
						Observe(wait.Seconds())
				}
			}

			resp, err := next.RoundTrip(req)
			if err != nil {
				return nil, err
			}

			if newUntil := nextFromHeaders(resp, time.Now()); !newUntil.IsZero() {
				state.mu.Lock()

				if newUntil.After(state.t) {
					state.t = newUntil
				}

				state.lastUsed = time.Now()
				state.mu.Unlock()
			}

			return resp, nil
		})
	}
}

func nextFromHeaders(resp *http.Response, now time.Time) time.Time {
	// Retry-After (seconds or HTTP-date)
	if v := resp.Header.Get("Retry-After"); v != "" {
		seconds, atoiErr := strconv.Atoi(v)
		if atoiErr == nil {
			return now.Add(time.Duration(seconds) * time.Second)
		}

		parsedTime, parseErr := http.ParseTime(v)
		if parseErr == nil {
			return parsedTime
		}
	}

	// RateLimit-Reset (seconds)
	if v := resp.Header.Get("Ratelimit-Reset"); v != "" {
		seconds, err := strconv.Atoi(v)
		if err == nil {
			return now.Add(time.Duration(seconds) * time.Second)
		}
	}

	return time.Time{}
}

func (h *hostLimiter) addJitter(duration time.Duration, fraction float64) time.Duration {
	jitterRange := int64(float64(duration) * fraction)
	if jitterRange <= 0 {
		return duration
	}

	h.muRand.Lock()
	offset := h.rand.Int63n(2*jitterRange+1) - jitterRange
	h.muRand.Unlock()

	return duration + time.Duration(offset)
}

type hostState struct {
	mu       sync.Mutex
	t        time.Time
	lastUsed time.Time
}

type hostLimiter struct {
	states sync.Map
	stop   chan struct{}
	once   sync.Once
	rand   *rand.Rand
	muRand sync.Mutex
}

func newHostLimiter() *hostLimiter {
	hostLimiterInstance := new(hostLimiter)
	hostLimiterInstance.stop = make(chan struct{})
	hostLimiterInstance.rand = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // jitter does not require cryptographic randomness

	go hostLimiterInstance.cleanupLoop()

	runtime.SetFinalizer(hostLimiterInstance, func(l *hostLimiter) {
		l.once.Do(func() {
			close(l.stop)
		})
	})

	return hostLimiterInstance
}

// Close stops the cleanup goroutine and prevents resource leaks.
// It's safe to call Close multiple times.
func (h *hostLimiter) Close() error {
	h.once.Do(func() {
		close(h.stop)
		runtime.SetFinalizer(h, nil)
	})

	return nil
}

func (h *hostLimiter) stateFor(host string) *hostState {
	now := time.Now()

	if val, ok := h.states.Load(host); ok {
		state, castOK := val.(*hostState)
		if !castOK {
			return h.createState(now, host)
		}

		state.mu.Lock()
		state.lastUsed = now
		state.mu.Unlock()

		return state
	}

	return h.createState(now, host)
}

func (h *hostLimiter) createState(now time.Time, host string) *hostState {
	state := new(hostState)
	state.lastUsed = now

	actual, loaded := h.states.LoadOrStore(host, state)
	if !loaded {
		return state
	}

	existing, castOK := actual.(*hostState)
	if !castOK {
		// If cast failed, try to store our new state again
		// This handles the rare case where the value type changed
		actual2, loaded2 := h.states.LoadOrStore(host, state)
		if !loaded2 {
			return state
		}

		// If still loaded, use what's there
		if existing2, ok := actual2.(*hostState); ok {
			existing2.mu.Lock()
			if existing2.lastUsed.Before(now) {
				existing2.lastUsed = now
			}
			existing2.mu.Unlock()

			return existing2
		}

		return state
	}

	existing.mu.Lock()

	// Update lastUsed atomically to avoid race conditions
	if existing.lastUsed.Before(now) {
		existing.lastUsed = now
	}

	existing.mu.Unlock()

	return existing
}

func (h *hostLimiter) cleanupLoop() {
	ticker := time.NewTicker(hostCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.cleanup()
		case <-h.stop:
			return
		}
	}
}

func (h *hostLimiter) cleanup() {
	now := time.Now()

	h.states.Range(func(key, value any) bool {
		state, ok := value.(*hostState)
		if !ok {
			return true
		}

		state.mu.Lock()
		idleTooLong := now.Sub(state.lastUsed) > hostStateTTL
		limitActive := state.t.After(now)
		state.mu.Unlock()

		if idleTooLong && !limitActive {
			h.states.Delete(key)
		}

		return true
	})
}
