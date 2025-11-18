package http_client

import (
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	hostStateTTL        = 5 * time.Minute
	hostCleanupInterval = time.Minute
)

type ServerLimitConfig struct {
	JitterFraction float64
	Metrics        *Metrics
	Client         string
}

func ServerRateLimitMiddleware(cfg ServerLimitConfig) Middleware {
	jitterFraction := cfg.JitterFraction
	if jitterFraction < 0 {
		jitterFraction = 0
	}

	if jitterFraction > 1 {
		jitterFraction = 1
	}

	limiter := newHostLimiter()

	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			state := limiter.stateFor(req.URL.Host)

			var wait time.Duration

			state.mu.Lock()

			currentTime := time.Now()
			if state.t.After(currentTime) {
				wait = time.Until(state.t)
			}

			state.mu.Unlock()

			if wait > 0 {
				wait = addJitter(wait, jitterFraction)

				timer := time.NewTimer(wait)
				select {
				case <-req.Context().Done():
					timer.Stop()
					return nil, req.Context().Err()
				case <-timer.C:
					timer.Stop()
				}

				recordWait(req.Context(), "server", wait)

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
	var until time.Time

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

	return until
}

func addJitter(duration time.Duration, fraction float64) time.Duration {
	jitterRange := int64(float64(duration) * fraction)
	if jitterRange <= 0 {
		return duration
	}

	// #nosec G404 -- jitter does not require cryptographic randomness.
	offset := rand.Int63n(2*jitterRange+1) - jitterRange

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
}

func newHostLimiter() *hostLimiter {
	hostLimiterInstance := new(hostLimiter)
	hostLimiterInstance.stop = make(chan struct{})

	go hostLimiterInstance.cleanupLoop()

	runtime.SetFinalizer(hostLimiterInstance, func(l *hostLimiter) {
		l.once.Do(func() {
			close(l.stop)
		})
	})

	return hostLimiterInstance
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
		return state
	}

	existing.mu.Lock()

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
