package replica

import (
	"context"
	"sync/atomic"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// Tracker is the write watermark of one unit of work — an HTTP request, a
// consumed message, a cron tick.
//
// It lives behind a pointer in the context, and a context value copies the
// pointer rather than the pointee — so a write made by one goroutine is
// immediately visible to every goroutine sharing that context. This is the standard Go
// idiom for request-scoped mutable state, and here it is also the desired
// semantics: if any branch of a fan-out wrote, the whole request must read its
// own write.
//
// Both fields are monotone — the taint is a latch, the watermark only ever
// rises — so plain atomics suffice and there is no lock to order.
//
// The zero value is usable, and every method is nil-safe so call sites do not
// have to branch on "was a tracker installed".
type Tracker struct {
	// watermark is the highest WAL position this context must observe.
	// an unknown position means a write happened whose position could not be
	// determined, which pins every subsequent read to the primary.
	watermark atomic.Uint64

	// tainted records that a write happened on this context, whether or not a
	// position is known.
	tainted atomic.Bool

	// parent is set by Fork. Observe propagates upward; taint stays local.
	parent *Tracker
}

type trackerKey struct{}

// WithTracker installs a fresh Tracker. Call it once per unit of work.
//
// Without a tracker, reads fall back to the router's NoTrackerPolicy, which
// defaults to the primary — correct, but it buys no replica traffic. The
// boundary middlewares install one for you.
func WithTracker(ctx context.Context) context.Context {
	return context.WithValue(ctx, trackerKey{}, &Tracker{})
}

// TrackerFromContext returns the Tracker in ctx, or nil.
func TrackerFromContext(ctx context.Context) *Tracker {
	tracker, ok := ctx.Value(trackerKey{}).(*Tracker)
	if !ok {
		return nil
	}

	return tracker
}

// WithWatermark installs a Tracker seeded with lsn and no taint. It is the
// receiving half of a cross-boundary handoff: the writer captured a position,
// the reader requires it, and nothing in between has to be tainted.
func WithWatermark(ctx context.Context, lsn wal.LSN) context.Context {
	tracker := &Tracker{}
	tracker.watermark.Store(uint64(lsn))

	return context.WithValue(ctx, trackerKey{}, tracker)
}

// Fork returns a context with a child Tracker seeded from the parent's current
// watermark.
//
// The asymmetry is deliberate. The child's Observe propagates upward, so the
// parent can still read what the child wrote; the child's taint stays local,
// because the point of forking is to say "this work's writes are not mine".
// Use it when a request spawns background work whose writes must not pin the
// request's own reads to the primary.
//
// The child is seeded by value: writes the parent makes after the fork are not
// seen by the child.
func Fork(ctx context.Context) context.Context {
	parent := TrackerFromContext(ctx)

	child := &Tracker{parent: parent}
	child.watermark.Store(uint64(parent.Watermark()))

	return context.WithValue(ctx, trackerKey{}, child)
}

// Taint marks the context as having written, without a known WAL position.
// Every subsequent read on this context goes to the primary.
func (t *Tracker) Taint() {
	if t == nil {
		return
	}

	t.tainted.Store(true)
}

// Tainted reports whether a write has happened on this context.
func (t *Tracker) Tainted() bool {
	return t != nil && t.tainted.Load()
}

// Watermark returns the highest WAL position this context is known to require.
func (t *Tracker) Watermark() wal.LSN {
	if t == nil {
		return 0
	}

	return wal.LSN(t.watermark.Load())
}

// Observe raises the watermark to lsn if lsn is higher, and propagates to the
// parent chain installed by Fork.
func (t *Tracker) Observe(lsn wal.LSN) {
	for node := t; node != nil; node = node.parent {
		for {
			current := node.watermark.Load()
			if uint64(lsn) <= current {
				break
			}

			if node.watermark.CompareAndSwap(current, uint64(lsn)) {
				break
			}
		}
	}
}
