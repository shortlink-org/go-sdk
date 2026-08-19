package replica

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/metrics"
	"github.com/shortlink-org/go-sdk/logger"
)

// applicationNameReplica labels each replica pool's connections. It is the
// cheapest way to confirm from pg_stat_activity that routing is actually
// happening, rather than trusting a metric that could be counting decisions
// nobody acted on.
const applicationNameReplica = "-replica-%d"

// opOpen names the construction phase in an Error.
const opOpen = "open"

// Config is what Open needs from the driver.
type Config struct {
	// Primary is the already-open primary pool.
	Primary *pgxpool.Pool

	// PrimaryConfig is the configuration the primary was built from. Replica
	// pools inherit its sizing, lifetimes and registered types.
	PrimaryConfig *pgxpool.Config

	Options Options
	Log     logger.Logger
	Meter   *sdkmetric.MeterProvider

	// Instrument wires tracing into a pool config, so a replica query is traced
	// identically to a primary one. The driver owns the tracer, so it supplies
	// this rather than the router reaching for it.
	Instrument func(*pgxpool.Config)
}

// Open builds a router over the primary and the configured replicas.
//
// With no replica DSNs it returns a router that owns only the primary: Enabled
// reports false and every statement runs where it ran before.
func Open(ctx context.Context, cfg *Config) (*Router, error) {
	instruments, err := metrics.New(cfg.Meter)
	if err != nil {
		return nil, err
	}

	nodes, err := buildPools(ctx, cfg)
	if err != nil {
		return nil, err
	}

	gate := newGate(cfg.Primary, nodes, &cfg.Options, cfg.Log, instruments)

	router := &Router{
		primary: cfg.Primary,
		gate:    gate,
		metrics: instruments,
		opts:    cfg.Options,
	}

	if len(nodes) == 0 {
		return router, nil
	}

	err = verify(ctx, gate, nodes, cfg.Log, &cfg.Options)
	if err != nil {
		closePools(nodes)

		return nil, err
	}

	gate.resolveSystemID(ctx)
	gate.start(ctx)

	err = instruments.Observe(cfg.Meter, gate.snapshot)
	if err != nil {
		gate.close()

		return nil, err
	}

	return router, nil
}

// buildPools opens one pool per replica DSN.
//
// Each DSN is parsed on its own. Reusing the primary's *pgxpool.Config object
// and only swapping the host would be the obvious shortcut, and it silently
// connects every pool to the primary: everything works, the metrics look
// plausible, and nothing tells you.
func buildPools(ctx context.Context, cfg *Config) ([]*replicaNode, error) {
	if len(cfg.Options.URIs) == 0 {
		return nil, nil
	}

	primary := cfg.PrimaryConfig
	nodes := make([]*replicaNode, 0, len(cfg.Options.URIs))

	for index, uri := range cfg.Options.URIs {
		poolConfig, err := pgxpool.ParseConfig(uri)
		if err != nil {
			closePools(nodes)

			return nil, &Error{
				Op:      opOpen,
				Err:     err,
				Details: fmt.Sprintf("failed to parse replica %d connection config", index),
			}
		}

		if cfg.Instrument != nil {
			cfg.Instrument(poolConfig)
		}

		// Carry over what the primary was configured with, so a replica pool
		// is not accidentally a different animal: the same registered types,
		// the same sizing, the same connection lifetimes.
		poolConfig.AfterConnect = primary.AfterConnect
		poolConfig.MaxConns = primary.MaxConns
		poolConfig.MinConns = primary.MinConns
		poolConfig.MaxConnLifetime = primary.MaxConnLifetime
		poolConfig.MaxConnIdleTime = primary.MaxConnIdleTime
		poolConfig.HealthCheckPeriod = primary.HealthCheckPeriod

		applicationName(poolConfig, fmt.Sprintf(applicationNameReplica, index), primary)

		if cfg.Options.PoolConfig != nil {
			cfg.Options.PoolConfig(index, poolConfig)
		}

		pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			closePools(nodes)

			return nil, &Error{
				Op:      opOpen,
				Err:     err,
				Details: fmt.Sprintf("failed to open replica %d", index),
			}
		}

		nodes = append(nodes, &replicaNode{pool: pool, host: HostOf(pool), index: index})
	}

	return nodes, nil
}

// verify rejects a topology that cannot support read routing, so that startup
// fails loudly instead of the feature quietly doing nothing.
func verify(ctx context.Context, gate *gate, nodes []*replicaNode, log logger.Logger, opts *Options) error {
	errs := make([]error, 0, len(nodes))

	for _, node := range nodes {
		err := node.pool.Ping(ctx)
		if err != nil {
			errs = append(errs, &Error{
				Op:      opOpen,
				Err:     err,
				Details: "failed to reach replica " + node.host,
			})

			continue
		}

		errs = append(errs, gate.checkApplyDelay(ctx, node))
	}

	err := errors.Join(errs...)
	if err != nil {
		return err
	}

	if log != nil {
		log.Info("postgres read-replica routing enabled",
			slog.Int("replicas", len(nodes)),
			slog.String("no_tracker_policy", opts.NoTracker.String()),
			slog.Duration("poll_interval", opts.PollInterval),
		)
	}

	return nil
}

// applicationName appends a suffix to whatever the DSN already set, so that a
// deliberately chosen application_name is extended rather than discarded.
func applicationName(target *pgxpool.Config, suffix string, primary *pgxpool.Config) {
	base := primary.ConnConfig.RuntimeParams["application_name"]
	if base == "" {
		return
	}

	target.ConnConfig.RuntimeParams["application_name"] = base + suffix
}

func closePools(nodes []*replicaNode) {
	for _, node := range nodes {
		node.pool.Close()
	}
}
