package app

import "sync"

// seriesLocks provides one mutex per (tenant, series) so issuance against the
// same series serializes in-process — the fast path that keeps the hash chain
// intact without relying on retries. Keys are never reclaimed; the set is
// bounded by the number of series, which is small.
type seriesLocks struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func newSeriesLocks() *seriesLocks {
	return &seriesLocks{m: make(map[string]*sync.Mutex)}
}

// lock acquires the mutex for (tenantID, seriesID) and returns its unlock func.
func (l *seriesLocks) lock(tenantID, seriesID string) func() {
	key := tenantID + "\x00" + seriesID
	l.mu.Lock()
	m, ok := l.m[key]
	if !ok {
		m = &sync.Mutex{}
		l.m[key] = m
	}
	l.mu.Unlock()

	m.Lock()
	return m.Unlock
}
