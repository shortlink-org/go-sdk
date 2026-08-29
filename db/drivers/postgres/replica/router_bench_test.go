//go:build unit

package replica

import (
	"context"
	"fmt"
	"testing"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// The routing decision sits in front of every statement, so its cost is added
// to every query in the process. These benchmarks exist to keep it in the
// noise next to a network round trip — a Postgres query on the same host is
// tens of microseconds at best, so anything under a microsecond here is free.

// Statements the routing benchmarks run against.
var benchStatements = []struct {
	name string
	sql  string
}{
	{name: "select_short", sql: sqlSelectOne},
	{name: "select_join", sql: `SELECT u.id, u.name FROM users u JOIN orders o ON o.user_id = u.id WHERE u.id = $1 ORDER BY o.created_at DESC LIMIT 10`},
	{name: "insert_returning", sql: `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`},
	{name: "cte_write", sql: `WITH d AS (DELETE FROM sessions WHERE expires_at < now() RETURNING id) SELECT count(*) FROM d`},
	{name: "select_for_update", sql: `SELECT id FROM jobs WHERE state = 'ready' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`},
}

// BenchmarkRoute measures the whole decision: classify, read the context,
// consult the poller's cached sample, pick a pool. No I/O anywhere in it.
func BenchmarkRoute(b *testing.B) {
	cases := []struct {
		name string
		ctx  func(context.Context) context.Context
		sql  string
	}{
		{
			name: "read_to_replica",
			ctx:  WithTracker,
			sql:  benchStatements[1].sql,
		},
		{
			name: "read_after_write",
			ctx: func(ctx context.Context) context.Context {
				ctx = WithTracker(ctx)
				TrackerFromContext(ctx).Taint()

				return ctx
			},
			sql: benchStatements[1].sql,
		},
		{
			name: "read_with_watermark",
			ctx: func(ctx context.Context) context.Context {
				return WithWatermark(ctx, replicaReplayLSN-1)
			},
			sql: benchStatements[1].sql,
		},
		{
			name: "write_to_primary",
			ctx:  WithTracker,
			sql:  benchStatements[2].sql,
		},
		{
			name: "unscoped_read",
			ctx:  func(ctx context.Context) context.Context { return ctx },
			sql:  benchStatements[0].sql,
		},
	}

	router := newTestRouter(b, 3)

	for _, tt := range cases {
		b.Run(tt.name, func(b *testing.B) {
			ctx := tt.ctx(context.Background())

			b.ReportAllocs()

			for b.Loop() {
				_, _ = router.Route(ctx, tt.sql) //nolint:errcheck // the benchmark measures the call, not its result
			}
		})
	}
}

// BenchmarkRouteParallel checks that the decision does not serialize. Every
// statement in the process goes through it, so a lock here would show up as a
// throughput ceiling rather than as latency.
func BenchmarkRouteParallel(b *testing.B) {
	router := newTestRouter(b, 3)
	ctx := WithTracker(context.Background())
	sql := benchStatements[1].sql

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = router.Route(ctx, sql) //nolint:errcheck // the benchmark measures the call, not its result
		}
	})
}

// maxBenchReplicas is the widest fan-out the selection benchmark tries.
const maxBenchReplicas = 8

// BenchmarkGatePick isolates the replica selection from the classification, so
// that a regression can be attributed to one or the other.

func BenchmarkGatePick(b *testing.B) {
	for _, replicas := range []int{1, 3, maxBenchReplicas} {
		b.Run(fmt.Sprintf("replicas_%d", replicas), func(b *testing.B) {
			router := newTestRouter(b, replicas)

			b.ReportAllocs()

			for b.Loop() {
				_ = router.gate.pick(replicaReplayLSN)
			}
		})
	}
}

// BenchmarkTracker measures the per-statement bookkeeping. Both operations are
// on the hot path of every routed call.
func BenchmarkTracker(b *testing.B) {
	b.Run("observe", func(b *testing.B) {
		tracker := &Tracker{}

		b.ReportAllocs()

		for i := range b.N {
			tracker.Observe(wal.LSN(i))
		}
	})

	b.Run("tainted", func(b *testing.B) {
		tracker := &Tracker{}

		b.ReportAllocs()

		for b.Loop() {
			_ = tracker.Tainted()
		}
	})

	b.Run("from_context", func(b *testing.B) {
		ctx := WithTracker(context.Background())

		b.ReportAllocs()

		for b.Loop() {
			_ = TrackerFromContext(ctx)
		}
	})
}
