package replica

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
)

// Defaults for read-replica routing. They are deliberately conservative: the
// feature costs nothing when unused and gives up throughput rather than
// correctness when the topology misbehaves.
const (
	defaultPollInterval     = 250 * time.Millisecond
	defaultPollJitter       = 0.15
	defaultProbeTimeout     = 500 * time.Millisecond
	defaultSampleStaleAfter = 2 * time.Second
	defaultMaxLagBytes      = 8 << 20 // 8 MiB
	defaultGateMaxWait      = 250 * time.Millisecond
)

// Options is the resolved routing configuration. The driver assembles it from
// defaults, then configuration keys, then functional options, and hands it to
// New.
type Options struct {
	URIs       []string
	PoolConfig func(index int, cfg *pgxpool.Config)

	PollInterval     time.Duration
	PollJitter       float64
	ProbeTimeout     time.Duration
	SampleStaleAfter time.Duration
	MaxLagBytes      int64
	GateMaxWait      time.Duration

	NoTracker  NoTrackerPolicy
	Classifier sqlclass.Classifier
	Fallback   bool

	SyncWatermark bool
	TxLookup      func(ctx context.Context) pgx.Tx
}

// DefaultOptions returns the conservative defaults.
func DefaultOptions() Options {
	return Options{
		PollInterval:     defaultPollInterval,
		PollJitter:       defaultPollJitter,
		ProbeTimeout:     defaultProbeTimeout,
		SampleStaleAfter: defaultSampleStaleAfter,
		MaxLagBytes:      defaultMaxLagBytes,
		GateMaxWait:      defaultGateMaxWait,
		NoTracker:        NoTrackerPrimary,
		Classifier:       sqlclass.DefaultClassifier(),
		Fallback:         true,
	}
}
