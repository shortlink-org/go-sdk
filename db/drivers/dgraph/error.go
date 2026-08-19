package dgraph

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

var (
	ErrDgraphClient  = errors.New("failed to create Dgraph gRPC client")
	ErrDgraphMigrate = errors.New("failed to migrate Dgraph schema")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
