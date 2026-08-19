package postgres

import (
	"github.com/shortlink-org/go-sdk/db"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica"
)

// RouterFrom returns the read-replica router of a postgres store.
//
//	router, err := postgres.RouterFrom(store)
//
// It reports ErrNotPostgresStore when another driver is in use, and
// replica.ErrRouterDisabled when no replicas were configured.
//
// It deliberately does not return a nil router that quietly forwards
// everything to the primary. That shape is convenient right up to the day
// somebody spends an afternoon asking why the replica is idle.
func RouterFrom(store db.DB) (*replica.Router, error) {
	driver, direct := store.(*Store)
	if !direct {
		// db.Store embeds the driver as an interface field, so unwrap it.
		wrapper, wrapped := store.(*db.Store)
		if !wrapped {
			return nil, ErrNotPostgresStore
		}

		unwrapped, isPostgres := wrapper.DB.(*Store)
		if !isPostgres {
			return nil, ErrNotPostgresStore
		}

		driver = unwrapped
	}

	if driver.router == nil || !driver.router.Enabled() {
		return nil, replica.ErrRouterDisabled
	}

	return driver.router, nil
}

// Router returns the store's router, or nil when Init has not run.
//
// Unlike RouterFrom it does not insist that replicas are configured: a
// disabled router is still usable and sends every statement to the primary.
func (s *Store) Router() *replica.Router {
	return s.router
}
