package badger

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for common Badger errors.
var (
	// ErrBadgerOpen indicates a failure to open the Badger database.
	ErrBadgerOpen = errors.New("failed to open Badger DB")
	// ErrBadgerClose indicates a failure to close the Badger database.
	ErrBadgerClose = errors.New("failed to close Badger DB")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
