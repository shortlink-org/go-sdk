package aerospike

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidURI indicates an invalid Aerospike URI.
	ErrInvalidURI = errors.New("invalid Aerospike URI")
	// ErrInvalidPort indicates a failure during port conversion.
	ErrInvalidPort = errors.New("invalid port")
	// ErrClientConnection indicates a failure to connect to Aerospike.
	ErrClientConnection = errors.New("failed to connect to Aerospike client")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
