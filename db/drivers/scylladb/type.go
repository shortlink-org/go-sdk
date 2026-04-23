package scylladb

import (
	"github.com/gocql/gocql"

	"github.com/shortlink-org/go-sdk/config"
)

// Config holds ScyllaDB / Cassandra CQL client settings.
type Config struct {
	Hosts       []string
	Keyspace    string
	Consistency gocql.Consistency
}

// Store implements db.DB using a gocql session.
type Store struct {
	session *gocql.Session
	config  Config
	cfg     *config.Config
}
