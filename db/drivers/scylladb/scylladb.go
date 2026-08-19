package scylladb

import (
	"context"
	"strings"

	gocql "github.com/apache/cassandra-gocql-driver/v2"

	"github.com/shortlink-org/go-sdk/config"
)

// New creates a ScyllaDB store configured via cfg.
func New(cfg *config.Config) *Store {
	return &Store{cfg: cfg}
}

// Init opens a CQL session and verifies connectivity.
func (s *Store) Init(ctx context.Context) error {
	s.setConfig()

	if len(s.config.Hosts) == 0 {
		return &StoreError{
			Driver:  driverName,
			Op:      "init",
			Err:     ErrInvalidHosts,
			Details: "STORE_SCYLLADB_HOSTS is empty",
		}
	}

	cluster := gocql.NewCluster(s.config.Hosts...)
	cluster.Consistency = s.config.Consistency
	cluster.Keyspace = s.config.Keyspace

	var err error

	s.session, err = cluster.CreateSession()
	if err != nil {
		return &StoreError{
			Driver:  driverName,
			Op:      "init",
			Err:     ErrClientConnection,
			Details: err.Error(),
		}
	}

	var version string

	// IterContext replaces the deprecated WithContext; Scan reports whether a
	// row was read, and Close carries the error.
	iter := s.session.Query("SELECT release_version FROM system.local").IterContext(ctx)
	iter.Scan(&version)

	err = iter.Close()
	if err != nil {
		s.session.Close()
		s.session = nil

		return &PingConnectionError{Driver: driverName, Err: err}
	}

	go func() {
		<-ctx.Done()

		s.close()
	}()

	return nil
}

// GetConn returns the *gocql.Session.
func (s *Store) GetConn() any {
	return s.session
}

func (s *Store) close() {
	if s.session == nil {
		return
	}

	s.session.Close()
	s.session = nil
}

func (s *Store) setConfig() {
	s.cfg.SetDefault("STORE_SCYLLADB_HOSTS", "127.0.0.1:9042")
	s.cfg.SetDefault("STORE_SCYLLADB_KEYSPACE", "")
	s.cfg.SetDefault("STORE_SCYLLADB_CONSISTENCY", "ONE")

	hostsStr := strings.TrimSpace(s.cfg.GetString("STORE_SCYLLADB_HOSTS"))

	var hosts []string

	for h := range strings.SplitSeq(hostsStr, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			hosts = append(hosts, h)
		}
	}

	consistency, err := gocql.ParseConsistencyWrapper(s.cfg.GetString("STORE_SCYLLADB_CONSISTENCY"))
	if err != nil {
		consistency = gocql.One
	}

	s.config = Config{
		Hosts:       hosts,
		Keyspace:    s.cfg.GetString("STORE_SCYLLADB_KEYSPACE"),
		Consistency: consistency,
	}
}
