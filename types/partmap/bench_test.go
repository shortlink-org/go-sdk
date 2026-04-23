package partmap

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkStd(b *testing.B) {
	b.Run("set std concurrently", func(b *testing.B) {
		m := make(map[string]int)

		var (
			wg      sync.WaitGroup
			mu      sync.RWMutex
			counter int64
		)

		b.ResetTimer()
		b.ReportAllocs()

		for b.Loop() {
			wg.Go(func() {
				i := atomic.AddInt64(&counter, 1)
				key := strconv.FormatInt(i, 10)

				mu.Lock()
				m[key] = int(i)
				mu.Unlock()
			})
		}

		wg.Wait()
	})
}

func BenchmarkSyncStd(b *testing.B) {
	b.Run("set sync map std concurrently", func(b *testing.B) {
		var (
			m       sync.Map
			wg      sync.WaitGroup
			counter int64
		)

		b.ResetTimer()
		b.ReportAllocs()

		for b.Loop() {
			wg.Go(func() {
				i := atomic.AddInt64(&counter, 1)
				key := strconv.FormatInt(i, 10)
				m.Store(key, int(i))
			})
		}

		wg.Wait()
	})
}

func BenchmarkPartitioned(b *testing.B) {
	m, err := New(&HashSumPartitioner{1000}, 1000)
	if err != nil {
		b.Fatalf("Failed to create PartMap: %v", err)
	}

	b.Run("set partitioned concurrently", func(b *testing.B) {
		var (
			wg      sync.WaitGroup
			counter int64
		)

		b.ResetTimer()
		b.ReportAllocs()

		for b.Loop() {
			wg.Go(func() {
				i := atomic.AddInt64(&counter, 1)

				key := strconv.FormatInt(i, 10)
				err := m.Set(key, int(i))
				if err != nil {
					b.Errorf("Failed to set value in PartMap: %v", err)
				}
			})
		}

		wg.Wait()
	})
}
