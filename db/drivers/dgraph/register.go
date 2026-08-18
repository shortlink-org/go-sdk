package dgraph

import (
	"github.com/shortlink-org/go-sdk/db"
)

// driverName is the STORE_TYPE value this driver answers to.
const driverName = "dgraph"

//nolint:gochecknoinits // driver registration
func init() {
	db.Register(driverName, func(deps db.Deps) (db.DB, error) {
		return New(deps.Log, deps.Cfg), nil
	})
}
