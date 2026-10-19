package server

import "errors"

var (
	// ErrUnspecifiedStatus is reported when the raft node has no status set.
	// The election timer logs it and keeps running instead of stopping the node.
	ErrUnspecifiedStatus = errors.New("raft/server: raft status is unspecified")

	// ErrUnknownStatus is reported when the raft node has a status the
	// election timer does not know how to handle.
	ErrUnknownStatus = errors.New("raft/server: raft status is unknown")
)
