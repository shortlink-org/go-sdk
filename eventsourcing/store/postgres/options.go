package es_postgres

const (
	defaultSnapshotBatchSize = 1000
	defaultSnapshotThreshold = 1
)

type options struct {
	skipMigrations    bool
	snapshotBatchSize uint64
	snapshotThreshold int32
}

// Option configures an EventStore.
type Option func(*options)

// WithoutMigrations stops New from applying the schema, for applications that
// run without DDL privileges and apply Migrations themselves.
func WithoutMigrations() Option {
	return func(o *options) {
		o.skipMigrations = true
	}
}

// WithSnapshotBatchSize sets how many aggregates one call of
// GetAggregateWithoutSnapshot returns, 1000 by default.
func WithSnapshotBatchSize(size uint64) Option {
	return func(o *options) {
		if size > 0 {
			o.snapshotBatchSize = size
		}
	}
}

// WithSnapshotThreshold sets how many events an aggregate must gain since its
// last snapshot before GetAggregateWithoutSnapshot returns it, 1 by default.
func WithSnapshotThreshold(events int32) Option {
	return func(o *options) {
		if events > 0 {
			o.snapshotThreshold = events
		}
	}
}
