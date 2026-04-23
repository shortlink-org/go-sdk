package worker_pool

// WorkerPool distributes Task values across worker goroutines.
type WorkerPool struct {
	taskQueue chan Task
	Result    chan Result
}

// New creates a WorkerPool with workerNum concurrent workers.
func New(workerNum int) *WorkerPool {
	pool := &WorkerPool{
		taskQueue: make(chan Task, workerNum),
		Result:    make(chan Result, workerNum),
	}

	for range workerNum {
		go NewWorker(pool.taskQueue, pool.Result)
	}

	return pool
}

// Push enqueues one or more tasks for workers to execute.
func (wp *WorkerPool) Push(task ...Task) {
	for _, t := range task {
		wp.taskQueue <- t
	}
}

// Close closes the task queue channel.
func (wp *WorkerPool) Close() {
	close(wp.taskQueue)
}
