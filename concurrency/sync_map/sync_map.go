// Package sync_map provides a copy-on-write map stored in an atomic.Value.
package sync_map

import (
	"sync/atomic"
)

// SyncMap is a concurrent map safe for concurrent readers without a mutex.
type SyncMap struct {
	// We use atomic.Value to store the map, so we don't need to use mutex
	m atomic.Value
}

// New returns an empty SyncMap.
func New() *SyncMap {
	m := make(map[any]any)
	v := atomic.Value{}
	v.Store(m)

	return &SyncMap{
		m: v,
	}
}

// Get returns the value stored in the map for a key.
func (s *SyncMap) Get(key any) any {
	snapshot, ok := s.m.Load().(map[any]any)
	if !ok {
		return nil
	}

	return snapshot[key]
}

// Set stores a value in the map for a key.
func (s *SyncMap) Set(key, value any) {
	snapshot, ok := s.m.Load().(map[any]any)
	if !ok {
		return
	}

	snapshot[key] = value
	s.m.Store(snapshot)
}
