package couchbase

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

var ErrCouchbaseConnect = errors.New("failed to connect to Couchbase cluster")

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
