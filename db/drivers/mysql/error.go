package mysql

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidDSN indicates an invalid MySQL DSN.
	ErrInvalidDSN = errors.New("invalid MySQL DSN")
	// ErrClientConnection indicates a failure to connect to MySQL.
	ErrClientConnection = errors.New("failed to connect to MySQL server")
	// ErrInvalidDatabase indicates an invalid database name.
	ErrInvalidDatabase = errors.New("invalid database name")
	// ErrInvalidCredentials indicates invalid authentication credentials.
	ErrInvalidCredentials = errors.New("invalid MySQL credentials")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
