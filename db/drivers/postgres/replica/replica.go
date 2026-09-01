package replica

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// ReplicaHealth is what the poller last saw for one replica. It is a snapshot:
// reading it never touches the database.
type ReplicaHealth struct {
	Index int
	// Host is host:port only. Credentials never appear here — this value
	// reaches logs and metric attributes.
	Host string
	// Healthy reports whether the replica is currently eligible for reads.
	Healthy bool
	// Quarantined reports whether the node was seen leaving recovery. A
	// promoted node is a topology change, so it is excluded permanently.
	Quarantined bool
	InRecovery  bool
	ReplayLSN   wal.LSN
	ReceiveLSN  wal.LSN
	// LagBytes is how far the replay position trails the primary's insert
	// position. Bytes, not seconds: a replica can close five minutes of lag in
	// seconds, or fail to close thirty seconds of it, so elapsed time is a poor
	// predictor of readiness. A negative value means the primary position was
	// unavailable and the lag could not be calculated.
	LagBytes int64
	LastPoll time.Time
	Err      error
}

// replicaNode is one standby and everything the poller knows about it.
type replicaNode struct {
	pool  *pgxpool.Pool
	host  string
	index int

	mu    sync.RWMutex
	state replicaState
}

type replicaLifecycle uint8

const (
	replicaUnobserved replicaLifecycle = iota
	replicaStandby
	replicaQuarantined
)

// replicaState is the complete cached observation of one node. Its mutation
// methods are the only transitions the poller can make, and its predicates own
// eligibility and watermark qualification. The enclosing node mutex protects
// the value.
type replicaState struct {
	lifecycle  replicaLifecycle
	replayLSN  wal.LSN
	receiveLSN wal.LSN
	lagBytes   int64
	lastPoll   time.Time
	lastErr    error
	failures   int
}

func (s replicaState) inRecovery() bool { return s.lifecycle == replicaStandby }

func (s replicaState) quarantined() bool { return s.lifecycle == replicaQuarantined }

func (s replicaState) eligible(staleAfter time.Duration) bool {
	return s.lifecycle == replicaStandby &&
		!s.lastPoll.IsZero() &&
		s.failures < failureThreshold &&
		time.Since(s.lastPoll) <= staleAfter
}

func (s replicaState) qualification(
	required wal.LSN,
	maxLagBytes int64,
	staleAfter time.Duration,
) replicaQualification {
	if !s.eligible(staleAfter) {
		return replicaUnavailable
	}

	if required > 0 {
		if s.replayLSN < required {
			return replicaBehind
		}

		return replicaCaughtUp
	}

	if maxLagBytes > 0 && (s.lagBytes == unknownLagBytes || s.lagBytes > maxLagBytes) {
		return replicaBehind
	}

	return replicaWithinLag
}

func (s *replicaState) recordFailure(err error) {
	s.failures++
	s.lastErr = err
}

func (s *replicaState) recordPromotion(observed time.Time) (alreadyKnown bool) {
	alreadyKnown = s.lifecycle == replicaQuarantined
	s.lifecycle = replicaQuarantined
	s.lastPoll = observed
	s.lastErr = ErrReplicaPromoted

	return alreadyKnown
}

func (s *replicaState) recordStandby(
	replay wal.LSN,
	receive wal.LSN,
	lag int64,
	observed time.Time,
) {
	// Promotion is a topology change, so quarantine is irreversible for this
	// process even if a later probe reaches a differently configured endpoint.
	if s.lifecycle == replicaQuarantined {
		return
	}

	s.lifecycle = replicaStandby
	s.replayLSN = replay
	s.receiveLSN = receive
	s.lagBytes = lag
	s.lastPoll = observed
	s.lastErr = nil
	s.failures = 0
}

// failureThreshold is how many consecutive failed probes take a replica out of
// rotation. One failed probe is a hiccup; the poller runs often enough that a
// real outage is confirmed within a second.
const failureThreshold = 2

// unknownLagBytes distinguishes "the replica is current" from "the primary
// probe failed, so lag cannot be calculated". Treating both as zero would let
// an unwatermarked read bypass MaxLagBytes during a primary monitoring outage.
const unknownLagBytes int64 = -1

// gate is the read side of replication lag: a background poller that samples
// every replica, and the predicates the router asks before choosing a pool.
//
// Everything the hot path reads is a cached sample behind a mutex. No routing
// decision issues a query.
type gate struct {
	primary  *pgxpool.Pool
	replicas []*replicaNode

	interval     time.Duration
	jitter       float64
	probeTimeout time.Duration
	staleAfter   time.Duration
	maxLagBytes  int64

	log     *slog.Logger
	metrics *metrics.Metrics

	// primary sample
	pmu        sync.RWMutex
	primaryLSN wal.LSN
	primaryAt  time.Time
	primaryErr error
	systemID   uint64
	timeline   uint32

	// cursor spreads reads across eligible replicas.
	cursor atomic.Uint64

	// notify is closed and replaced on every completed poll round, so a waiter
	// wakes as soon as there is new information instead of on a fixed tick.
	notifyMu sync.Mutex
	notify   chan struct{}

	// started records whether the poller goroutine was ever launched. Without
	// it, closing a gate that has no replicas — and therefore no poller —
	// waits forever for a goroutine that was never started.
	started   bool
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func newGate(primary *pgxpool.Pool, replicas []*replicaNode, cfg *Options, log *slog.Logger, instruments *metrics.Metrics) *gate {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	return &gate{
		primary:      primary,
		replicas:     replicas,
		interval:     cfg.PollInterval,
		jitter:       cfg.PollJitter,
		probeTimeout: cfg.ProbeTimeout,
		staleAfter:   cfg.SampleStaleAfter,
		maxLagBytes:  cfg.MaxLagBytes,
		log:          log,
		metrics:      instruments,
		notify:       make(chan struct{}),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// health returns a snapshot of every replica.
func (g *gate) health() []ReplicaHealth {
	if g == nil {
		return nil
	}

	out := make([]ReplicaHealth, 0, len(g.replicas))

	for _, node := range g.replicas {
		node.mu.RLock()
		state := node.state
		out = append(out, ReplicaHealth{
			Index:       node.index,
			Host:        node.host,
			Healthy:     state.eligible(g.staleAfter),
			Quarantined: state.quarantined(),
			InRecovery:  state.inRecovery(),
			ReplayLSN:   state.replayLSN,
			ReceiveLSN:  state.receiveLSN,
			LagBytes:    state.lagBytes,
			LastPoll:    state.lastPoll,
			Err:         state.lastErr,
		})
		node.mu.RUnlock()
	}

	return out
}

// eligible reports whether a node may serve reads at all. The caller must hold
// at least a read lock on node.
func (g *gate) eligible(node *replicaNode) bool {
	return node.state.eligible(g.staleAfter)
}

// pick chooses a replica able to serve a read requiring the given position, or
// returns -1 and the reason no replica qualified.
//
// A zero requirement means the caller has no watermark to satisfy. In that case
// the byte-lag threshold applies instead — with a concrete watermark it does
// not, because comparing WAL positions is strictly stronger than comparing lag.
func (g *gate) pick(required wal.LSN) replicaSelection {
	if g == nil || len(g.replicas) == 0 {
		return replicaSelection{qualification: replicaUnavailable}
	}

	best := replicaUnavailable
	// Bounded by the modulus, so the conversion cannot overflow.
	start := int(g.cursor.Add(1) % uint64(len(g.replicas))) //nolint:gosec // bounded by len(g.replicas)

	for offset := range g.replicas {
		node := g.replicas[(start+offset)%len(g.replicas)]

		node.mu.RLock()
		qualification := g.qualifies(node, required)
		node.mu.RUnlock()

		if qualification.eligible() {
			return replicaSelection{node: node, qualification: qualification}
		}

		// Report the most specific reason seen: "behind" tells a very
		// different operational story from "no healthy replica".
		if qualification == replicaBehind {
			best = qualification
		}
	}

	return replicaSelection{qualification: best}
}

// qualifies reports whether one node can serve a read requiring the given
// position. The caller must hold a read lock on node.
func (g *gate) qualifies(node *replicaNode, required wal.LSN) replicaQualification {
	return node.state.qualification(required, g.maxLagBytes, g.staleAfter)
}

// ready reports whether some replica has replayed past the given position.
func (g *gate) ready(required wal.LSN) bool {
	return g.pick(required).available()
}

// await waits, up to max, for a replica to become able to serve a read
// requiring the given position. It wakes on each completed poll round rather
// than on a fixed tick, so it returns as soon as the information exists.
//
// A bounded wait, not an unbounded one: a standby resolving a query conflict
// can freeze replay for up to max_standby_streaming_delay, and blocking that
// long would collide with the caller's own timeout and produce a worse error
// than the honest one.
func (g *gate) await(ctx context.Context, required wal.LSN, maxWait time.Duration) (bool, error) {
	started := time.Now()

	defer func() {
		g.metrics.GateWait(ctx, time.Since(started))
	}()

	if g.ready(required) {
		return true, nil
	}

	if maxWait <= 0 {
		return false, nil
	}

	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()

	for {
		// Take the wake channel before re-checking, so a sample landing
		// between the check and the select is not missed.
		wake := g.sampled()

		if g.ready(required) {
			return true, nil
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return g.ready(required), nil
		case <-wake:
		}
	}
}

// primaryPosition returns the primary's last sampled insert position.
//
// It is what clamps a client-supplied watermark: without the clamp, a crafted
// header pins every one of that client's reads to the primary for as long as
// they keep sending it.
func (g *gate) primaryPosition() primarySample {
	if g == nil {
		return primarySample{}
	}

	g.pmu.RLock()
	defer g.pmu.RUnlock()

	return primarySample{position: g.primaryLSN, observed: g.primaryAt}
}

// observePrimary raises the cached primary position from a live read.
//
// Router.Watermark issues that read, and its answer is strictly fresher than
// the poller's sample. Without this, a token minted moments ago names a
// position ahead of what the gate believes the primary has written, and the
// clamp in ResolveToken would treat our own watermark as forged.
func (g *gate) observePrimary(position wal.LSN) {
	if g == nil {
		return
	}

	g.pmu.Lock()

	if position > g.primaryLSN {
		g.primaryLSN = position
	}

	g.primaryAt = time.Now()
	g.pmu.Unlock()
}

// lineage returns the cluster and timeline the current samples belong to.
// A token from a different lineage is not comparable and must be discarded.
func (g *gate) lineage() clusterLineage {
	if g == nil {
		return clusterLineage{}
	}

	g.pmu.RLock()
	defer g.pmu.RUnlock()

	return clusterLineage{systemID: g.systemID, timeline: g.timeline}
}

func (g *gate) sampled() <-chan struct{} {
	g.notifyMu.Lock()
	defer g.notifyMu.Unlock()

	return g.notify
}

func (g *gate) broadcast() {
	g.notifyMu.Lock()
	close(g.notify)
	g.notify = make(chan struct{})
	g.notifyMu.Unlock()
}

// HostOf renders a pool's endpoint without credentials, which is what the
// health snapshot and the metric attributes carry.
func HostOf(pool *pgxpool.Pool) string {
	config := pool.Config().ConnConfig

	return net.JoinHostPort(config.Host, strconv.Itoa(int(config.Port)))
}

// markFailure records a statement-level failure against a replica, so that the
// router stops choosing it before the next poll round would notice.
func (g *gate) markFailure(index int, err error) {
	if g == nil || index < 0 || index >= len(g.replicas) {
		return
	}

	node := g.replicas[index]

	node.mu.Lock()
	node.state.recordFailure(err)
	node.mu.Unlock()
}

// snapshot renders the health of every replica in the shape the metrics
// package observes. It is a function value rather than a dependency, which is
// what keeps metrics from having to know about routing at all.
func (g *gate) snapshot() []metrics.Health {
	samples := g.health()
	out := make([]metrics.Health, 0, len(samples))

	for _, node := range samples {
		out = append(out, metrics.Health{Host: node.Host, LagBytes: node.LagBytes, Up: node.Healthy})
	}

	return out
}
