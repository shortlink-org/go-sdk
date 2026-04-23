package batch

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/shortlink-org/go-sdk/config"
)

func collectNonNilErrors(errChan <-chan error) []error {
	var errors []error

	for err := range errChan {
		if err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

func TestNewSync(t *testing.T) {
	t.Parallel()

	t.Run("Returns cleanly after context cancellation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())

		aggrCB := func(args []*Item[string]) error {
			for _, item := range args {
				item.CallbackChannel <- item.Item

				close(item.CallbackChannel)
			}

			return nil
		}

		// Call NewSync in a goroutine because it blocks until ctx is done.
		done := make(chan struct{})

		var (
			resultBatch *Batch[string]
			err         error
		)

		go func() {
			cfg, setupErr := config.New()
			if setupErr != nil {
				err = setupErr

				close(done)

				return
			}

			resultBatch, err = NewSync(ctx, cfg, aggrCB)

			close(done)
		}()

		// Give the goroutine a moment to start.
		time.Sleep(5 * time.Millisecond)
		// Trigger shutdown.
		cancel()

		// Wait for NewSync to return.
		<-done

		require.NotNil(t, resultBatch)
		require.NoError(t, err)
	})
}

// TestBatchProcessingWithSynctest verifies batch processing behavior with deterministic timing.
// This test validates both size-based and time-based flush mechanisms using synctest
// to eliminate timing dependencies and ensure consistent test execution.
func TestBatchProcessingWithSynctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, testBatchProcessingWithSynctestInner)
}

func testBatchProcessingWithSynctestInner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callbackCount atomic.Int64

	aggrCB := func(items []*Item[string]) error {
		callbackCount.Add(1)

		for _, item := range items {
			item.CallbackChannel <- item.Item

			close(item.CallbackChannel)
		}

		return nil
	}

	cfg, err := config.New()
	require.NoError(t, err)

	batchInst, errChan := New(ctx, cfg, aggrCB, WithInterval[string](100*time.Millisecond), WithSize[string](3))

	ch1 := batchInst.Push("item1")
	ch2 := batchInst.Push("item2")

	synctest.Wait()
	require.Equal(t, int64(0), callbackCount.Load())

	ch3 := batchInst.Push("item3")

	synctest.Wait()

	require.Equal(t, int64(1), callbackCount.Load())
	require.Equal(t, "item1", <-ch1)
	require.Equal(t, "item2", <-ch2)
	require.Equal(t, "item3", <-ch3)

	ch4 := batchInst.Push("item4")

	time.Sleep(100 * time.Millisecond)
	synctest.Wait()

	require.Equal(t, int64(2), callbackCount.Load())
	require.Equal(t, "item4", <-ch4)

	cancel()

	require.Empty(t, collectNonNilErrors(errChan))
}

// TestBatchCancellationWithSynctest verifies proper resource cleanup and graceful shutdown
// when the batch context is canceled. Ensures that pending items are handled correctly
// and no goroutines are leaked during cancellation scenarios.
func TestBatchCancellationWithSynctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		var processedCount atomic.Int64

		aggrCB := func(items []*Item[string]) error {
			processedCount.Add(int64(len(items)))

			for _, item := range items {
				item.CallbackChannel <- item.Item

				close(item.CallbackChannel)
			}

			return nil
		}

		// Create batch with long interval to test cancellation
		cfg, err := config.New()
		require.NoError(t, err)

		batch, errChan := New(ctx, cfg, aggrCB, WithInterval[string](10*time.Second), WithSize[string](100))

		// Add some items
		ch1 := batch.Push("item1")
		ch2 := batch.Push("item2")

		// Cancel context immediately
		cancel()

		// Wait for batch to process cancellation
		synctest.Wait()

		// Verify channels are closed (items should be dropped on cancellation)
		_, ok1 := <-ch1
		_, ok2 := <-ch2

		require.False(t, ok1, "channel should be closed")
		require.False(t, ok2, "channel should be closed")

		require.Empty(t, collectNonNilErrors(errChan))
	})
}

// TestBatchTimeBasedFlushWithSynctest validates the time-based flush mechanism.
// Verifies that batches are flushed according to the configured interval when
// the size threshold is not reached, ensuring predictable batch processing behavior.
func TestBatchTimeBasedFlushWithSynctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var flushCount atomic.Int64

		aggrCB := func(items []*Item[string]) error {
			flushCount.Add(1)

			for _, item := range items {
				item.CallbackChannel <- item.Item

				close(item.CallbackChannel)
			}

			return nil
		}

		// Create batch with 50ms interval and high size limit
		cfg, err := config.New()
		require.NoError(t, err)

		batch, errChan := New(ctx, cfg, aggrCB, WithInterval[string](50*time.Millisecond), WithSize[string](1000))

		// Add items over multiple intervals
		ch1 := batch.Push("item1")

		// Advance time to the first interval flush (50ms)
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		require.Equal(t, int64(1), flushCount.Load())
		require.Equal(t, "item1", <-ch1)

		// Add more items
		ch2 := batch.Push("item2")
		ch3 := batch.Push("item3")

		// Advance time to the next interval flush
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		require.Equal(t, int64(2), flushCount.Load())
		require.Equal(t, "item2", <-ch2)
		require.Equal(t, "item3", <-ch3)

		// Clean up
		cancel()

		for err := range errChan {
			_ = err
		}
	})
}
