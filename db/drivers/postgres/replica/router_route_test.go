//go:build unit

package replica

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// replicaReplayLSN is what every fake replica reports as replayed. Watermarks
// below it are satisfied; above it are not.
// Statements reused across the tables.
const (
	sqlSelectOne = `SELECT 1`
	sqlUpdate    = `UPDATE t SET a = 1`
)

const replicaReplayLSN = wal.LSN(1000)

// Timings for the wait tests, named so the intent is on the page rather than
// in a bare number.
const (
	sampleDelay   = 20 * time.Millisecond
	shortDeadline = 20 * time.Millisecond
	spreadRounds  = 30
	loopRounds    = 20
	testSystemID  = uint64(42)
	nextTimeline  = uint32(4)
	awaitSlack    = 20 * time.Millisecond

	liveTimeline = uint32(3)

	// The lag budget sits far below the lag the test then reports, so it is the
	// budget that excludes the replica rather than anything else.
	tinyLagBudget = int64(16)
	reportedLag   = int64(1) << 20
)

// newTestRouter builds a router over fake replicas whose health samples are
// set directly, so the whole decision table can be exercised without a server.
func newTestRouter(tb testing.TB, replicas int, tune ...func(*Options)) *Router {
	tb.Helper()

	opts := DefaultOptions()
	for _, fn := range tune {
		fn(&opts)
	}

	instruments, err := metrics.New(nil)
	require.NoError(tb, err)

	nodes := make([]*replicaNode, 0, replicas)

	for index := range replicas {
		nodes = append(nodes, &replicaNode{
			host:  fmt.Sprintf("replica-%d:5432", index),
			index: index,
			state: replicaState{
				lifecycle: replicaStandby,
				replayLSN: replicaReplayLSN,
				lastPoll:  time.Now(),
			},
		})
	}

	gate := newGate(nil, nodes, &opts, nil, instruments)
	gate.primaryLSN = replicaReplayLSN
	gate.primaryAt = time.Now()
	gate.timeline = 1

	return &Router{gate: gate, metrics: instruments, opts: opts}
}

//nolint:funlen,maintidx // one table, one case per branch of the routing decision
func TestRouterRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		replicas    int
		tune        func(*Options)
		ctx         func(context.Context) context.Context
		sql         string
		wantPrimary bool
		wantReason  string
		wantErr     error
	}{
		{
			name:        "write goes to the primary",
			replicas:    1,
			ctx:         WithTracker,
			sql:         sqlUpdate,
			wantPrimary: true,
			wantReason:  metrics.ReasonWrite,
		},
		{
			name:        "unclassifiable goes to the primary",
			replicas:    1,
			ctx:         WithTracker,
			sql:         `SELECT 1; SELECT 2`,
			wantPrimary: true,
			wantReason:  metrics.ReasonUnknown,
		},
		{
			name:        "read with a clean tracker goes to a replica",
			replicas:    1,
			ctx:         WithTracker,
			sql:         sqlSelectOne,
			wantPrimary: false,
			wantReason:  metrics.ReasonWithinLag,
		},
		{
			name:        "unscoped read goes to the primary by default",
			replicas:    1,
			ctx:         func(ctx context.Context) context.Context { return ctx },
			sql:         sqlSelectOne,
			wantPrimary: true,
			wantReason:  metrics.ReasonNoTracker,
		},
		{
			name:        "unscoped read goes to a replica when the policy says so",
			replicas:    1,
			tune:        func(o *Options) { o.NoTracker = NoTrackerReplica },
			ctx:         func(ctx context.Context) context.Context { return ctx },
			sql:         sqlSelectOne,
			wantPrimary: false,
			wantReason:  metrics.ReasonWithinLag,
		},
		{
			name:     "a tainted context reads from the primary",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				ctx = WithTracker(ctx)
				TrackerFromContext(ctx).Taint()

				return ctx
			},
			sql:         sqlSelectOne,
			wantPrimary: true,
			wantReason:  metrics.ReasonTainted,
		},
		{
			name:     "a satisfied watermark reads from a replica",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				return WithWatermark(ctx, replicaReplayLSN-1)
			},
			sql:         sqlSelectOne,
			wantPrimary: false,
			wantReason:  metrics.ReasonCaughtUp,
		},
		{
			name:     "an exactly met watermark reads from a replica",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				return WithWatermark(ctx, replicaReplayLSN)
			},
			sql:         sqlSelectOne,
			wantPrimary: false,
			wantReason:  metrics.ReasonCaughtUp,
		},
		{
			name:     "an unmet watermark reads from the primary",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				return WithWatermark(ctx, replicaReplayLSN+1)
			},
			sql:         sqlSelectOne,
			wantPrimary: true,
			wantReason:  metrics.ReasonBehind,
		},
		{
			name:     "an unknown watermark reads from the primary",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				return WithWatermark(ctx, wal.Unknown)
			},
			sql:         sqlSelectOne,
			wantPrimary: true,
			wantReason:  metrics.ReasonBehind,
		},
		{
			name:        "an explicit primary strategy wins over everything",
			replicas:    1,
			ctx:         func(ctx context.Context) context.Context { return OnPrimary(WithTracker(ctx)) },
			sql:         sqlSelectOne,
			wantPrimary: true,
			wantReason:  metrics.ReasonExplicit,
		},
		{
			name:     "a stale read ignores the taint",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				ctx = WithTracker(ctx)
				TrackerFromContext(ctx).Taint()

				return Stale(ctx)
			},
			sql:         sqlSelectOne,
			wantPrimary: false,
			wantReason:  metrics.ReasonWithinLag,
		},
		{
			name:     "a stale read ignores an unmet watermark",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				return Stale(WithWatermark(ctx, replicaReplayLSN+1))
			},
			sql:         sqlSelectOne,
			wantPrimary: false,
			wantReason:  metrics.ReasonWithinLag,
		},
		{
			name:        "no replicas configured reads from the primary",
			replicas:    0,
			ctx:         WithTracker,
			sql:         sqlSelectOne,
			wantPrimary: true,
			wantReason:  metrics.ReasonRouterDisabled,
		},
		{
			name:     "a write under a replica-only strategy is refused",
			replicas: 1,
			ctx:      func(ctx context.Context) context.Context { return OnReplica(WithTracker(ctx)) },
			sql:      sqlUpdate,
			wantErr:  ErrWriteOnReplica,
		},
		{
			name:     "a replica-only read with no healthy replica is refused",
			replicas: 0,
			ctx:      func(ctx context.Context) context.Context { return OnReplica(WithTracker(ctx)) },
			sql:      `SELECT 1`,
			wantErr:  ErrRouterDisabled,
		},
		{
			name:     "a replica-only read behind its watermark is refused",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				return OnReplica(WithWatermark(ctx, replicaReplayLSN+1))
			},
			sql:     sqlSelectOne,
			wantErr: ErrNoHealthyReplica,
		},
		{
			name:     "a tainted replica-only read is refused",
			replicas: 1,
			ctx: func(ctx context.Context) context.Context {
				ctx = WithTracker(ctx)
				TrackerFromContext(ctx).Taint()

				return OnReplica(ctx)
			},
			sql:     sqlSelectOne,
			wantErr: ErrNoHealthyReplica,
		},
		{
			name:     "fallback disabled turns a lagging replica into an error",
			replicas: 1,
			tune:     func(o *Options) { o.Fallback = FallbackReject },
			ctx: func(ctx context.Context) context.Context {
				return WithWatermark(ctx, replicaReplayLSN+1)
			},
			sql:     sqlSelectOne,
			wantErr: ErrNoHealthyReplica,
		},
		{
			name:        "a routing hint overrides the classifier",
			replicas:    1,
			ctx:         WithTracker,
			sql:         `/* route:read */ SELECT * FROM t FOR UPDATE`,
			wantPrimary: false,
			wantReason:  metrics.ReasonWithinLag,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tunes := []func(*Options){}
			if tt.tune != nil {
				tunes = append(tunes, tt.tune)
			}

			router := newTestRouter(t, tt.replicas, tunes...)

			target, err := router.Route(tt.ctx(context.Background()), tt.sql)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPrimary, target.OnPrimary(), "reason: %s", target.Reason())
			assert.Equal(t, tt.wantReason, target.Reason())
			if tt.wantPrimary {
				assert.Nil(t, target.Replica())
			} else {
				require.NotNil(t, target.Replica())
				assert.GreaterOrEqual(t, target.Replica().Index, 0)
			}
		})
	}
}

// TestRouterRouteTaintsOnWrite is the in-request half of read-your-writes: one
// write, and everything after it on that context stays on the primary.
func TestRouterRouteTaintsOnWrite(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)
	ctx := WithTracker(context.Background())

	read, err := router.Route(ctx, sqlSelectOne)
	require.NoError(t, err)
	require.False(t, read.OnPrimary(), "a clean context should reach the replica")

	_, err = router.Route(ctx, `INSERT INTO t VALUES (1)`)
	require.NoError(t, err)

	after, err := router.Route(ctx, sqlSelectOne)
	require.NoError(t, err)
	assert.True(t, after.OnPrimary(), "a read after a write must not go to a replica")
	assert.Equal(t, metrics.ReasonTainted, after.Reason())
}

func TestGateEligibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		set  func(*replicaNode)
		want bool
	}{
		{name: "healthy", set: func(*replicaNode) {}, want: true},
		{name: "quarantined", set: func(n *replicaNode) { n.state.lifecycle = replicaQuarantined }},
		{name: "never polled", set: func(n *replicaNode) { n.state.lastPoll = time.Time{} }},
		{name: "consecutive failures", set: func(n *replicaNode) { n.state.failures = failureThreshold }},
		{name: "left recovery", set: func(n *replicaNode) { n.state.lifecycle = replicaUnobserved }},
		{name: "stale sample", set: func(n *replicaNode) { n.state.lastPoll = time.Now().Add(-time.Hour) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			router := newTestRouter(t, 1)
			tt.set(router.gate.replicas[0])

			assert.Equal(t, tt.want, router.gate.eligible(router.gate.replicas[0]))
		})
	}
}

func TestReplicaQuarantineIsIrreversible(t *testing.T) {
	t.Parallel()

	state := replicaState{lifecycle: replicaStandby, lastPoll: time.Now()}
	state.recordPromotion(time.Now())
	state.recordStandby(replicaReplayLSN, replicaReplayLSN, 0, time.Now())

	assert.True(t, state.quarantined())
	assert.False(t, state.inRecovery())
	assert.ErrorIs(t, state.lastErr, ErrReplicaPromoted)
}

// TestGateLagThresholdOnlyAppliesWithoutAWatermark: comparing WAL positions is
// strictly stronger than comparing lag, so a satisfied watermark must not be
// overruled by a byte budget.
func TestGateLagThresholdOnlyAppliesWithoutAWatermark(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1, func(o *Options) { o.MaxLagBytes = tinyLagBudget })
	router.gate.replicas[0].state.lagBytes = reportedLag

	selection := router.gate.pick(0)
	assert.False(t, selection.available(), "a read with no watermark is subject to the lag budget")
	assert.Equal(t, metrics.ReasonBehind, selection.reason())

	selection = router.gate.pick(replicaReplayLSN)
	assert.True(t, selection.available(), "a satisfied watermark outranks the lag budget")
	assert.Equal(t, metrics.ReasonCaughtUp, selection.reason())
}

func TestGateUnknownLagDoesNotBypassTheLagBudget(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)
	router.gate.replicas[0].state.lagBytes = unknownLagBytes

	selection := router.gate.pick(0)
	assert.False(t, selection.available(), "unknown lag must not look like zero lag")
	assert.Equal(t, metrics.ReasonBehind, selection.reason())

	selection = router.gate.pick(replicaReplayLSN)
	assert.True(t, selection.available(), "a concrete satisfied watermark is stronger than a lag sample")
	assert.Equal(t, metrics.ReasonCaughtUp, selection.reason())
}

// TestGatePickSpreadsAcrossReplicas: without this, every read lands on the
// first healthy replica and the others idle.
func TestGatePickSpreadsAcrossReplicas(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 3)
	seen := map[int]bool{}

	for range spreadRounds {
		selection := router.gate.pick(0)
		require.True(t, selection.available())
		seen[selection.node.index] = true
	}

	assert.Len(t, seen, 3)
}

func TestGatePickSkipsUnhealthyReplicas(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 3)
	router.gate.replicas[0].state.lifecycle = replicaQuarantined
	router.gate.replicas[1].state.lifecycle = replicaUnobserved

	for range spreadRounds / 3 {
		selection := router.gate.pick(0)
		require.True(t, selection.available())
		require.Equal(t, 2, selection.node.index)
		require.Equal(t, metrics.ReasonWithinLag, selection.reason())
	}
}

func TestReplicaEligible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options pgx.TxOptions
		want    bool
	}{
		{
			name:    "read only",
			options: pgx.TxOptions{AccessMode: pgx.ReadOnly},
			want:    true,
		},
		{
			name:    "read only repeatable read",
			options: pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead},
			want:    true,
		},
		{
			name:    "default options are read write",
			options: pgx.TxOptions{},
		},
		{
			name:    "explicitly read write",
			options: pgx.TxOptions{AccessMode: pgx.ReadWrite},
		},
		{
			// pgx emits BeginQuery verbatim and ignores AccessMode when it is
			// set, so this inspects as read-only and is not.
			name:    "custom begin query defeats the access mode",
			options: pgx.TxOptions{AccessMode: pgx.ReadOnly, BeginQuery: "BEGIN"},
		},
		{
			name:    "custom commit query",
			options: pgx.TxOptions{AccessMode: pgx.ReadOnly, CommitQuery: "COMMIT"},
		},
		{
			// A hot standby refuses serializable outright.
			name:    "serializable is unavailable on a standby",
			options: pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.Serializable},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, replicaEligible(tt.options))
		})
	}
}

func TestRouterResolveTokenDistinguishesForeignAndForgedTokens(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)
	router.gate.systemID = testSystemID
	router.gate.timeline = liveTimeline

	valid := wal.Token{SystemID: testSystemID, Timeline: liveTimeline, LSN: replicaReplayLSN - 1}
	resolution := router.ResolveToken(valid)
	require.Equal(t, TokenAccepted, resolution.State())
	assert.Equal(t, valid.LSN, resolution.Position())

	resolution = router.ResolveToken(wal.Token{SystemID: testSystemID, Timeline: nextTimeline, LSN: 1})
	assert.Equal(t, TokenUnusable, resolution.State(), "a token from another timeline is not comparable")

	resolution = router.ResolveToken(wal.Token{SystemID: 99, Timeline: liveTimeline, LSN: 1})
	assert.Equal(t, TokenUnusable, resolution.State(), "a token from another cluster is not comparable")

	// A position beyond what the primary is known to have written is clamped,
	// not refused: refusing would drop the guarantee entirely, which is the
	// stale read the token exists to prevent.
	resolution = router.ResolveToken(wal.Token{SystemID: testSystemID, Timeline: liveTimeline, LSN: wal.Unknown})
	require.Equal(t, TokenAccepted, resolution.State())
	assert.Equal(t, router.gate.primaryLSN, resolution.Position(), "a forged position is clamped to the primary")

	assert.Equal(t, TokenAbsent, router.ResolveToken(wal.Token{}).State())
}

func TestRouterResolveTokenPinsUntilLineageIsKnown(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)
	router.gate.timeline = 0

	token := wal.Token{SystemID: testSystemID, Timeline: liveTimeline, LSN: replicaReplayLSN}
	resolution := router.ResolveToken(token)
	assert.Equal(t, TokenUnusable, resolution.State(), "a bare LSN is not comparable before the first primary probe")

	ctx := router.WithToken(context.Background(), token)
	assert.Equal(t, StrategyPrimary, StrategyFromContext(ctx))
}

// TestRouterWithTokenPinsOnAnUninterpretableToken: a token we cannot compare
// still says "this reader has seen a write". Dropping it would serve the stale
// read the token exists to prevent.
func TestRouterWithTokenPinsOnAnUninterpretableToken(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)
	router.gate.systemID = testSystemID
	router.gate.timeline = liveTimeline

	foreign := wal.Token{SystemID: testSystemID, Timeline: nextTimeline, LSN: 1}

	ctx := router.WithToken(context.Background(), foreign)
	assert.Equal(t, StrategyPrimary, StrategyFromContext(ctx))

	target, err := router.Route(ctx, sqlSelectOne)
	require.NoError(t, err)
	assert.True(t, target.OnPrimary())

	// A token we can compare installs the watermark instead.
	ctx = router.WithToken(context.Background(), wal.Token{SystemID: testSystemID, Timeline: liveTimeline, LSN: replicaReplayLSN})
	assert.Equal(t, replicaReplayLSN, TrackerFromContext(ctx).Watermark())

	// No token at all leaves the context alone.
	assert.Equal(t, context.Background(), router.WithToken(context.Background(), wal.Token{}))
}

func TestRouterAwaitReturnsImmediatelyWhenReady(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1)

	ready, err := router.Await(context.Background(), replicaReplayLSN)
	require.NoError(t, err)
	assert.True(t, ready)
}

// TestRouterAwaitWakesOnANewSample checks that a waiter is released by the
// poller's broadcast rather than by its own timeout.
func TestRouterAwaitWakesOnANewSample(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1, func(o *Options) { o.GateMaxWait = 5 * time.Second })
	required := replicaReplayLSN + 100

	go func() {
		time.Sleep(sampleDelay)

		node := router.gate.replicas[0]
		node.mu.Lock()
		node.state.recordStandby(required, 0, 0, time.Now())
		node.mu.Unlock()

		router.gate.broadcast()
	}()

	started := time.Now()
	ready, err := router.Await(context.Background(), required)

	require.NoError(t, err)
	assert.True(t, ready)
	assert.Less(t, time.Since(started), time.Second, "the waiter should wake on the sample, not on its deadline")
}

func TestRouterAwaitGivesUpAtTheDeadline(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1, func(o *Options) { o.GateMaxWait = shortDeadline })

	ready, err := router.Await(context.Background(), replicaReplayLSN+1)
	require.NoError(t, err)
	assert.False(t, ready)
}

func TestRouterAwaitHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 1, func(o *Options) { o.GateMaxWait = time.Minute })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := router.Await(ctx, replicaReplayLSN+1)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRouterDisabledAwaitIsANoOp(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t, 0)

	ready, err := router.Await(context.Background(), wal.Unknown)
	require.NoError(t, err)
	assert.True(t, ready, "with no replicas every read is already on the primary")
}
