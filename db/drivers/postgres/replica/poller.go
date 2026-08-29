package replica

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// Probe statements.
//
// Every function used here is available to an ordinary role. The pg_control
// family would give the timeline and system identifier more directly, but it is
// restricted to superusers by default, and a monitoring path that needs
// superuser is a monitoring path that gets turned off.
const (
	// The primary probe reads the insert position and, through the WAL file
	// name, the timeline.
	//
	// The CASE guards matter, because reading the insert position raises
	// "recovery is in progress" when the node we were told is the primary is
	// itself a standby — a cascading setup, or the window during a failover.
	primaryProbeSQL = `
		SELECT pg_is_in_recovery(),
		       CASE WHEN pg_is_in_recovery() THEN NULL
		            ELSE pg_current_wal_insert_lsn()::text END,
		       CASE WHEN pg_is_in_recovery() THEN NULL
		            ELSE pg_walfile_name(pg_current_wal_insert_lsn()) END`

	// The replica probe reads a standby's replay and receive positions.
	//
	// Both are nullable, and that is the point: a promoted node returns NULL
	// rather than an error. Receive far ahead of replay is the fingerprint of a
	// standby pausing replay to resolve a query conflict.
	replicaProbeSQL = `
		SELECT pg_is_in_recovery(),
		       pg_last_wal_replay_lsn()::text,
		       pg_last_wal_receive_lsn()::text`

	// The system identifier is best-effort, because it needs privileges an
	// application role often lacks. Without it, lineage checks fall back to the
	// timeline alone, which still detects every failover.
	systemIDSQL = `SELECT system_identifier FROM pg_control_system()`

	// The apply-delay probe detects a deliberately delayed standby.
	applyDelaySQL = `SHOW recovery_min_apply_delay`
)

// walFileTimelineLen is the length of the timeline prefix in a WAL file name
// ("000000010000000000000001" — the first eight hex digits).
const walFileTimelineLen = 8

// Numeric bases and widths used when reading probe output.
const (
	hexBase      = 16
	decimalBase  = 10
	timelineBits = 32
	delayBits    = 64
)

// start launches the poller. The goroutine ends on ctx cancellation or on
// close, and close waits for it: the driver's tests run under goleak, and a
// poller outliving its store fails them.
func (g *gate) start(ctx context.Context) {
	// A zero interval is the documented way to disable polling. Resetting a
	// timer to zero would instead create a tight query loop against every node.
	if g == nil || g.interval <= 0 {
		return
	}

	g.started = true

	go g.run(ctx)
}

func (g *gate) run(ctx context.Context) {
	defer close(g.done)

	// Fire immediately: until the first round completes every replica is
	// ineligible, so a slow first tick is a slow start for the whole feature.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-g.stop:
			return
		case <-timer.C:
		}

		g.probeAll(ctx)
		g.broadcast()

		timer.Reset(g.nextInterval())
	}
}

// close stops the poller and waits for it to exit.
func (g *gate) close() {
	if g == nil {
		return
	}

	g.closeOnce.Do(func() {
		close(g.stop)

		// Only wait for a poller that exists. A gate with no replicas never
		// started one, and waiting on its done channel would hang forever.
		if g.started {
			<-g.done
		}

		g.metrics.Close()

		for _, node := range g.replicas {
			node.pool.Close()
		}
	})
}

// nextInterval spreads polls so that a fleet of pods does not probe the same
// replica in lockstep.
func (g *gate) nextInterval() time.Duration {
	if g.jitter <= 0 {
		return g.interval
	}

	// A fraction above one would produce negative timer durations for part of
	// the random range, which time.Timer treats as "fire immediately". Clamp it
	// so a bad deployment value cannot turn into a polling storm.
	spread := float64(g.interval) * min(g.jitter, 1)
	next := time.Duration(float64(g.interval) + (rand.Float64()*2-1)*spread) //nolint:gosec // jitter, not a secret

	return max(next, time.Nanosecond)
}

func (g *gate) probeAll(ctx context.Context) {
	g.probePrimary(ctx)

	var probes sync.WaitGroup

	for _, node := range g.replicas {
		probes.Go(func() {
			g.probeReplica(ctx, node)
		})
	}

	probes.Wait()
}

func (g *gate) probePrimary(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, g.probeTimeout)
	defer cancel()

	var (
		inRecovery bool
		insertLSN  *string
		walFile    *string
	)

	err := g.primary.QueryRow(probeCtx, primaryProbeSQL).Scan(&inRecovery, &insertLSN, &walFile)
	if err != nil {
		g.pmu.Lock()
		g.primaryErr = err
		g.pmu.Unlock()

		g.metrics.ProbeFailed(ctx, HostOf(g.primary), err)

		return
	}

	// The node we call the primary is in recovery. Its insert position is
	// unavailable, so lag is unknown; say so rather than computing a negative
	// number and calling it zero.
	if inRecovery || insertLSN == nil {
		g.pmu.Lock()
		g.primaryLSN = 0
		g.primaryErr = ErrPrimaryInRecovery
		g.pmu.Unlock()

		return
	}

	position, err := wal.ParseLSN(*insertLSN)
	if err != nil {
		g.pmu.Lock()
		g.primaryErr = err
		g.pmu.Unlock()

		return
	}

	timeline := timelineFromWALFile(walFile)

	g.pmu.Lock()
	previous := g.timeline
	g.primaryLSN = position
	g.primaryAt = time.Now()
	g.primaryErr = nil

	if timeline != 0 {
		g.timeline = timeline
	}
	g.pmu.Unlock()

	if previous != 0 && timeline != 0 && previous != timeline {
		g.logWarn("postgres: primary timeline changed, watermarks from the previous timeline are no longer comparable",
			slog.Uint64("previous", uint64(previous)),
			slog.Uint64("current", uint64(timeline)),
		)
	}
}

func (g *gate) probeReplica(ctx context.Context, node *replicaNode) {
	probeCtx, cancel := context.WithTimeout(ctx, g.probeTimeout)
	defer cancel()

	started := time.Now()

	var (
		inRecovery  bool
		replayText  *string
		receiveText *string
	)

	err := node.pool.QueryRow(probeCtx, replicaProbeSQL).Scan(&inRecovery, &replayText, &receiveText)
	elapsed := time.Since(started)

	g.metrics.ProbeDuration(ctx, node.host, elapsed)

	if err != nil {
		node.mu.Lock()
		node.state.recordFailure(err)
		node.mu.Unlock()

		g.metrics.ProbeFailed(ctx, node.host, err)

		return
	}

	// A node that left recovery was promoted. That is a topology change, not a
	// transient fault: retrying cannot fix it, and routing reads there means
	// reading a diverged timeline from a node that now accepts writes. Exclude
	// it for the lifetime of this process and say so loudly.
	if !inRecovery {
		node.mu.Lock()
		alreadyKnown := node.state.recordPromotion(time.Now())
		node.mu.Unlock()

		if !alreadyKnown {
			g.metrics.Promotion(ctx, node.host)
			g.logWarn("postgres: replica left recovery and was quarantined",
				slog.String("server.address", node.host),
			)
		}

		return
	}

	replay := parseNullableLSN(replayText)
	receive := parseNullableLSN(receiveText)

	primary := g.primaryPosition()
	lag := primary.lagBehind(replay, g.staleAfter)

	node.mu.Lock()
	node.state.recordStandby(replay, receive, lag, time.Now())
	node.mu.Unlock()
}

// checkApplyDelay refuses to enable the gate against a deliberately delayed
// standby. Such a replica can never satisfy a fresh watermark, so every gated
// read would fall back to the primary forever and the feature would be an
// expensive no-op that looks like it is working.
func (g *gate) checkApplyDelay(ctx context.Context, node *replicaNode) error {
	var delay string

	err := node.pool.QueryRow(ctx, applyDelaySQL).Scan(&delay)
	if err != nil {
		// Not fatal: the setting is readable by ordinary roles, but a server
		// that will not answer is a server we have no opinion about.
		return nil //nolint:nilerr // absence of an answer is not a delayed standby
	}

	if isZeroDelay(delay) {
		return nil
	}

	return &ConfigError{Host: node.host, Setting: "recovery_min_apply_delay", Value: delay}
}

// resolveSystemID fills in the cluster identity, if the role is allowed to
// read it.
func (g *gate) resolveSystemID(ctx context.Context) {
	var systemID uint64

	err := g.primary.QueryRow(ctx, systemIDSQL).Scan(&systemID)
	if err != nil {
		g.logDebug("postgres: system identifier unavailable, lineage checks will use the timeline alone",
			slog.String("error", err.Error()),
		)

		return
	}

	g.pmu.Lock()
	g.systemID = systemID
	g.pmu.Unlock()
}

// timelineFromWALFile extracts the timeline from a WAL segment name. The first
// eight hex digits are the timeline id.
func timelineFromWALFile(name *string) uint32 {
	if name == nil || len(*name) < walFileTimelineLen {
		return 0
	}

	timeline, err := strconv.ParseUint((*name)[:walFileTimelineLen], hexBase, timelineBits)
	if err != nil {
		return 0
	}

	return uint32(timeline) //nolint:gosec // ParseUint was given a 32-bit size
}

// parseNullableLSN turns a nullable text position into an LSN. NULL and
// unparseable both become zero, which reads as "infinitely behind" — the safe
// direction, because a zero position satisfies no watermark.
func parseNullableLSN(text *string) wal.LSN {
	if text == nil {
		return 0
	}

	position, err := wal.ParseLSN(*text)
	if err != nil {
		return 0
	}

	return position
}

// isZeroDelay reports whether recovery_min_apply_delay is off. PostgreSQL
// renders it as a number with a unit: "0", "0ms", "500ms", "1min".
func isZeroDelay(value string) bool {
	trimmed := strings.TrimSpace(value)

	end := 0
	for end < len(trimmed) && trimmed[end] >= '0' && trimmed[end] <= '9' {
		end++
	}

	if end == 0 {
		// No leading number at all. Nothing to act on, and refusing to start
		// over a value we cannot read would be worse than proceeding.
		return true
	}

	amount, err := strconv.ParseUint(trimmed[:end], decimalBase, delayBits)

	return err != nil || amount == 0 //nolint:gosec // compared, never converted
}

func (g *gate) logWarn(msg string, fields ...slog.Attr) {
	if g.log == nil {
		return
	}

	g.log.Warn(msg, fields...)
}

func (g *gate) logDebug(msg string, fields ...slog.Attr) {
	if g.log == nil {
		return
	}

	g.log.Debug(msg, fields...)
}
