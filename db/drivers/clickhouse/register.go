package clickhouse

import (
	"github.com/shortlink-org/go-sdk/db"
)

// driverName is the STORE_TYPE value this driver answers to.
const driverName = "clickhouse"

//nolint:gochecknoinits // driver registration
func init() {
	db.Register(driverName, func(deps db.Deps) (db.DB, error) {
		return New(deps.Cfg), nil
	})
}
