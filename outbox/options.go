package outbox

import "time"

// Defaults for the relay's read loop and its reaper.
const (
	defaultBatchSize     = 100
	defaultPollInterval  = 500 * time.Millisecond
	defaultRetention     = 7 * 24 * time.Hour
	defaultReapInterval  = time.Hour
	defaultReapBatchSize = 1000
)

// tableName is fixed on purpose.
//
// A per-topic table would have to derive an identifier from a topic name, and
// a topic like "auth.user" then becomes the quoted identifier
// "outbox_auth.user" — something that reads as schema-qualified and is not.
// One table with a topic column has no names to derive, and it is the table
// the migrations in this package create.
const tableName = "outbox"

// Options configure the relay and the publisher.
type Options struct {
	// BatchSize is how many rows one read claims. It bounds both the
	// concurrency of dispatch and how long the claiming transaction stays
	// open, because the transaction is held until the whole batch is
	// acknowledged.
	BatchSize int

	// PollInterval is the wait after a read that found nothing.
	PollInterval time.Duration

	// Retention is how long a delivered row is kept before the reaper
	// removes it. Zero keeps rows forever, and makes cleanup the service's
	// problem.
	Retention time.Duration

	// ReapInterval is how often the reaper runs.
	ReapInterval time.Duration

	// ReapBatchSize bounds one delete, so that a long-neglected table is
	// drained over several passes instead of in one lock-heavy statement.
	ReapBatchSize int
}

// Option configures the outbox.
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		BatchSize:     defaultBatchSize,
		PollInterval:  defaultPollInterval,
		Retention:     defaultRetention,
		ReapInterval:  defaultReapInterval,
		ReapBatchSize: defaultReapBatchSize,
	}
}

//nolint:gocritic // Options is public API; callers build it by value
func applyOptions(opts []Option) Options {
	options := defaultOptions()

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		opt(&options)
	}

	if options.BatchSize <= 0 {
		options.BatchSize = defaultBatchSize
	}

	if options.PollInterval <= 0 {
		options.PollInterval = defaultPollInterval
	}

	if options.ReapInterval <= 0 {
		options.ReapInterval = defaultReapInterval
	}

	if options.ReapBatchSize <= 0 {
		options.ReapBatchSize = defaultReapBatchSize
	}

	if options.Retention < 0 {
		options.Retention = 0
	}

	return options
}

// WithBatchSize sets how many rows one read claims.
func WithBatchSize(size int) Option {
	return func(o *Options) {
		o.BatchSize = size
	}
}

// WithPollInterval sets the wait after a read that found nothing.
func WithPollInterval(interval time.Duration) Option {
	return func(o *Options) {
		o.PollInterval = interval
	}
}

// WithRetention sets how long a delivered row is kept. The default is seven
// days: long enough to answer "did we send it?" during an incident, short
// enough that the table does not become the largest one in the database.
func WithRetention(ttl time.Duration) Option {
	return func(o *Options) {
		o.Retention = ttl
	}
}

// WithoutRetention keeps delivered rows forever and stops the reaper. Use it
// when the service cleans the table itself — partitions, pg_cron — and knows
// it does.
func WithoutRetention() Option {
	return func(o *Options) {
		o.Retention = 0
	}
}

// WithReapInterval sets how often the reaper runs.
func WithReapInterval(interval time.Duration) Option {
	return func(o *Options) {
		o.ReapInterval = interval
	}
}

// WithReapBatchSize bounds how many rows one reaper pass deletes.
func WithReapBatchSize(size int) Option {
	return func(o *Options) {
		o.ReapBatchSize = size
	}
}
