package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// reapSQL deletes one bounded batch of rows that have been delivered for
// longer than the retention window. The bound matters: a table nobody cleaned
// for a year should drain over several passes rather than in one statement
// that locks everything it touches.
//
//nolint:gosec // tableName is a package constant, not user input
var reapSQL = fmt.Sprintf(
	`DELETE FROM %[1]s
	  WHERE ctid IN (
	        SELECT ctid
	          FROM %[1]s
	         WHERE delivered_at IS NOT NULL
	           AND delivered_at < now() - ($1::BIGINT * INTERVAL '1 microsecond')
	         ORDER BY delivered_at
	         LIMIT $2
	  )`,
	tableName,
)

// reap removes delivered rows past the retention window until ctx is done.
//
// Without it the outbox is an append-only log of everything the service ever
// emitted, and it becomes the largest table in the database within a year.
func (r *Relay) reap(ctx context.Context) error {
	if r.opts.Retention <= 0 {
		return nil
	}

	ticker := time.NewTicker(r.opts.ReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		removed, err := r.reapOnce(ctx)
		if err != nil {
			// Shutdown cancels the context mid-delete; that is not a failure.
			if ctx.Err() != nil {
				return nil
			}

			r.log.Error("outbox: failed to remove delivered messages",
				slog.String("error", err.Error()),
			)

			continue
		}

		if removed > 0 {
			r.log.Info("outbox: removed delivered messages",
				slog.Int64("count", removed),
				slog.String("retention", r.opts.Retention.String()),
			)
		}
	}
}

// reapOnce deletes at most one batch and reports how many rows went.
func (r *Relay) reapOnce(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, reapSQL, r.opts.Retention.Microseconds(), r.opts.ReapBatchSize)
	if err != nil {
		return 0, fmt.Errorf("outbox: reap: %w", err)
	}

	return tag.RowsAffected(), nil
}
