package clickhouse

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Common error variables for Clickhouse store operations.
var (
	// ErrPing indicates that a Ping to the Clickhouse database failed.
	ErrPing = errors.New("failed to ping Clickhouse database")
	// ErrClose indicates that closing the Clickhouse connection failed.
	ErrClose = errors.New("failed to close Clickhouse connection")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
