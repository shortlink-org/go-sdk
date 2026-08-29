package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
	"github.com/shortlink-org/go-sdk/logger"
)

// WithLogger gives the driver a logger. Without one, the replica poller stays
// silent about promotions and timeline switches, which are exactly the events
// you want to hear about.
//
// The db.New constructor wires this from its own deps; set it only when
// building the store directly.
func WithLogger(log logger.Logger) Option {
	return func(s *Store) {
		s.log = log
	}
}

// WithReplicaURI adds replica DSNs. Repeated calls append. With none, the
// router stays disabled and every statement runs on the primary.
//
// One DSN must resolve to one standby. Pointing it at a load balancer in front
// of several standbys breaks the guarantee: the health probe describes one node
// and the read reaches another.
func WithReplicaURI(uri ...string) Option {
	return func(s *Store) {
		s.routing.URIs = append(s.routing.URIs, uri...)
	}
}

// WithReplicaPoolConfig adjusts each replica's pool before it is built — sizing,
// timeouts, application_name. It runs after the primary's settings have been
// copied over, with the replica's index.
func WithReplicaPoolConfig(fn func(index int, cfg *pgxpool.Config)) Option {
	return func(s *Store) {
		s.routing.PoolConfig = fn
	}
}

// WithReplicaPollInterval sets how often replica positions are refreshed.
// Zero disables the poller, which leaves every replica ineligible.
func WithReplicaPollInterval(d time.Duration) Option {
	return func(s *Store) {
		s.routing.PollInterval = d
	}
}

// WithReplicaPollJitter spreads polls by the given fraction of the interval, so
// that a fleet of pods does not probe the same replica in lockstep. Values
// above one are clamped to one to keep timer durations positive.
func WithReplicaPollJitter(fraction float64) Option {
	return func(s *Store) {
		s.routing.PollJitter = fraction
	}
}

// WithReplicaProbeTimeout bounds a single health probe.
func WithReplicaProbeTimeout(d time.Duration) Option {
	return func(s *Store) {
		s.routing.ProbeTimeout = d
	}
}

// WithReplicaSampleStaleAfter sets the age past which a health sample is no
// longer trusted. A stale sample is indistinguishable from a hung replica.
func WithReplicaSampleStaleAfter(d time.Duration) Option {
	return func(s *Store) {
		s.routing.SampleStaleAfter = d
	}
}

// WithMaxReplicaLagBytes is the staleness budget for reads that carry no
// watermark. Zero means unlimited.
//
// It does not apply when a concrete watermark is being satisfied: comparing WAL
// positions is strictly stronger than comparing lag.
func WithMaxReplicaLagBytes(n int64) Option {
	return func(s *Store) {
		s.routing.MaxLagBytes = n
	}
}

// WithGateMaxWait bounds how long a caller waits inline for a replica to
// replay a required position. Zero disables waiting.
func WithGateMaxWait(d time.Duration) Option {
	return func(s *Store) {
		s.routing.GateMaxWait = d
	}
}

// WithNoTrackerPolicy decides where a read goes when its context carries
// neither a tracker nor an explicit strategy. Default replica.NoTrackerPrimary.
func WithNoTrackerPolicy(p replica.NoTrackerPolicy) Option {
	return func(s *Store) {
		s.routing.NoTracker = p
	}
}

// WithClassifier replaces the built-in statement classifier.
func WithClassifier(c sqlclass.Classifier) Option {
	return func(s *Store) {
		if c != nil {
			s.routing.Classifier = c
		}
	}
}

// WithReplicaFallback sets what a read does when no replica qualifies. The
// default is replica.FallbackToPrimary.
//
// Use replica.FallbackReject in tests, where a routing bug should be loud, or in a service
// that would rather shed read load than melt the primary. It never affects
// StrategyReplica, which never falls back either way.
func WithReplicaFallback(policy replica.FallbackPolicy) Option {
	return func(s *Store) {
		s.routing.Fallback = policy
	}
}

// WithWatermarkPolicy sets when a committed transaction resolves its WAL
// position. WatermarkOnCommit costs one extra primary round trip per commit.
//
// On-handoff capture is the default. In-request read-after-write needs only the taint, and a
// cross-boundary handoff is better served by calling Router.Watermark once,
// where the caller can see what it costs.
func WithWatermarkPolicy(policy replica.WatermarkPolicy) Option {
	return func(s *Store) {
		s.routing.Watermark = policy
	}
}

// WithTxLookup lets the router honor a transaction the application manages
// itself. Wire it at the composition root:
//
//	postgres.With(postgres.WithTxLookup(uow.FromContext))
//
// Without it the router cannot tell that a transaction is in flight, and will
// happily run the statement on a different connection — outside the
// transaction, without its locks, and able to deadlock against it. This is the
// single most dangerous way to use the router, so wire the hook.
func WithTxLookup(fn func(ctx context.Context) pgx.Tx) Option {
	return func(s *Store) {
		s.routing.TxLookup = fn
	}
}
