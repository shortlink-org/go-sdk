package replica

import (
	"errors"
	"fmt"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass"
	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// Operation names used by Error, so that a log line can be grepped for one
// phase of the lifecycle.
const (
	opInTx      = "in_tx"
	opWatermark = "watermark"
)

// Routing errors.
var (
	// ErrRouterDisabled indicates no replicas were configured.
	ErrRouterDisabled = errors.New("no postgres replicas configured")
	// ErrNoHealthyReplica indicates no replica can currently serve the read.
	ErrNoHealthyReplica = errors.New("no healthy postgres replica")
	// ErrWriteOnReplica indicates a write was issued under a replica-only
	// strategy. Promoting it to the primary would make the strategy a lie, and
	// sending it to a standby would fail with a worse message.
	ErrWriteOnReplica = errors.New("write statement under a replica-only strategy")
	// ErrPrimaryInRecovery indicates the node configured as the primary is
	// itself a standby — a cascading setup, or a failover in progress.
	ErrPrimaryInRecovery = errors.New("configured primary is in recovery")
	// ErrReplicaPromoted indicates a replica left recovery and was quarantined.
	ErrReplicaPromoted = errors.New("replica left recovery and was quarantined")
)

// Error reports a failure inside the router, with the phase it happened in.
type Error struct {
	Op      string
	Err     error
	Details string
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("postgres router error during %s: %s: %v", e.Op, e.Details, e.Err)
	}

	return fmt.Sprintf("postgres router error during %s: %v", e.Op, e.Err)
}

// Unwrap allows errors.Is and errors.As to reach the cause.
func (e *Error) Unwrap() error {
	return e.Err
}

// RoutingError reports a failure to choose a pool. It carries the inputs of
// the decision, which is what a log line needs in order to explain why a
// statement went where it went.
type RoutingError struct {
	Strategy  Strategy
	Class     sqlclass.Class
	Watermark wal.LSN
	Err       error
}

// Error implements the error interface.
func (e *RoutingError) Error() string {
	return fmt.Sprintf("postgres routing failed (strategy=%s class=%s watermark=%s): %v",
		e.Strategy, e.Class, e.Watermark, e.Err)
}

// Unwrap allows errors.Is and errors.As to reach the sentinel.
func (e *RoutingError) Unwrap() error {
	return e.Err
}

// ConfigError reports a replica whose server settings make read-routing
// unsound, so that startup fails instead of the feature silently doing nothing.
type ConfigError struct {
	Host    string
	Setting string
	Value   string
}

// Error implements the error interface.
func (e *ConfigError) Error() string {
	return fmt.Sprintf("replica %s has %s=%s, which no fresh watermark can ever satisfy",
		e.Host, e.Setting, e.Value)
}
