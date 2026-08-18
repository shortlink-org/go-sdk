package mysql

import (
	"github.com/shortlink-org/go-sdk/db"
)

// driverName is the STORE_TYPE value this driver answers to.
const driverName = "mysql"

//nolint:gochecknoinits // driver registration
func init() {
	db.Register(driverName, func(deps db.Deps) (db.DB, error) {
		return New(deps.Tracer, deps.Metrics, deps.Cfg), nil
	})
}
