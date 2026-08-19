package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/shortlink-org/go-sdk/db"
)

// Phases a StoreError can name. Naming them here rather than at each call site
// keeps the set a log query can filter on visible in one place.
const (
	opConfig  = "config"
	opConnect = "connect"
)

// Kinds of failure, for callers that want to branch rather than log.
//
// Each is reachable with errors.Is on whatever Init returns: the driver
// classifies the underlying pgx error and wraps the matching sentinel
// alongside the cause, so both are found.
//
//	err := store.Init(ctx)
//	switch {
//	case errors.Is(err, postgres.ErrInvalidCredentials):
//		// the password is wrong, and retrying will not help
//	case errors.Is(err, postgres.ErrClientConnection):
//		// the server is unreachable, and retrying might
//	}
var (
	// ErrInvalidDSN indicates a connection string PostgreSQL cannot parse.
	ErrInvalidDSN = errors.New("invalid PostgreSQL DSN")
	// ErrClientConnection indicates a failure to reach the PostgreSQL server.
	ErrClientConnection = errors.New("failed to connect to PostgreSQL server")
	// ErrInvalidDatabase indicates the named database does not exist.
	ErrInvalidDatabase = errors.New("invalid database name")
	// ErrInvalidCredentials indicates the server rejected the credentials.
	ErrInvalidCredentials = errors.New("invalid PostgreSQL credentials")
	// ErrInvalidSchema indicates the named schema does not exist.
	ErrInvalidSchema = errors.New("invalid schema name")
	// ErrNotPostgresStore indicates the store is backed by another driver.
	ErrNotPostgresStore = errors.New("store is not backed by the postgres driver")
)

// storeError builds a StoreError carrying both the kind and the cause.
//
// Wrapping the two is what lets a caller ask errors.Is about the sentinel
// without losing the pgx error underneath, which is the one worth logging.
func storeError(phase string, kind, cause error, details string) *StoreError {
	err := cause

	switch {
	case kind == nil:
	case cause == nil:
		err = kind
	default:
		err = fmt.Errorf("%w: %w", kind, cause)
	}

	return &StoreError{Driver: driverName, Op: phase, Err: err, Details: details}
}

// classify maps a PostgreSQL failure onto the sentinel that describes it.
//
// The SQLSTATE code is the only reliable signal: message text is localized and
// changes between releases, so matching on it is how error handling quietly
// stops working after an upgrade. A failure the server gave no recognized code
// for is reported as a connection failure, which is the honest answer — we
// know the connection did not come up, not why.
func classify(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ErrClientConnection
	}

	switch pgErr.Code {
	case pgerrcode.InvalidCatalogName:
		return ErrInvalidDatabase

	case pgerrcode.InvalidPassword,
		pgerrcode.InvalidAuthorizationSpecification:
		return ErrInvalidCredentials

	case pgerrcode.InvalidSchemaName:
		return ErrInvalidSchema

	default:
		return ErrClientConnection
	}
}

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
