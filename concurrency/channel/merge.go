// Package channel provides helpers for composing channels.
package channel

import (
	"sync"
)

// Merge merges multiple channels into one.
func Merge[T any](items ...<-chan T) <-chan T {
	out := make(chan T)

	var waitGroup sync.WaitGroup

	for _, item := range items {
		waitGroup.Go(func() {
			for n := range item {
				out <- n
			}
		})
	}

	go func() {
		waitGroup.Wait()
		close(out)
	}()

	return out
}
