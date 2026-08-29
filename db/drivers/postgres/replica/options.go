package replica

import (
	"context"
	"fmt"
	"math"
	"strings"
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
	Fallback   FallbackPolicy
	Watermark  WatermarkPolicy
	TxLookup   func(ctx context.Context) pgx.Tx
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
		Fallback:         FallbackToPrimary,
	}
}

// Validate checks the fully resolved routing configuration. Callers are
// expected to merge defaults, environment values and functional options
// first; validation then describes the value that would actually be used.
func (o Options) Validate() error {
	switch {
	case o.PollInterval < 0:
		return invalidOption("PollInterval", o.PollInterval, "zero or a positive duration")
	case math.IsNaN(o.PollJitter) || math.IsInf(o.PollJitter, 0) || o.PollJitter < 0 || o.PollJitter > 1:
		return invalidOption("PollJitter", o.PollJitter, "a finite fraction between 0 and 1")
	case o.ProbeTimeout <= 0:
		return invalidOption("ProbeTimeout", o.ProbeTimeout, "a positive duration")
	case o.SampleStaleAfter <= 0:
		return invalidOption("SampleStaleAfter", o.SampleStaleAfter, "a positive duration")
	case o.MaxLagBytes < 0:
		return invalidOption("MaxLagBytes", o.MaxLagBytes, "zero or a positive byte count")
	case o.GateMaxWait < 0:
		return invalidOption("GateMaxWait", o.GateMaxWait, "zero or a positive duration")
	case !o.NoTracker.valid():
		return invalidOption("NoTracker", o.NoTracker, "a known no-tracker policy")
	case !o.Fallback.valid():
		return invalidOption("Fallback", o.Fallback, "a known fallback policy")
	case !o.Watermark.valid():
		return invalidOption("Watermark", o.Watermark, "a known watermark policy")
	case o.Classifier == nil:
		return invalidOption("Classifier", nil, "a non-nil classifier")
	}

	for index, uri := range o.URIs {
		if strings.TrimSpace(uri) == "" {
			return invalidOption(
				"URIs",
				fmt.Sprintf("entry %d is empty", index),
				"only non-empty replica connection strings",
			)
		}
	}

	return nil
}

func invalidOption(option string, value any, constraint string) error {
	return &OptionError{Option: option, Value: value, Constraint: constraint}
}
