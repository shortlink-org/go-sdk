//go:build unit

package replica

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// Positions used across the tracker tables.
const (
	lowPosition    = wal.LSN(50)
	basePosition   = wal.LSN(100)
	higherPosition = wal.LSN(200)
	childPosition  = wal.LSN(300)
	handedOver     = wal.LSN(4096)
)

// unrelatedKey stands in for some other package's context key. An anonymous
// empty struct would collide with every other package that did the same.
type unrelatedKey struct{}

func TestTrackerNilSafe(t *testing.T) {
	t.Parallel()

	var tracker *Tracker

	assert.NotPanics(t, func() {
		tracker.Taint()
		tracker.Observe(wal.LSN(42))
	})
	assert.False(t, tracker.Tainted())
	assert.Equal(t, wal.LSN(0), tracker.Watermark())

	// A context with no tracker must behave the same way.
	assert.Nil(t, TrackerFromContext(context.Background()))
}

func TestTrackerTaintIsALatch(t *testing.T) {
	t.Parallel()

	tracker := &Tracker{}
	assert.False(t, tracker.Tainted())

	tracker.Taint()
	assert.True(t, tracker.Tainted())

	tracker.Taint()
	assert.True(t, tracker.Tainted())
}

func TestTrackerWatermarkIsMonotone(t *testing.T) {
	t.Parallel()

	tracker := &Tracker{}

	tracker.Observe(basePosition)
	assert.Equal(t, basePosition, tracker.Watermark())

	tracker.Observe(lowPosition)
	assert.Equal(t, basePosition, tracker.Watermark(), "a lower position must not lower the watermark")

	tracker.Observe(higherPosition)
	assert.Equal(t, higherPosition, tracker.Watermark())

	tracker.Observe(wal.Unknown)
	assert.Equal(t, wal.Unknown, tracker.Watermark(), "an unknown position outranks every real one")
}

// TestTrackerSharedThroughContext is the property the whole in-request scheme
// rests on: the context copies the pointer, so a write seen by one goroutine
// is seen by all of them.
func TestTrackerSharedThroughContext(t *testing.T) {
	t.Parallel()

	ctx := WithTracker(context.Background())

	derived := context.WithValue(ctx, unrelatedKey{}, "unrelated")
	TrackerFromContext(derived).Taint()

	assert.True(t, TrackerFromContext(ctx).Tainted(), "taint must be visible through the original context")
}

func TestWithWatermark(t *testing.T) {
	t.Parallel()

	ctx := WithWatermark(context.Background(), handedOver)
	tracker := TrackerFromContext(ctx)

	require.NotNil(t, tracker)
	assert.Equal(t, handedOver, tracker.Watermark())
	assert.False(t, tracker.Tainted(), "a handed-over watermark is a requirement, not a write")
}

func TestForkKeepsTaintLocalAndPropagatesWatermark(t *testing.T) {
	t.Parallel()

	parentCtx := WithTracker(context.Background())
	parent := TrackerFromContext(parentCtx)
	parent.Observe(basePosition)

	childCtx := Fork(parentCtx)
	child := TrackerFromContext(childCtx)

	assert.Equal(t, basePosition, child.Watermark(), "the child inherits what the parent already required")

	child.Taint()
	assert.True(t, child.Tainted())
	assert.False(t, parent.Tainted(), "the child's writes are not the parent's")

	child.Observe(childPosition)
	assert.Equal(t, childPosition, parent.Watermark(), "the parent must still be able to read what the child wrote")
}

func TestForkWithoutParentTracker(t *testing.T) {
	t.Parallel()

	ctx := Fork(context.Background())
	tracker := TrackerFromContext(ctx)

	require.NotNil(t, tracker)
	assert.Equal(t, wal.LSN(0), tracker.Watermark())
}

// TestTrackerConcurrent is the reason both fields are atomics. Run under -race.
func TestTrackerConcurrent(t *testing.T) {
	t.Parallel()

	const goroutines = 64

	tracker := &Tracker{}

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := range goroutines {
		go func() {
			defer wg.Done()

			tracker.Observe(wal.LSN(i)) //nolint:gosec // i is a small loop counter
			tracker.Taint()
			_ = tracker.Watermark()
			_ = tracker.Tainted()
		}()
	}

	wg.Wait()

	assert.Equal(t, wal.LSN(goroutines-1), tracker.Watermark())
	assert.True(t, tracker.Tainted())
}

func TestStrategyContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	assert.Equal(t, StrategyUnset, StrategyFromContext(ctx))

	assert.Equal(t, StrategyStaleRead, StrategyFromContext(Stale(ctx)))
	assert.Equal(t, StrategyPrimary, StrategyFromContext(OnPrimary(ctx)))
	assert.Equal(t, StrategyReplica, StrategyFromContext(OnReplica(ctx)))
	assert.Equal(t, StrategyReadAfterWrite, StrategyFromContext(WithStrategy(ctx, StrategyReadAfterWrite)))
}

func TestStrategyString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unset", StrategyUnset.String())
	assert.Equal(t, "read_after_write", StrategyReadAfterWrite.String())
	assert.Equal(t, "stale_read", StrategyStaleRead.String())
	assert.Equal(t, "primary", StrategyPrimary.String())
	assert.Equal(t, "replica", StrategyReplica.String())

	assert.Equal(t, "primary", NoTrackerPrimary.String())
	assert.Equal(t, "replica", NoTrackerReplica.String())
}
