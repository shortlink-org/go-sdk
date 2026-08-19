package edgedb

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

var (
	ErrConnect = errors.New("failed to connect to EdgeDB")
	ErrClose   = errors.New("failed to close EdgeDB connection")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
