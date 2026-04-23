// Package rate_limiter implements a token-bucket style limiter driven by a time ticker.
package rate_limiter

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRateLimiterCanceled is returned when Wait is unblocked by context cancellation.
var ErrRateLimiterCanceled = errors.New("rate limiter context canceled")

// RateLimiter limits work to a maximum of limit acquisitions per interval.
type RateLimiter struct {
	mu   sync.Mutex
	done chan struct{}

	limiter chan struct{}
	ticker  *time.Ticker
	limit   int64
}

// New starts a rate limiter that allows up to limit acquisitions per interval.
func New(ctx context.Context, limit int64, interval time.Duration) (*RateLimiter, error) {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	rateLimiter := &RateLimiter{
		mu:      sync.Mutex{},
		limiter: make(chan struct{}, limit),
		ticker:  ticker,
		limit:   limit,
		done:    done,
	}

	go rateLimiter.refill()

	// Graceful shutdown: when the context is canceled, signal via the done channel.
	go func() {
		<-ctx.Done()
		close(done)
		ticker.Stop()
		close(rateLimiter.limiter)
	}()

	return rateLimiter, nil
}

// Wait blocks until a token is available or the limiter is stopped.
func (r *RateLimiter) Wait() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-r.limiter:
		return nil
	case <-r.done:
		return ErrRateLimiterCanceled
	}
}

// refill refills tokens periodically.
func (r *RateLimiter) refill() {
	for {
		select {
		case <-r.ticker.C:
			// Refill up to 'limit' tokens.
			for range r.limit {
				select {
				case r.limiter <- struct{}{}:
				default:
				}
			}
		case <-r.done:
			r.ticker.Stop()

			return
		}
	}
}
