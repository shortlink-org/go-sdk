package etcd

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidURI indicates an invalid etcd URI.
	ErrInvalidURI = errors.New("invalid etcd URI")
	// ErrInvalidEndpoints indicates invalid etcd endpoints.
	ErrInvalidEndpoints = errors.New("invalid etcd endpoints")
	// ErrClientConnection indicates a failure to connect to etcd.
	ErrClientConnection = errors.New("failed to connect to etcd client")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
