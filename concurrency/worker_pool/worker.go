// Package worker_pool runs tasks on a fixed-size pool of workers.
package worker_pool

// Task is a unit of work executed by a worker.
type Task func() (any, error)

// Result carries the outcome of executing a Task.
type Result struct {
	Value any
	Error error
}

// Worker reads tasks from taskQueue and publishes Result values to result.
type Worker struct {
	taskQueue <-chan Task
	result    chan<- Result
}

// NewWorker starts a goroutine that executes tasks until taskQueue is closed.
func NewWorker(taskQueue <-chan Task, result chan<- Result) *Worker {
	worker := &Worker{
		taskQueue: taskQueue,
		result:    result,
	}

	go worker.run()

	return worker
}

func (w *Worker) run() {
	for task := range w.taskQueue {
		result, err := task()
		w.result <- Result{result, err}
	}
}
