package mongo

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidURI indicates an invalid MongoDB URI.
	ErrInvalidURI = errors.New("invalid MongoDB URI")
	// ErrClientConnection indicates a failure to connect to MongoDB.
	ErrClientConnection = errors.New("failed to connect to MongoDB client")
	// ErrInvalidDatabase indicates an invalid database name.
	ErrInvalidDatabase = errors.New("invalid database name")
	// ErrInvalidCollection indicates an invalid collection name.
	ErrInvalidCollection = errors.New("invalid collection name")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
