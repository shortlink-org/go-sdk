package batch

import (
	"context"

	"github.com/shortlink-org/go-sdk/config"
)

// NewSync creates a batch that runs in the background like New,
// but it blocks until the passed context is canceled and all
// pending items have been processed. The first error returned
// by the user-supplied callback (if any) is propagated as the
// returned error.
func NewSync[T any](
	ctx context.Context,
	cfg *config.Config,
	callback func([]*Item[T]) error,
	opts ...Option[T],
) (*Batch[T], error) {
	// Re-use the asynchronous constructor.
	batch, errChan := New(ctx, cfg, callback, opts...)

	var firstErr error
	for err := range errChan { // errChan closes when ctx.Done() is observed.
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return batch, firstErr
}
