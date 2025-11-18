package http_client

import (
	"context"
	"testing"
	"time"

	"github.com/shortlink-org/go-sdk/http/client/internal/types"
	"github.com/stretchr/testify/require"
)

func TestNewTokenBucketLimiter_InvalidConfig(t *testing.T) {
	_, err := NewTokenBucketLimiter(0, 10, 0)
	require.Error(t, err)
	require.Equal(t, types.ErrInvalidLimiterConfig, err)

	_, err = NewTokenBucketLimiter(10, 0, 0)
	require.Error(t, err)
	require.Equal(t, types.ErrInvalidLimiterConfig, err)

	_, err = NewTokenBucketLimiter(-1, 10, 0)
	require.Error(t, err)
	require.Equal(t, types.ErrInvalidLimiterConfig, err)

	_, err = NewTokenBucketLimiter(10, -1, 0)
	require.Error(t, err)
	require.Equal(t, types.ErrInvalidLimiterConfig, err)
}

func TestNewTokenBucketLimiter_JitterNormalization(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(10, 10, -0.5)
	require.NoError(t, err)
	require.NotNil(t, limiter)

	limiter2, err := NewTokenBucketLimiter(10, 10, 1.5)
	require.NoError(t, err)
	require.NotNil(t, limiter2)
}

func TestTokenBucketLimiter_Wait_Immediate(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(10, 10, 0)
	require.NoError(t, err)

	ctx := context.Background()
	wait, err := limiter.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), wait)
}

func TestTokenBucketLimiter_Wait_ContextCancellation(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(0.1, 1, 0) // very slow rate
	require.NoError(t, err)

	// Consume the token
	_, err = limiter.Wait(context.Background())
	require.NoError(t, err)

	// Now we'll have to wait, but cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	wait, err := limiter.Wait(ctx)
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
	require.GreaterOrEqual(t, wait, time.Duration(0))
}

func TestTokenBucketLimiter_Wait_ContextTimeout(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(0.1, 1, 0) // very slow rate
	require.NoError(t, err)

	// Consume the token
	_, err = limiter.Wait(context.Background())
	require.NoError(t, err)

	// Now we'll have to wait, but timeout the context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	wait, err := limiter.Wait(ctx)
	require.Error(t, err)
	require.Equal(t, context.DeadlineExceeded, err)
	require.GreaterOrEqual(t, wait, time.Duration(0))
}

func TestTokenBucketLimiter_Refill(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(10, 5, 0) // 10 tokens/sec, burst 5
	require.NoError(t, err)

	// Consume all tokens
	for i := 0; i < 5; i++ {
		_, err = limiter.Wait(context.Background())
		require.NoError(t, err)
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond) // should refill ~1.5 tokens

	// Should be able to get at least one token
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	wait, err := limiter.Wait(ctx)
	require.NoError(t, err)
	require.Less(t, wait, 100*time.Millisecond)
}

func TestTokenBucketLimiter_Jitter(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(1, 1, 0.1) // 10% jitter
	require.NoError(t, err)

	// Consume the token
	_, err = limiter.Wait(context.Background())
	require.NoError(t, err)

	// Wait should have some jitter applied
	// Base wait should be ~1 second, jitter can add up to 10% = 100ms
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wait1, err1 := limiter.Wait(ctx)
	require.NoError(t, err1)

	// Wait time should be between base (1s) and base + jitter (1.1s)
	// but could be slightly more due to timing
	baseWait := 1 * time.Second
	maxWait := 1200 * time.Millisecond              // 1.2 seconds
	require.GreaterOrEqual(t, wait1, baseWait*9/10) // at least 90% of base (allowing negative jitter)
	require.LessOrEqual(t, wait1, maxWait)          // at most 20% more than base
}

func TestTokenBucketLimiter_BurstLimit(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(10, 3, 0) // 10 tokens/sec, burst 3
	require.NoError(t, err)

	// Consume burst
	for i := 0; i < 3; i++ {
		_, err = limiter.Wait(context.Background())
		require.NoError(t, err)
	}

	// Next wait should take time even though rate is high
	// With 10 tokens/sec, we need 0.1s (100ms) to refill 1 token
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	wait, err := limiter.Wait(ctx)
	// Should timeout because we need to wait for refill (~100ms) but timeout is 80ms
	require.Error(t, err)
	require.Equal(t, context.DeadlineExceeded, err)
	// Wait time accumulates during the actual wait and may be close to timeout
	// It should be less than the required refill time (~100ms)
	require.Less(t, wait, 110*time.Millisecond)   // should be less than refill time
	require.Greater(t, wait, 50*time.Millisecond) // but more than half timeout
}

func TestTokenBucketLimiter_ConcurrentAccess(t *testing.T) {
	limiter, err := NewTokenBucketLimiter(100, 10, 0)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan error, 10)

	// Launch 10 goroutines
	for i := 0; i < 10; i++ {
		go func() {
			_, err := limiter.Wait(ctx)
			done <- err
		}()
	}

	// All should succeed
	for i := 0; i < 10; i++ {
		err := <-done
		require.NoError(t, err)
	}
}
