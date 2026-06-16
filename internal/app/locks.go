package app

import (
	"sort"
	"sync"
)

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

// sourceLocks provides one mutex per (tenant, sourceDocNumber) so concurrent
// issuances that allocate against the same source document are serialized
// in-process. This closes the check-then-act race window: allocation validation
// + issue + save all run while the source lock is held, preventing two receipts
// (or credit notes) in different series from simultaneously passing the ceiling
// check and both committing, which would together exceed the source gross.
//
// The SQL durable backstop (SELECT ... FOR UPDATE on the source row, or an
// optimistic version on a per-source consumed-balance row) is deferred to the
// real persistence adapter; the in-process lock is sufficient for memstore.
//
// Keys are never reclaimed; the set is bounded by the number of source
// documents touched across the process lifetime, which is small in practice.
type sourceLocks struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func newSourceLocks() *sourceLocks {
	return &sourceLocks{m: make(map[string]*sync.Mutex)}
}

// lockMany acquires the mutex for each (tenantID, sourceDocNumber) key in
// sorted order (to avoid deadlock when an issuance references multiple sources)
// and returns a single unlock func that releases all of them in reverse order.
func (l *sourceLocks) lockMany(tenantID string, sourceNums []string) func() {
	if len(sourceNums) == 0 {
		return func() {}
	}

	// Deduplicate and sort for deterministic acquire order (deadlock prevention).
	seen := make(map[string]struct{}, len(sourceNums))
	unique := sourceNums[:0:0]
	for _, n := range sourceNums {
		if _, ok := seen[n]; !ok {
			seen[n] = struct{}{}
			unique = append(unique, n)
		}
	}
	sort.Strings(unique)

	// Fetch (or create) each mutex under the guard lock, then acquire them in
	// order outside the guard lock to avoid holding the guard while blocking.
	mutexes := make([]*sync.Mutex, len(unique))
	l.mu.Lock()
	for i, n := range unique {
		key := tenantID + "\x00" + n
		m, ok := l.m[key]
		if !ok {
			m = &sync.Mutex{}
			l.m[key] = m
		}
		mutexes[i] = m
	}
	l.mu.Unlock()

	for _, m := range mutexes {
		m.Lock()
	}
	return func() {
		for i := len(mutexes) - 1; i >= 0; i-- {
			mutexes[i].Unlock()
		}
	}
}
