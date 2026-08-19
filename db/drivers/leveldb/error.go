package leveldb

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidPath indicates an invalid LevelDB path.
	ErrInvalidPath = errors.New("invalid LevelDB path")
	// ErrDatabaseOpen indicates a failure to open the database.
	ErrDatabaseOpen = errors.New("failed to open LevelDB database")
	// ErrDatabaseClosed indicates operations on a closed database.
	ErrDatabaseClosed = errors.New("database is closed")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
