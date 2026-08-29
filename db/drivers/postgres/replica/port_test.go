//go:build unit

package replica

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// The watermill module declares these two interfaces locally, so that it never
// imports db. Structural typing means nothing checks the two sides match —
// except this. If a signature here drifts, the wiring silently stops
// compiling in the consuming service instead of here, which is the wrong place
// to find out.
type (
	watermarkerContract interface {
		Capture(ctx context.Context) (string, error)
	}

	replicaGateContract interface {
		Await(ctx context.Context, token string, maxWait time.Duration) (bool, error)
		Apply(ctx context.Context, token string) context.Context
		Scope(ctx context.Context) context.Context
	}

	// The grpc module's consistency interceptors declare this one.
	grpcConsistencyContract interface {
		Capture(ctx context.Context) (string, error)
		Apply(ctx context.Context, token string) context.Context
		Observe(ctx context.Context, token string)
		Scope(ctx context.Context) context.Context
	}
)

var (
	_ watermarkerContract     = (*TextPort)(nil)
	_ replicaGateContract     = (*TextPort)(nil)
	_ grpcConsistencyContract = (*TextPort)(nil)
)

// Values for the watermark-store tables.
const (
	sampleSystemID = uint64(7482913740192837465)

	storeCapacity = 4
	storeLow      = wal.LSN(100)
	storeMid      = wal.LSN(150)
	storeHigh     = wal.LSN(200)
	storeEntries  = 200
	storeKey      = "alice"
)

func TestTextPortCaptureSkipsCleanContexts(t *testing.T) {
	t.Parallel()

	port := newTestRouter(t, 1).Port()

	// A context that never wrote has nothing to hand over, and must not pay a
	// round trip to find that out.
	token, err := port.Capture(WithTracker(context.Background()))
	require.NoError(t, err)
	assert.Empty(t, token)

	token, err = port.Capture(context.Background())
	require.NoError(t, err)
	assert.Empty(t, token)
}

func TestTextPortCaptureReportsFailureAfterAWrite(t *testing.T) {
	t.Parallel()

	ctx := WithTracker(context.Background())
	TrackerFromContext(ctx).Taint()

	_, err := (&Router{}).Port().Capture(ctx)
	require.Error(t, err)
}

func TestTextPortApply(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)
	router.gate.systemID = sampleSystemID
	router.gate.timeline = 1
	port := router.Port()

	token := wal.Token{SystemID: sampleSystemID, Timeline: 1, LSN: replicaReplayLSN, IssuedAt: time.Now()}

	ctx := port.Apply(context.Background(), token.String())
	assert.Equal(t, replicaReplayLSN, TrackerFromContext(ctx).Watermark())

	// A token we cannot read still means the producer wrote something.
	ctx = port.Apply(context.Background(), "not-a-token")
	assert.Equal(t, StrategyPrimary, StrategyFromContext(ctx))

	// So does one from another timeline.
	foreign := wal.Token{SystemID: sampleSystemID, Timeline: 2, LSN: replicaReplayLSN, IssuedAt: time.Now()}
	ctx = port.Apply(context.Background(), foreign.String())
	assert.Equal(t, StrategyPrimary, StrategyFromContext(ctx))
}

func TestTextPortAwait(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)
	router.gate.systemID = sampleSystemID
	router.gate.timeline = 1
	port := router.Port()

	satisfied := wal.Token{SystemID: sampleSystemID, Timeline: 1, LSN: replicaReplayLSN, IssuedAt: time.Now()}
	ready, err := port.Await(context.Background(), satisfied.String(), 10*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, ready)

	// A malformed token is not something to wait for. Apply pins the read to
	// the primary instead, which is both correct and immediate.
	ready, err = port.Await(context.Background(), "garbage", 10*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestMemoryWatermarks(t *testing.T) {
	t.Parallel()

	store := NewMemoryWatermarks(time.Minute, storeCapacity)
	ctx := context.Background()

	_, found, err := store.Get(ctx, storeKey)
	require.NoError(t, err)
	assert.False(t, found)

	require.NoError(t, store.Set(ctx, storeKey, storeLow))

	position, found, err := store.Get(ctx, storeKey)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, storeLow, position)

	// Two concurrent writes by one actor can finish out of order. The earlier
	// one must not overwrite the later one, or the next read is gated on a
	// position that has already been passed.
	require.NoError(t, store.Set(ctx, storeKey, storeHigh))
	require.NoError(t, store.Set(ctx, storeKey, storeMid))

	position, _, err = store.Get(ctx, storeKey)
	require.NoError(t, err)
	assert.Equal(t, storeHigh, position)
}

func TestMemoryWatermarksExpire(t *testing.T) {
	t.Parallel()

	store := NewMemoryWatermarks(time.Millisecond, storeCapacity)
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, storeKey, storeLow))

	assert.Eventually(t, func() bool {
		_, found, _ := store.Get(ctx, storeKey) //nolint:errcheck // the memory store never errors

		return !found
	}, time.Second, 5*time.Millisecond)
}

// TestMemoryWatermarksAreBounded: the keys come from request data, so an
// unbounded map is a memory leak somebody else can drive.
func TestMemoryWatermarksAreBounded(t *testing.T) {
	t.Parallel()

	const maxEntries = 8

	store := NewMemoryWatermarks(time.Minute, maxEntries)
	ctx := context.Background()

	for i := range storeEntries {
		require.NoError(t, store.Set(ctx, string(rune('a'+i%128))+string(rune(i)), wal.LSN(i)))
	}

	assert.LessOrEqual(t, store.Len(), maxEntries)
}
