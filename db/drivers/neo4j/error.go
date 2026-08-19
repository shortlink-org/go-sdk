package neo4j

import (
	"errors"

	"github.com/shortlink-org/go-sdk/db"
)

// Error variables for wrapping underlying errors.
var (
	// ErrInvalidURI indicates an invalid Neo4j URI.
	ErrInvalidURI = errors.New("invalid Neo4j URI")
	// ErrClientConnection indicates a failure to connect to Neo4j.
	ErrClientConnection = errors.New("failed to connect to Neo4j server")
	// ErrInvalidCredentials indicates invalid authentication credentials.
	ErrInvalidCredentials = errors.New("invalid Neo4j credentials")
	// ErrCypherQuery indicates an invalid Cypher query.
	ErrCypherQuery = errors.New("invalid Cypher query")
)

// The lifecycle error types are shared by every driver, so that a fix reaches
// all of them at once. Aliases rather than new types: existing callers keep
// working with *StoreError unchanged.
type (
	StoreError          = db.StoreError
	PingConnectionError = db.PingConnectionError
)
