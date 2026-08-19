package sqlite

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidPath indicates an invalid SQLite database path.
	ErrInvalidPath = errors.New("invalid SQLite database path")
	// ErrClientConnection indicates a failure to connect to SQLite.
	ErrClientConnection = errors.New("failed to connect to SQLite database")
	// ErrInvalidConfiguration indicates invalid SQLite configuration.
	ErrInvalidConfiguration = errors.New("invalid SQLite configuration")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
