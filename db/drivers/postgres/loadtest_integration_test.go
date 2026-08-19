//go:build database && postgres && replica

package postgres

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
)

// This file answers the question the microbenchmarks cannot: what does routing
// buy? Nothing in latency — a replica read is not faster and crosses one more
// hop. What it buys is capacity on the primary.
//
// Two things about the design, both learned the hard way.
//
// The load is rate-limited rather than open-loop. Driving both modes to
// saturation compares different amounts of offered work and produces latency
// figures that are queuing artifacts; at a fixed rate the question becomes the
// right one — for the same work, how much of it did the primary do?
//
// Each node is capped at one CPU. Uncapped, the primary and the standby draw
// from the same cores, so moving a read from one to the other changes which
// container burns a core and adds nothing. The cap models what this feature is
// for: two machines.
//
// What the test deliberately does not claim is a throughput win. With a single
// replica and a read-heavy mix, routing relocates the bottleneck rather than
// removing it — you need several replicas for that, and this harness is not the
// place to prove it.

const (
	// nodeCPUs caps each node, so that one container models one machine.
	nodeCPUs = 1.0

	loadRows = 200_000

	// loadRate is the offered rate, identical in both modes and comfortably
	// below what one CPU can serve, so the measurement is of work done rather
	// than of a queue.
	loadRate = 250

	loadWorkers  = 16
	loadDuration = 6 * time.Second

	// loadOffloadTolerance is how far below the arithmetic ceiling the
	// measured offload may fall before the routing is considered broken. The
	// slack absorbs the health poller's own traffic, which lands on the
	// replica in both modes.
	loadOffloadTolerance = 0.8
)

// loadReadSQL is a bounded range scan: real work, but cheap enough that a rate
// of a few hundred per second stays below saturation.
const loadReadSQL = `SELECT count(*), sum(length(payload)) FROM load_probe WHERE id BETWEEN $1 AND $1 + 400`

const loadWriteSQL = `INSERT INTO load_probe (payload) VALUES ($1)`

// serverStatsSQL reads the node's own transaction counter. Counting in the
// harness alone would only prove the harness agrees with itself.
const serverStatsSQL = `SELECT xact_commit + xact_rollback FROM pg_stat_database WHERE datname = current_database()`

type loadResult struct {
	mode        string
	readShare   float64
	operations  int64
	primaryTxns int64
	replicaTxns int64
	readP50     time.Duration
	readP99     time.Duration
}

func (r loadResult) offload() float64 {
	total := r.primaryTxns + r.replicaTxns
	if total == 0 {
		return 0
	}

	return float64(r.replicaTxns) / float64(total)
}

func TestReplicaLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := startClusterWithCPU(ctx, t, nodeCPUs)
	_, router := newRoutedStore(ctx, t, c)

	seedLoadTable(ctx, t, router)

	shares := []float64{0.9, 0.7, 0.5}
	results := make([]loadResult, 0, len(shares)*2)

	for _, share := range shares {
		results = append(results,
			runLoad(ctx, t, router, "baseline", share),
			runLoad(ctx, t, router, "routed", share),
		)
	}

	reportLoad(t, results)

	for i := 0; i < len(results); i += 2 {
		baseline, routed := results[i], results[i+1]

		t.Run(fmt.Sprintf("reads_%.0f", routed.readShare*100), func(t *testing.T) {
			// Both modes were offered the same work, so the comparison is of
			// where it ran rather than of how much there was.
			assert.InDelta(t, baseline.operations, routed.operations, float64(baseline.operations)*0.15,
				"the two modes must be offered comparable work to be comparable")

			assert.Less(t, routed.primaryTxns, baseline.primaryTxns,
				"routing must move transactions off the primary")

			// The ceiling is arithmetic, not aspirational: every unit does one
			// read, a (1-r) share of units also writes, and those units' reads
			// are tainted back onto the primary. So the primary keeps 2(1-r)
			// transactions out of (2-r), and the most that can ever leave is
			// r/(2-r) — at a 50% read mix that is 33%, not 50%.
			ceiling := routed.readShare / (2 - routed.readShare)

			assert.Greater(t, routed.offload(), ceiling*loadOffloadTolerance,
				"%.1f%% of transactions left the primary, against a ceiling of %.1f%%",
				routed.offload()*100, ceiling*100)
		})
	}
}

func seedLoadTable(ctx context.Context, t testing.TB, router *replica.Router) {
	t.Helper()

	_, err := router.Exec(ctx, `DROP TABLE IF EXISTS load_probe`)
	require.NoError(t, err)

	_, err = router.Exec(ctx, `CREATE TABLE load_probe (id bigserial PRIMARY KEY, payload text NOT NULL)`)
	require.NoError(t, err)

	_, err = router.Exec(ctx, `
		INSERT INTO load_probe (payload)
		SELECT md5(g::text) || repeat('x', 40)
		FROM generate_series(1, $1) AS g`, loadRows)
	require.NoError(t, err)

	// Without this the first reads pay for a fresh visibility map, and
	// whichever mode runs first looks worse for no reason.
	_, err = router.Exec(ctx, `VACUUM ANALYZE load_probe`)
	require.NoError(t, err)

	position, err := router.Watermark(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return router.Ready(position)
	}, catchUpTimeout, 100*time.Millisecond, "the standby never replayed the seed data")
}

// runLoad drives one scenario at a fixed offered rate.
//
// A unit of work is one request: a fresh tracker, then either a read, or a
// write followed by a read. The second shape is the interesting one — it is
// where the taint sends the read back to the primary, so the measured offload
// already accounts for read-after-write rather than pretending it away.
func runLoad(ctx context.Context, t testing.TB, router *replica.Router, mode string, readShare float64) loadResult {
	t.Helper()

	primaryBefore, replicaBefore := serverStats(ctx, t, router)

	var (
		operations atomic.Int64
		mu         sync.Mutex
		latencies  = make([]time.Duration, 0, loadRate*int(loadDuration.Seconds())+1)
		wg         sync.WaitGroup
	)

	runCtx, stop := context.WithTimeout(ctx, loadDuration)
	defer stop()

	// One ticker shared by every worker is the rate limiter: the offered rate
	// is a property of the scenario, not of how fast the server answers.
	ticker := time.NewTicker(time.Second / loadRate)
	defer ticker.Stop()

	wg.Add(loadWorkers)

	for worker := range loadWorkers {
		go func() {
			defer wg.Done()

			local := make([]time.Duration, 0, 1024)

			for iteration := 0; ; iteration++ {
				select {
				case <-runCtx.Done():
					mu.Lock()
					latencies = append(latencies, local...)
					mu.Unlock()

					return
				case <-ticker.C:
				}

				// A deterministic mix, so both modes see the same sequence.
				write := float64((worker*7+iteration)%100) >= readShare*100

				elapsed, err := loadUnit(runCtx, router, mode, write)
				if err != nil {
					continue
				}

				operations.Add(1)
				local = append(local, elapsed)
			}
		}()
	}

	wg.Wait()

	primaryAfter, replicaAfter := serverStats(ctx, t, router)

	return loadResult{
		mode:        mode,
		readShare:   readShare,
		operations:  operations.Load(),
		primaryTxns: primaryAfter - primaryBefore,
		replicaTxns: replicaAfter - replicaBefore,
		readP50:     percentile(latencies, 0.50),
		readP99:     percentile(latencies, 0.99),
	}
}

// loadUnit performs one request's worth of work and returns how long its read
// took.
func loadUnit(ctx context.Context, router *replica.Router, mode string, write bool) (time.Duration, error) {
	unitCtx := replica.WithTracker(ctx)

	var pool replica.Pool = router
	if mode == "baseline" {
		pool = router.Primary()
	}

	if write {
		_, err := pool.Exec(unitCtx, loadWriteSQL, "load")
		if err != nil {
			return 0, err
		}
	}

	started := time.Now()

	var (
		rows  int64
		bytes *int64
	)

	err := pool.QueryRow(unitCtx, loadReadSQL, 1+time.Now().UnixNano()%int64(loadRows-500)).Scan(&rows, &bytes)
	if err != nil {
		return 0, err
	}

	return time.Since(started), nil
}

func serverStats(ctx context.Context, t testing.TB, router *replica.Router) (int64, int64) {
	t.Helper()

	read := func(pool *pgxpool.Pool) int64 {
		var txns int64

		require.NoError(t, pool.QueryRow(ctx, serverStatsSQL).Scan(&txns))

		return txns
	}

	replicas := router.Replicas()
	require.Len(t, replicas, 1)

	return read(router.Primary()), read(replicas[0])
}

func percentile(values []time.Duration, q float64) time.Duration {
	if len(values) == 0 {
		return 0
	}

	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })

	return values[int(float64(len(values)-1)*q)]
}

// reportLoad prints the table that goes into the README, and a machine-readable
// line per scenario so the chart can be regenerated without re-reading prose.
func reportLoad(t testing.TB, results []loadResult) {
	t.Helper()

	t.Logf("%d workers at %d ops/s offered, %s per scenario, %.1f CPU per node, %d rows",
		loadWorkers, loadRate, loadDuration, nodeCPUs, loadRows)
	t.Logf("%-9s %-6s %8s %13s %13s %10s %10s %9s",
		"mode", "reads", "ops", "primary txns", "replica txns", "p50", "p99", "offload")

	for _, r := range results {
		t.Logf("%-9s %-6.0f %8d %13d %13d %10s %10s %8.1f%%",
			r.mode, r.readShare*100, r.operations, r.primaryTxns, r.replicaTxns,
			r.readP50.Round(time.Microsecond), r.readP99.Round(time.Microsecond), r.offload()*100)

		fmt.Printf("LOADCSV,%s,%.0f,%d,%d,%d,%.3f,%.3f,%.1f\n",
			r.mode, r.readShare*100, r.operations, r.primaryTxns, r.replicaTxns,
			r.readP50.Seconds()*1000, r.readP99.Seconds()*1000, r.offload()*100)
	}
}
