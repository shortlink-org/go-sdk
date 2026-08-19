//go:build database && postgres && replica

// Package-level note: this file is the only place the routing is proved
// against real streaming replication. Everything else in the suite works on
// fabricated health samples, which cannot catch the things that actually go
// wrong — a standby refusing a statement, a promotion returning NULL instead
// of an error, a watermark satisfied one WAL record too early.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

const (
	// pgImage stays on latest deliberately: a container test that pins a tag
	// stops testing the thing people actually run.
	pgImage = "postgres:latest"

	// pgData is set explicitly because the official image has moved its
	// default PGDATA between major versions.
	pgData = "/var/lib/postgresql/pgdata"

	// primaryAlias is how the standby reaches the primary on the test network.
	primaryAlias = "primary"

	replicaStartupTimeout = 3 * time.Minute
	catchUpTimeout        = 30 * time.Second
)

// replicationHBA is run by the image as an init script.
//
// POSTGRES_HOST_AUTH_METHOD only appends "host all all all <method>" to
// pg_hba.conf, and a replication connection is not matched by "all" — it needs
// its own line. Without this, pg_basebackup fails with "no pg_hba.conf entry
// for replication connection", which is a confusing way to learn it.
const replicationHBA = `#!/bin/sh
set -eu
echo "host replication all all trust" >> "$PGDATA/pg_hba.conf"
`

// standbyScript replaces the image entrypoint. pg_basebackup -R writes
// standby.signal and primary_conninfo, which is all a streaming standby needs.
const standbyScript = `
set -eu
mkdir -p "$PGDATA"
chmod 0700 "$PGDATA"
until pg_isready -h ` + primaryAlias + ` -U postgres -q; do sleep 1; done
pg_basebackup -h ` + primaryAlias + ` -U postgres -D "$PGDATA" -Fp -Xs -R
exec postgres -D "$PGDATA" -c hot_standby=on
`

type cluster struct {
	primaryDSN string
	replicaDSN string
	primary    testcontainers.Container
	replica    testcontainers.Container
}

// startCluster brings up a primary and a streaming standby on a private
// network.
func startCluster(ctx context.Context, t testing.TB) *cluster {
	t.Helper()

	return startClusterWithCPU(ctx, t, 0)
}

// startClusterWithCPU brings up the cluster with an optional per-node CPU
// limit. Zero leaves both nodes unconstrained.
//
// The limit is what makes a load test on a single machine mean anything. Left
// unconstrained, the primary and the standby draw from the same cores, so
// moving reads to the standby only changes which container burns a core and
// adds no capacity at all. Capping each node models the topology this feature
// is actually for: two machines.
func startClusterWithCPU(ctx context.Context, t testing.TB, cpus float64) *cluster {
	t.Helper()

	limit := func(hc *container.HostConfig) {
		if cpus > 0 {
			hc.NanoCPUs = int64(cpus * 1e9)
		}
	}

	net, err := tcnetwork.New(ctx)
	require.NoError(t, err)
	testcontainers.CleanupNetwork(t, net)

	primary, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        pgImage,
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER": "postgres",
				"POSTGRES_DB":   "shortlink",
				// Trust also opens replication connections, which is what the
				// standby's pg_basebackup needs. Test-only, obviously.
				"POSTGRES_HOST_AUTH_METHOD": "trust",
				"PGDATA":                    pgData,
			},
			Cmd: []string{
				"postgres",
				"-c", "wal_level=replica",
				"-c", "max_wal_senders=10",
				"-c", "hot_standby=on",
				"-c", "wal_keep_size=64MB",
			},
			Files: []testcontainers.ContainerFile{{
				Reader:            strings.NewReader(replicationHBA),
				ContainerFilePath: "/docker-entrypoint-initdb.d/00-replication.sh",
				FileMode:          0o755,
			}},
			HostConfigModifier: limit,
			Networks:           []string{net.Name},
			NetworkAliases:     map[string][]string{net.Name: {primaryAlias}},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("5432/tcp"),
				wait.ForExec([]string{"pg_isready", "-U", "postgres"}),
			).WithDeadline(replicaStartupTimeout),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, primary)
	require.NoError(t, err)

	standby, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:              pgImage,
			ExposedPorts:       []string{"5432/tcp"},
			Env:                map[string]string{"PGDATA": pgData},
			User:               "postgres",
			Entrypoint:         []string{"bash", "-c", standbyScript},
			HostConfigModifier: limit,
			Networks:           []string{net.Name},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("5432/tcp"),
				wait.ForExec([]string{"pg_isready", "-U", "postgres"}),
			).WithDeadline(replicaStartupTimeout),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, standby)
	require.NoError(t, err)

	return &cluster{
		primaryDSN: dsn(ctx, t, primary),
		replicaDSN: dsn(ctx, t, standby),
		primary:    primary,
		replica:    standby,
	}
}

func dsn(ctx context.Context, t testing.TB, container testcontainers.Container) string {
	t.Helper()

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)

	return fmt.Sprintf("postgres://postgres@%s:%s/shortlink?sslmode=disable", host, port.Port())
}

// newRoutedStore wires a store against the cluster and returns its router.
func newRoutedStore(ctx context.Context, t testing.TB, c *cluster, opts ...Option) (*Store, *replica.Router) {
	t.Helper()

	cfg, err := config.New()
	require.NoError(t, err)

	t.Setenv("STORE_POSTGRES_URI", c.primaryDSN)
	t.Setenv("STORE_POSTGRES_REPLICA_URI", c.replicaDSN)

	store := New(noop.NewTracerProvider(), metric.NewMeterProvider(), cfg, opts...)
	require.NoError(t, store.Init(ctx))
	t.Cleanup(store.Close)

	router := store.Router()
	require.NotNil(t, router)
	require.True(t, router.Enabled())

	waitForHealthy(t, router)

	return store, router
}

// waitForHealthy blocks until the poller has produced a usable sample.
func waitForHealthy(t testing.TB, router *replica.Router) {
	t.Helper()

	require.Eventually(t, func() bool {
		for _, health := range router.Health() {
			if health.Healthy {
				return true
			}
		}

		return false
	}, catchUpTimeout, 50*time.Millisecond, "no replica became healthy")
}

// onStandby asks the server itself where the statement ran. It is the only
// answer that cannot be fooled by a routing bug.
func onStandby(ctx context.Context, t testing.TB, router *replica.Router) bool {
	t.Helper()

	var inRecovery bool

	require.NoError(t, router.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&inRecovery))

	return inRecovery
}

func seedSchema(ctx context.Context, t testing.TB, router *replica.Router) {
	t.Helper()

	_, err := router.Exec(ctx, `CREATE TABLE IF NOT EXISTS routing_probe (id bigserial PRIMARY KEY, note text)`)
	require.NoError(t, err)
}

func TestReplicaRouting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := startCluster(ctx, t)
	_, router := newRoutedStore(ctx, t, c)

	seedSchema(ctx, t, router)

	t.Run("reads reach the standby", func(t *testing.T) {
		assert.True(t, onStandby(replica.WithTracker(ctx), t, router))
	})

	t.Run("writes reach the primary", func(t *testing.T) {
		target, err := router.Route(replica.WithTracker(ctx), `INSERT INTO routing_probe (note) VALUES ('x')`)
		require.NoError(t, err)
		assert.True(t, target.OnPrimary())
	})

	t.Run("an unscoped read stays on the primary", func(t *testing.T) {
		assert.False(t, onStandby(ctx, t, router), "no tracker means no reasoning; default to the primary")
	})

	// The in-request half of read-your-writes.
	t.Run("a read after a write stays on the primary", func(t *testing.T) {
		writeCtx := replica.WithTracker(ctx)

		require.True(t, onStandby(writeCtx, t, router), "the context starts clean")

		_, err := router.Exec(writeCtx, `INSERT INTO routing_probe (note) VALUES ('taint')`)
		require.NoError(t, err)

		assert.False(t, onStandby(writeCtx, t, router), "the write must pin the rest of this context")
	})

	// The cross-boundary half: the watermark is captured once, and a fresh
	// context — a different request, a consumed message — is gated on it.
	t.Run("a handed-over watermark gates a fresh context", func(t *testing.T) {
		writeCtx := replica.WithTracker(ctx)

		_, err := router.Exec(writeCtx, `INSERT INTO routing_probe (note) VALUES ('handoff')`)
		require.NoError(t, err)

		position, err := router.Watermark(writeCtx)
		require.NoError(t, err)
		require.Positive(t, uint64(position))

		readCtx := replica.WithWatermark(ctx, position)

		// Before the standby has replayed that far, the read belongs on the
		// primary. Once it has, it belongs on the standby. Await is what turns
		// the first into the second.
		ready, err := router.Await(readCtx, position)
		require.NoError(t, err)

		if !ready {
			require.Eventually(t, func() bool {
				target, routeErr := router.Route(readCtx, `SELECT 1`)

				return routeErr == nil && !target.OnPrimary()
			}, catchUpTimeout, 20*time.Millisecond, "the standby never replayed the watermark")
		}

		assert.True(t, onStandby(readCtx, t, router))
	})

	t.Run("an unreachable watermark stays on the primary", func(t *testing.T) {
		readCtx := replica.WithWatermark(ctx, wal.Unknown)

		target, err := router.Route(readCtx, `SELECT 1`)
		require.NoError(t, err)
		assert.True(t, target.OnPrimary())
		assert.Equal(t, metrics.ReasonBehind, target.Reason)
	})

	t.Run("an explicit stale read reaches the standby", func(t *testing.T) {
		writeCtx := replica.WithTracker(ctx)

		_, err := router.Exec(writeCtx, `INSERT INTO routing_probe (note) VALUES ('stale')`)
		require.NoError(t, err)

		assert.True(t, onStandby(replica.Stale(writeCtx), t, router), "a stale read accepts the taint")
	})

	// A standby rejects a locking clause outright. The classifier is what
	// keeps this from becoming a runtime error.
	t.Run("a locking select is not sent to the standby", func(t *testing.T) {
		rows, err := router.Query(replica.WithTracker(ctx), `SELECT id FROM routing_probe FOR UPDATE`)
		require.NoError(t, err)
		defer rows.Close()

		require.NoError(t, rows.Err())
	})

	t.Run("a read-only transaction may run on the standby", func(t *testing.T) {
		tx, err := router.BeginTx(replica.WithTracker(ctx), pgx.TxOptions{AccessMode: pgx.ReadOnly})
		require.NoError(t, err)

		defer func() { _ = tx.Rollback(ctx) }()

		var inRecovery bool
		require.NoError(t, tx.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&inRecovery))
		assert.True(t, inRecovery)
	})

	t.Run("health reports lag in bytes", func(t *testing.T) {
		health := router.Health()
		require.Len(t, health, 1)

		assert.True(t, health[0].InRecovery)
		assert.False(t, health[0].Quarantined)
		assert.GreaterOrEqual(t, health[0].LagBytes, int64(0))
		assert.Positive(t, uint64(health[0].ReplayLSN))
	})

	t.Run("the lineage is known", func(t *testing.T) {
		token, err := router.Token(replica.WithTracker(ctx))
		require.NoError(t, err)

		assert.Equal(t, uint32(1), token.Timeline, "a fresh cluster is on timeline 1")

		position, ok := router.Accept(token)
		require.True(t, ok)
		assert.Equal(t, token.LSN, position)

		foreign := wal.Token{SystemID: token.SystemID, Timeline: token.Timeline + 1, LSN: token.LSN}
		_, ok = router.Accept(foreign)
		assert.False(t, ok, "a token from another timeline must be discarded")

		// Discarded, but not ignored: the reader still gets pinned.
		assert.Equal(t, replica.StrategyPrimary, replica.StrategyFromContext(router.WithToken(ctx, foreign)))
	})
}

// TestReplicaFallbackWhenStandbyStops is the degradation path: the reads keep
// working, on the primary, and the health snapshot says why.
func TestReplicaFallbackWhenStandbyStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := startCluster(ctx, t)
	_, router := newRoutedStore(ctx, t, c,
		WithReplicaProbeTimeout(500*time.Millisecond),
		WithReplicaSampleStaleAfter(time.Second),
	)

	require.True(t, onStandby(replica.WithTracker(ctx), t, router))

	stopTimeout := 10 * time.Second
	require.NoError(t, c.replica.Stop(ctx, &stopTimeout))

	require.Eventually(t, func() bool {
		target, err := router.Route(replica.WithTracker(ctx), `SELECT 1`)

		return err == nil && target.OnPrimary()
	}, catchUpTimeout, 100*time.Millisecond, "routing never fell back to the primary")

	assert.False(t, onStandby(replica.WithTracker(ctx), t, router))

	health := router.Health()
	require.Len(t, health, 1)
	assert.False(t, health[0].Healthy)
}

// TestReplicaPromotionQuarantines covers the trap that pays a pager. A
// promoted node keeps answering, so nothing fails — it just silently stops
// being a replica of anything.
func TestReplicaPromotionQuarantines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := startCluster(ctx, t)
	_, router := newRoutedStore(ctx, t, c)

	require.True(t, onStandby(replica.WithTracker(ctx), t, router))

	code, _, err := c.replica.Exec(ctx, []string{"pg_ctl", "promote", "-D", pgData})
	require.NoError(t, err)
	require.Zero(t, code)

	require.Eventually(t, func() bool {
		health := router.Health()

		return len(health) == 1 && health[0].Quarantined
	}, catchUpTimeout, 100*time.Millisecond, "the promoted node was never quarantined")

	health := router.Health()
	assert.False(t, health[0].Healthy)
	assert.False(t, health[0].InRecovery)
	assert.ErrorIs(t, health[0].Err, replica.ErrReplicaPromoted)

	target, err := router.Route(replica.WithTracker(ctx), `SELECT 1`)
	require.NoError(t, err)
	assert.True(t, target.OnPrimary(), "a quarantined node must never be chosen again")
}

// TestReplicaPoolsAreDistinct guards the mistake that hides itself: reusing
// the primary's pool config for the replicas connects every pool to the
// primary, and everything works.
func TestReplicaPoolsAreDistinct(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := startCluster(ctx, t)
	store, router := newRoutedStore(ctx, t, c)

	pool, ok := store.GetConn().(*pgxpool.Pool)
	require.True(t, ok)
	assert.Same(t, router.Primary(), pool, "GetConn must keep returning the primary")

	replicas := router.Replicas()
	require.Len(t, replicas, 1)
	assert.NotEqual(t, replica.HostOf(router.Primary()), replica.HostOf(replicas[0]), "the replica pool must not point at the primary")

	var inRecovery bool
	require.NoError(t, replicas[0].QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&inRecovery))
	assert.True(t, inRecovery)
}

// BenchmarkEndToEnd puts the routing decision next to the thing it precedes.
// The decision is measured in nanoseconds and a query in microseconds, so the
// only number that matters here is the ratio.
func BenchmarkEndToEnd(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	c := startCluster(ctx, b)
	store, router := newRoutedStore(ctx, b, c)

	pool, ok := store.GetConn().(*pgxpool.Pool)
	require.True(b, ok)

	const query = `SELECT id FROM pg_class WHERE relname = $1`

	b.Run("raw_pool", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			var id uint32
			_ = pool.QueryRow(ctx, query, "pg_class").Scan(&id)
		}
	})

	// The watermark capture is the one place the router adds a round trip of
	// its own. This is what a SendBatch-with-COMMIT optimization would save.
	b.Run("watermark_capture", func(b *testing.B) {
		writeCtx := replica.WithTracker(ctx)

		b.ReportAllocs()

		for b.Loop() {
			_, _ = router.Watermark(writeCtx)
		}
	})

	b.Run("through_router", func(b *testing.B) {
		readCtx := replica.WithTracker(ctx)

		b.ReportAllocs()

		for b.Loop() {
			var id uint32
			_ = router.QueryRow(readCtx, query, "pg_class").Scan(&id)
		}
	})
}

// TestReplicaInTx covers the transaction helper, including the batched commit
// that reads the WAL position in the same round trip.
func TestReplicaInTx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := startCluster(ctx, t)
	_, router := newRoutedStore(ctx, t, c, WithSyncWatermark(true))

	seedSchema(ctx, t, router)

	t.Run("commits and records the watermark", func(t *testing.T) {
		writeCtx := replica.WithTracker(ctx)

		err := router.InTx(writeCtx, pgx.TxOptions{}, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO routing_probe (note) VALUES ('intx')`)

			return err
		})
		require.NoError(t, err)

		// The position must come back from the batched commit, and it must be
		// a real one — not the wal.Unknown fallback.
		watermark := replica.TrackerFromContext(writeCtx).Watermark()
		assert.Positive(t, uint64(watermark))
		assert.NotEqual(t, wal.Unknown, watermark, "the batched commit did not return a position")

		// Captured after the commit, so the standby reaching it means the row
		// is there. Read one WAL record too early and this is where it shows.
		require.Eventually(t, func() bool {
			return router.Ready(watermark)
		}, catchUpTimeout, 50*time.Millisecond)

		var count int
		require.NoError(t, router.QueryRow(replica.WithWatermark(ctx, watermark),
			`SELECT count(*) FROM routing_probe WHERE note = 'intx'`).Scan(&count))
		assert.Positive(t, count, "the standby reported catching up but does not have the row")
	})

	t.Run("rolls back on error", func(t *testing.T) {
		sentinel := errors.New("no")

		err := router.InTx(replica.WithTracker(ctx), pgx.TxOptions{}, func(ctx context.Context, tx pgx.Tx) error {
			_, execErr := tx.Exec(ctx, `INSERT INTO routing_probe (note) VALUES ('rolled-back')`)
			require.NoError(t, execErr)

			return sentinel
		})
		require.ErrorIs(t, err, sentinel)

		var count int
		require.NoError(t, router.QueryRow(replica.OnPrimary(ctx),
			`SELECT count(*) FROM routing_probe WHERE note = 'rolled-back'`).Scan(&count))
		assert.Zero(t, count)
	})

	t.Run("a read-only transaction runs on the standby", func(t *testing.T) {
		err := router.InTx(replica.WithTracker(ctx), pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
			var inRecovery bool
			if err := tx.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&inRecovery); err != nil {
				return err
			}

			assert.True(t, inRecovery)

			return nil
		})
		require.NoError(t, err)
	})

	// The pool must come back clean. A transaction left open, or a connection
	// released mid-transaction, would surface here as a stuck backend.
	t.Run("the connection is returned clean", func(t *testing.T) {
		for range 20 {
			require.NoError(t, router.InTx(replica.WithTracker(ctx), pgx.TxOptions{}, func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO routing_probe (note) VALUES ('loop')`)

				return err
			}))
		}

		var idleInTx int
		require.NoError(t, router.QueryRow(replica.OnPrimary(ctx), `
			SELECT count(*) FROM pg_stat_activity
			WHERE state = 'idle in transaction' AND pid <> pg_backend_pid()`).Scan(&idleInTx))
		assert.Zero(t, idleInTx, "a connection was returned to the pool mid-transaction")
	})
}

// BenchmarkInTxCommit is the number that justifies the batched commit.
func BenchmarkInTxCommit(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	c := startCluster(ctx, b)
	_, router := newRoutedStore(ctx, b, c, WithSyncWatermark(true))

	seedSchema(ctx, b, router)

	insert := func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO routing_probe (note) VALUES ('bench')`)

		return err
	}

	b.Run("batched_commit", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			_ = router.InTx(replica.WithTracker(ctx), pgx.TxOptions{}, insert)
		}
	})

	b.Run("separate_watermark_query", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			writeCtx := replica.WithTracker(ctx)

			tx, err := router.Primary().Begin(writeCtx)
			if err != nil {
				continue
			}

			_ = insert(writeCtx, tx)
			_ = tx.Commit(writeCtx)
			_, _ = router.Watermark(writeCtx)
		}
	})
}

// setApplyDelay appends recovery_min_apply_delay to the standby's
// postgresql.auto.conf and reloads. The parameter is SIGHUP-scoped, so no
// restart is needed, and appending is enough because the last assignment in
// the file wins — which is also how the test puts it back.
func setApplyDelay(ctx context.Context, t testing.TB, c *cluster, value string) {
	t.Helper()

	script := fmt.Sprintf(`set -eu
echo "recovery_min_apply_delay = '%s'" >> "$PGDATA/postgresql.auto.conf"
pg_ctl reload -D "$PGDATA"`, value)

	code, out, err := c.replica.Exec(ctx, []string{"bash", "-c", script})
	require.NoError(t, err)

	if code != 0 {
		body, readErr := io.ReadAll(out)
		require.NoError(t, readErr)
		t.Fatalf("setting recovery_min_apply_delay=%s failed: %s", value, body)
	}

	// A reload is asynchronous, so wait for the server to report the new value
	// rather than racing the driver's own probe against it.
	conn, err := pgx.Connect(ctx, c.replicaDSN)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, conn.Close(ctx))
	})

	require.Eventually(t, func() bool {
		var current string

		err := conn.QueryRow(ctx, `SHOW recovery_min_apply_delay`).Scan(&current)

		return err == nil && current == value
	}, catchUpTimeout, 100*time.Millisecond, "the standby never applied recovery_min_apply_delay=%s", value)
}

// TestReplicaApplyDelayRefusesToStart covers the failure the ADR calls out as
// the one that looks like success: a deliberately delayed standby answers
// every probe and stays healthy, but can never satisfy a fresh watermark, so
// every gated read falls back to the primary and the feature is an expensive
// no-op. Startup has to reject it.
func TestReplicaApplyDelayRefusesToStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c := startCluster(ctx, t)
	setApplyDelay(ctx, t, c, "5min")

	cfg, err := config.New()
	require.NoError(t, err)

	t.Setenv("STORE_POSTGRES_URI", c.primaryDSN)
	t.Setenv("STORE_POSTGRES_REPLICA_URI", c.replicaDSN)

	store := New(noop.NewTracerProvider(), metric.NewMeterProvider(), cfg)
	t.Cleanup(store.Close)

	err = store.Init(ctx)
	require.Error(t, err, "a delayed standby must fail startup, not enable the gate")

	var configErr *replica.ConfigError

	require.ErrorAs(t, err, &configErr)
	assert.Equal(t, "recovery_min_apply_delay", configErr.Setting)
	assert.Equal(t, "5min", configErr.Value)

	// Put it back and start again, so the failure above is attributable to the
	// setting and not to a cluster that was never going to work.
	setApplyDelay(ctx, t, c, "0")
	newRoutedStore(ctx, t, c)
}
