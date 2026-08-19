package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
)

// applicationNamePrimary labels the primary pool's connections, so that
// pg_stat_activity can confirm where a statement actually ran.
const applicationNamePrimary = "-primary"

// buildRouter assembles the read-replica router. With no replica DSNs it
// returns a router that owns only the primary, and every statement runs where
// it ran before.
func (s *Store) buildRouter(ctx context.Context) (*replica.Router, error) {
	applicationName(s.config.config, applicationNamePrimary)

	return replica.Open(ctx, &replica.Config{
		Primary:       s.client,
		PrimaryConfig: s.config.config,
		Options:       s.routing,
		Log:           s.log,
		Meter:         s.metrics,
		Instrument:    func(cfg *pgxpool.Config) { instrument(cfg, &s.tracer) },
	})
}

// applicationName appends a suffix to whatever the DSN already set, so that a
// deliberately chosen application_name is extended rather than discarded.
func applicationName(target *pgxpool.Config, suffix string) {
	base := target.ConnConfig.RuntimeParams["application_name"]
	if base == "" {
		return
	}

	target.ConnConfig.RuntimeParams["application_name"] = base + suffix
}
