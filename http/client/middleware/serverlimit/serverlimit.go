// Package serverlimit provides HTTP client middleware for respecting
// server-specified rate limits via Retry-After and RateLimit-Reset headers.
package serverlimit

import (
	"io"
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

// Config configures the server limit middleware.
type Config struct {
	JitterFraction float64
	Metrics        *types.Metrics
	Client         string
}

var (
	sharedLimiterOnce sync.Once
	sharedLimiter     *hostLimiter
)

// Middleware returns a middleware that shares a process-wide limiter.
// It avoids spawning a cleanup goroutine per client; use New() if you need explicit Close().
func Middleware(cfg Config) types.Middleware {
	limiter := &ServerLimiter{
		limiter:        sharedHostLimiter(),
		jitterFraction: normalizeJitter(cfg.JitterFraction),
		metrics:        cfg.Metrics,
		clientName:     cfg.Client,
	}

	return limiter.Middleware()
}

// ServerLimiter manages rate limiting based on server response headers.
// It tracks per-host rate limits and respects Retry-After/RateLimit-Reset headers.
//
// Important: Call Close() when done to stop the background cleanup goroutine.
type ServerLimiter struct {
	limiter        *hostLimiter
	jitterFraction float64
	metrics        *types.Metrics
	clientName     string
}

// New creates a new ServerLimiter.
// The caller must call Close() when done to prevent goroutine leaks.
func New(cfg Config) *ServerLimiter {
	return &ServerLimiter{
		limiter:        newHostLimiter(),
		jitterFraction: normalizeJitter(cfg.JitterFraction),
		metrics:        cfg.Metrics,
		clientName:     cfg.Client,
	}
}

// Close stops the background cleanup goroutine and releases resources.
// It's safe to call Close multiple times.
// Implements io.Closer.
func (s *ServerLimiter) Close() error {
	return s.limiter.Close()
}

// Middleware returns the HTTP client middleware.
func (s *ServerLimiter) Middleware() types.Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return types.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			state := s.limiter.stateFor(req.URL.Host)

			var wait time.Duration

			state.mu.Lock()

			currentTime := time.Now()
			if state.t.After(currentTime) {
				wait = time.Until(state.t)
			}

			state.mu.Unlock()

			if wait > 0 {
				wait = s.limiter.addJitter(wait, s.jitterFraction)

				timer := time.NewTimer(wait)
				select {
				case <-req.Context().Done():
					timer.Stop()

					return nil, req.Context().Err()
				case <-timer.C:
					timer.Stop()
				}

				otelwait.RecordWait(req.Context(), "server", wait)

				if s.metrics != nil {
					s.metrics.RateLimitWaitSeconds.
						WithLabelValues(s.clientName, req.URL.Host, req.Method, "server").
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

// Ensure ServerLimiter implements io.Closer.
var _ io.Closer = (*ServerLimiter)(nil)

func sharedHostLimiter() *hostLimiter {
	sharedLimiterOnce.Do(func() {
		sharedLimiter = newHostLimiter()
	})

	return sharedLimiter
}

func normalizeJitter(fraction float64) float64 {
	if fraction < 0 {
		return 0
	}

	if fraction > 1 {
		return 1
	}

	return fraction
}

func nextFromHeaders(resp *http.Response, now time.Time) time.Time {
	// Retry-After (seconds or HTTP-date)
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter != "" {
		seconds, atoiErr := strconv.Atoi(retryAfter)
		if atoiErr == nil {
			return now.Add(time.Duration(seconds) * time.Second)
		}

		parsedTime, parseErr := http.ParseTime(retryAfter)
		if parseErr == nil {
			return parsedTime
		}
	}

	// RateLimit-Reset (seconds)
	rateLimitReset := resp.Header.Get("Ratelimit-Reset")
	if rateLimitReset != "" {
		seconds, err := strconv.Atoi(rateLimitReset)
		if err == nil {
			return now.Add(time.Duration(seconds) * time.Second)
		}
	}

	return time.Time{}
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
	hostLimiterInstance := &hostLimiter{
		stop: make(chan struct{}),
		//nolint:gosec // jitter does not require cryptographic randomness
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

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
	state := &hostState{lastUsed: now}

	actual, loaded := h.states.LoadOrStore(host, state)
	if !loaded {
		return state
	}

	existing, castOK := actual.(*hostState)
	if !castOK {
		return h.handleCastFailure(now, host, state)
	}

	existing.mu.Lock()

	if existing.lastUsed.Before(now) {
		existing.lastUsed = now
	}

	existing.mu.Unlock()

	return existing
}

func (h *hostLimiter) handleCastFailure(now time.Time, host string, state *hostState) *hostState {
	actual2, loaded2 := h.states.LoadOrStore(host, state)
	if !loaded2 {
		return state
	}

	existing2, ok := actual2.(*hostState)
	if !ok {
		return state
	}

	existing2.mu.Lock()

	if existing2.lastUsed.Before(now) {
		existing2.lastUsed = now
	}

	existing2.mu.Unlock()

	return existing2
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
