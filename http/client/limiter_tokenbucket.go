package http_client

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
)

// Limiter is an alias for types.Limiter for backward compatibility.
type Limiter = types.Limiter

type tokenBucketLimiter struct {
	mu sync.Mutex

	rate   float64
	burst  float64
	tokens float64
	last   time.Time

	jitterFraction float64
	rand           *rand.Rand
	muRand         sync.Mutex
}

//lint:ignore ireturn we intentionally return the concrete limiter type for customization
func NewTokenBucketLimiter(ratePerSec float64, burst int, jitterFraction float64) (*tokenBucketLimiter, error) {
	if ratePerSec <= 0 || burst <= 0 {
		return nil, types.ErrInvalidLimiterConfig
	}

	if jitterFraction < 0 {
		jitterFraction = 0
	}

	if jitterFraction > 1 {
		jitterFraction = 1
	}

	limiter := new(tokenBucketLimiter)
	limiter.rate = ratePerSec
	limiter.burst = float64(burst)
	limiter.tokens = float64(burst)
	limiter.last = time.Now()
	limiter.jitterFraction = jitterFraction
	limiter.rand = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // jitter does not require cryptographic randomness

	return limiter, nil
}

func (l *tokenBucketLimiter) Wait(ctx context.Context) (time.Duration, error) {
	var total time.Duration

	for {
		l.mu.Lock()

		now := time.Now()
		l.refill(now)

		if l.tokens >= 1 {
			l.tokens -= 1
			l.mu.Unlock()

			return total, nil
		}

		need := 1 - l.tokens
		sec := need / l.rate

		wait := max(time.Duration(sec*float64(time.Second)), time.Millisecond)

		wait = l.jitter(wait)

		l.mu.Unlock()

		total += wait

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return total, ctx.Err()
		case <-timer.C:
			timer.Stop()
		}
	}
}

func (l *tokenBucketLimiter) refill(now time.Time) {
	elapsed := now.Sub(l.last).Seconds()
	if elapsed <= 0 {
		return
	}

	l.last = now

	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
}

func (l *tokenBucketLimiter) jitter(duration time.Duration) time.Duration {
	jitterRange := int64(float64(duration) * l.jitterFraction)
	if jitterRange <= 0 {
		return duration
	}

	l.muRand.Lock()
	offset := l.rand.Int63n(2*jitterRange+1) - jitterRange
	l.muRand.Unlock()

	return duration + time.Duration(offset)
}
