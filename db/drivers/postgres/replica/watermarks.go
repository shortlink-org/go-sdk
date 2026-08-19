package replica

import (
	"context"
	"sync"
	"time"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// Watermarks stores the WAL position each actor must not read behind.
//
// The key is opaque to the store: the HTTP middleware derives it from the
// authenticated subject, and the queue path does not use a store at all,
// because a message carries its own token.
//
// Implementing this is optional. The default path carries the token on the
// wire — a response header, a message metadata entry — which needs no shared
// state and stays correct across pods and across a rolling deploy. Reach for a
// store when you want the guarantee to survive a client that will not echo
// anything back: Redis with a compare-and-set, or a column on the actor's own
// row updated in the same transaction as the write, which is the only variant
// that also survives an application restart.
type Watermarks interface {
	// Get returns the position stored for key, and whether there was one.
	Get(ctx context.Context, key string) (wal.LSN, bool, error)
	// Set records a position for key. An implementation must keep the highest
	// position it has seen — see MemoryWatermarks.Set for why.
	Set(ctx context.Context, key string, position wal.LSN) error
}

// Default bounds for MemoryWatermarks.
const (
	// The lifetime is a memory knob, not a correctness one. Expiring early
	// degrades to a possible stale read; expiring late costs nothing, because
	// the gate opens the moment replay passes the position. It is set at
	// roughly a hundred times a typical replication lag.
	defaultWatermarkTTL = 30 * time.Second

	// The entry cap bounds the map, and it is not optional: the keys come from
	// request data, so an unbounded map is a memory leak an outsider can drive.
	defaultWatermarkMaxEntries = 100_000
)

// MemoryWatermarks keeps watermarks in this process only.
//
// It is correct exactly when every request from one actor reaches the same
// pod. It is therefore NOT wired in by default, and you should think twice
// before wiring it in yourself: with N pods and no session affinity, a request
// following a write lands on the writing pod one time in N. At N=10 it fixes a
// tenth of the cases it appears to fix, non-deterministically — and in a
// single-pod staging environment it looks like it works perfectly.
//
// Prefer carrying the token on the wire. Use this for a single-instance
// deployment, or in development.
type MemoryWatermarks struct {
	ttl        time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[string]watermarkEntry
}

type watermarkEntry struct {
	position wal.LSN
	expires  time.Time
}

// NewMemoryWatermarks returns an in-process store. Zero values select the
// defaults: a 30 second lifetime and 100 000 entries.
func NewMemoryWatermarks(ttl time.Duration, maxEntries int) *MemoryWatermarks {
	if ttl <= 0 {
		ttl = defaultWatermarkTTL
	}

	if maxEntries <= 0 {
		maxEntries = defaultWatermarkMaxEntries
	}

	return &MemoryWatermarks{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[string]watermarkEntry),
	}
}

var _ Watermarks = (*MemoryWatermarks)(nil)

// Get implements Watermarks.
func (m *MemoryWatermarks) Get(_ context.Context, key string) (wal.LSN, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if !ok || time.Now().After(entry.expires) {
		return 0, false, nil
	}

	return entry.position, true, nil
}

// Set implements Watermarks, keeping the highest position seen for the key.
//
// Last-writer-wins would be a correctness hole rather than a nit: two
// concurrent writes by one actor can finish out of order, and a slow Set from
// the earlier one overwriting the later one produces exactly the stale read
// this whole mechanism exists to prevent.
func (m *MemoryWatermarks) Set(_ context.Context, key string, position wal.LSN) error {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.entries[key]; ok && now.Before(existing.expires) && existing.position >= position {
		return nil
	}

	// Drop everything rather than track recency. The map is a cache of
	// short-lived entries, so a wholesale clear costs one round of extra
	// primary reads and needs no per-lookup bookkeeping.
	if len(m.entries) >= m.maxEntries {
		m.entries = make(map[string]watermarkEntry, m.maxEntries)
	}

	m.entries[key] = watermarkEntry{position: position, expires: now.Add(m.ttl)}

	return nil
}

// Len reports how many entries are currently held, expired ones included. It
// exists so that a caller can export the count and notice growth.
func (m *MemoryWatermarks) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.entries)
}
