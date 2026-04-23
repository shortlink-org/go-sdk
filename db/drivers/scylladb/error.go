package scylladb

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidHosts     = errors.New("invalid ScyllaDB hosts")
	ErrClientConnection = errors.New("failed to connect ScyllaDB client")
)

type StoreError struct {
	Op      string
	Err     error
	Details string
}

func (e *StoreError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("scylladb store error during %s: %s: %v", e.Op, e.Details, e.Err)
	}

	return fmt.Sprintf("scylladb store error during %s: %v", e.Op, e.Err)
}

func (e *StoreError) Unwrap() error {
	return e.Err
}

type PingConnectionError struct {
	Err error
}

func (e *PingConnectionError) Error() string {
	return "failed to ping ScyllaDB: " + e.Err.Error()
}
