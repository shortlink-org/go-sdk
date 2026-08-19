package postgres

import (
	"strings"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
)

// Config keys for read-replica routing. All are optional; leaving
// STORE_POSTGRES_REPLICA_URI empty keeps the driver exactly as it was.
const (
	cfgReplicaURI              = "STORE_POSTGRES_REPLICA_URI"
	cfgReplicaPollInterval     = "STORE_POSTGRES_REPLICA_POLL_INTERVAL"
	cfgReplicaPollJitter       = "STORE_POSTGRES_REPLICA_POLL_JITTER"
	cfgReplicaProbeTimeout     = "STORE_POSTGRES_REPLICA_PROBE_TIMEOUT"
	cfgReplicaSampleStaleAfter = "STORE_POSTGRES_REPLICA_SAMPLE_STALE_AFTER"
	cfgReplicaMaxLagBytes      = "STORE_POSTGRES_REPLICA_MAX_LAG_BYTES"
	cfgReplicaNoTrackerPolicy  = "STORE_POSTGRES_REPLICA_NO_TRACKER_POLICY"
	cfgGateMaxWait             = "STORE_POSTGRES_GATE_MAX_WAIT"
)

// routingFromConfig fills the routing options from configuration, starting
// from the defaults. It runs before the functional options, so an option
// always wins over an environment variable.
func routingFromConfig(cfg *config.Config) replica.Options {
	opts := replica.DefaultOptions()
	if cfg == nil {
		return opts
	}

	cfg.SetDefault(cfgReplicaURI, "")
	cfg.SetDefault(cfgReplicaPollInterval, opts.PollInterval)
	cfg.SetDefault(cfgReplicaPollJitter, opts.PollJitter)
	cfg.SetDefault(cfgReplicaProbeTimeout, opts.ProbeTimeout)
	cfg.SetDefault(cfgReplicaSampleStaleAfter, opts.SampleStaleAfter)
	cfg.SetDefault(cfgReplicaMaxLagBytes, opts.MaxLagBytes)
	cfg.SetDefault(cfgReplicaNoTrackerPolicy, replica.NoTrackerPrimary.String())
	cfg.SetDefault(cfgGateMaxWait, opts.GateMaxWait)

	opts.URIs = splitURIs(cfg.GetString(cfgReplicaURI))
	opts.PollInterval = cfg.GetDuration(cfgReplicaPollInterval)
	opts.PollJitter = cfg.GetFloat64(cfgReplicaPollJitter)
	opts.ProbeTimeout = cfg.GetDuration(cfgReplicaProbeTimeout)
	opts.SampleStaleAfter = cfg.GetDuration(cfgReplicaSampleStaleAfter)
	opts.MaxLagBytes = cfg.GetInt64(cfgReplicaMaxLagBytes)
	opts.GateMaxWait = cfg.GetDuration(cfgGateMaxWait)

	if strings.EqualFold(cfg.GetString(cfgReplicaNoTrackerPolicy), replica.NoTrackerReplica.String()) {
		opts.NoTracker = replica.NoTrackerReplica
	}

	return opts
}

// splitURIs parses a comma-separated DSN list, tolerating spaces and trailing
// separators so that a list assembled by a deployment template does not need to
// be perfectly formed.
func splitURIs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}
