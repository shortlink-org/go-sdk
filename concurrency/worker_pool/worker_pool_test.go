package worker_pool_test

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/concurrency/worker_pool"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func Test_WorkerPool(t *testing.T) {
	t.Parallel()

	workerPool := worker_pool.New(10)

	task := func() (any, error) {
		// some operation
		return 0, nil
	}

	waitGroup := sync.WaitGroup{}
	done := make(chan struct{})

	go func() {
		for range 1000 {
			workerPool.Push(task)
			waitGroup.Go(func() {
				<-workerPool.Result
			})
		}

		close(done)
	}()

	<-done
	waitGroup.Wait()
	close(workerPool.Result)

	t.Cleanup(func() {
		workerPool.Close()
	})
}

// TestWorkerPoolWithSynctest validates worker pool task execution and result collection.
// Tests that tasks are properly distributed across workers, executed concurrently,
// and results are collected correctly without timing dependencies.
func TestWorkerPoolWithSynctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			numWorkers = 3
			numTasks   = 10
		)

		workerPool := worker_pool.New(numWorkers)

		var (
			completedTasks atomic.Int64
			results        []any
			resultWg       sync.WaitGroup
		)

		resultWg.Add(1)

		// Define task function that simulates processing work
		taskFunc := func() (any, error) {
			// Simulate processing time - executes instantly in synctest
			time.Sleep(10 * time.Millisecond)

			return completedTasks.Add(1), nil
		}

		// Start background result collector goroutine
		go func() {
			defer resultWg.Done()

			for result := range workerPool.Result {
				results = append(results, result.Value)
				if len(results) == numTasks {
					return
				}
			}
		}()

		// Submit all tasks to the worker pool for concurrent execution
		for range numTasks {
			workerPool.Push(taskFunc)
		}

		// Wait for all tasks to be processed and results collected
		resultWg.Wait()

		// Properly shutdown worker pool and cleanup resources
		workerPool.Close()
		close(workerPool.Result)

		// Ensure all worker goroutines have terminated
		synctest.Wait()

		require.Equal(t, int64(numTasks), completedTasks.Load())
		require.Len(t, results, numTasks)
	})
}

// TestWorkerPoolSimpleWithSynctest validates basic worker functionality in isolation.
// Tests single worker task execution with controlled timing to ensure tasks are
// processed correctly and results are returned as expected.
func TestWorkerPoolSimpleWithSynctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Test isolated worker functionality without complex pool management
		var taskExecuted atomic.Int64

		taskFunc := func() (any, error) {
			// Simulate task processing time
			time.Sleep(50 * time.Millisecond)
			taskExecuted.Add(1)

			return "completed", nil
		}

		// Create minimal worker setup with buffered channels
		taskQueue := make(chan worker_pool.Task, 1)
		result := make(chan worker_pool.Result, 1)

		// Launch single worker to process tasks
		go func() {
			for task := range taskQueue {
				res, err := task()
				result <- worker_pool.Result{Value: res, Error: err}
			}
		}()

		// Submit task and signal completion
		taskQueue <- taskFunc

		close(taskQueue)

		// Retrieve task result
		res := <-result
		close(result)

		// Ensure all concurrent operations have completed
		synctest.Wait()

		require.Equal(t, int64(1), taskExecuted.Load())
		require.Equal(t, "completed", res.Value)
		require.NoError(t, res.Error)
	})
}
