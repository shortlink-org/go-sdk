package redis

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidURI indicates an invalid Redis URI.
	ErrInvalidURI = errors.New("invalid Redis URI")
	// ErrInvalidCredentials indicates invalid Redis credentials.
	ErrInvalidCredentials = errors.New("invalid Redis credentials")
	// ErrClientConnection indicates a failure to connect to Redis.
	ErrClientConnection = errors.New("failed to connect to Redis client")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
