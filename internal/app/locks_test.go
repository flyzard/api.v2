package app

import (
	"sync"
	"testing"
	"time"
)

func TestSeriesLocks_SameKeyExcludes(t *testing.T) {
	l := newSeriesLocks()
	unlock := l.lock("t", "s")

	reached := make(chan struct{})
	go func() {
		u2 := l.lock("t", "s")
		close(reached)
		u2()
	}()

	select {
	case <-reached:
		t.Fatal("second lock acquired while first was held")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("second lock never acquired after release")
	}
}

func TestSeriesLocks_DifferentKeysConcurrent(t *testing.T) {
	l := newSeriesLocks()
	u1 := l.lock("t", "s1")
	defer u1()

	done := make(chan struct{})
	go func() {
		u2 := l.lock("t", "s2")
		u2()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("different-key lock should not block")
	}
}

// ---------------------------------------------------------------------------
// sourceLocks / lockMany tests
// ---------------------------------------------------------------------------

// TestSourceLocks_OverlappingSetExcludes: holding lockMany({"B","A"}) blocks a
// second lockMany({"A"}) until the first unlock runs.
func TestSourceLocks_OverlappingSetExcludes(t *testing.T) {
	sl := newSourceLocks()
	unlock := sl.lockMany("tenant1", []string{"B", "A"})

	reached := make(chan struct{})
	go func() {
		u2 := sl.lockMany("tenant1", []string{"A"})
		close(reached)
		u2()
	}()

	select {
	case <-reached:
		t.Fatal("second lockMany acquired while first was held")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("second lockMany never acquired after release")
	}
}

// TestSourceLocks_DisjointSetsConcurrent: two lockMany calls with no key
// overlap do not block each other.
func TestSourceLocks_DisjointSetsConcurrent(t *testing.T) {
	sl := newSourceLocks()
	u1 := sl.lockMany("tenant1", []string{"A"})
	defer u1()

	done := make(chan struct{})
	go func() {
		u2 := sl.lockMany("tenant1", []string{"B"})
		u2()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disjoint-set lockMany should not block")
	}
}

// TestSourceLocks_DeadlockFreeOppositeOrder: two goroutines acquire
// overlapping multi-source sets in opposite caller order. Because lockMany
// sorts before acquiring there is a global ordering and no deadlock. The test
// asserts both complete within a generous timeout.
func TestSourceLocks_DeadlockFreeOppositeOrder(t *testing.T) {
	sl := newSourceLocks()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			u := sl.lockMany("tenant1", []string{"A", "B"}) // caller order A, B
			u()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			u := sl.lockMany("tenant1", []string{"B", "A"}) // caller order B, A
			u()
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected: goroutines did not complete within timeout")
	}
}

// TestSourceLocks_TenantIsolation: the same source key under different tenant
// IDs does not cause mutual exclusion.
func TestSourceLocks_TenantIsolation(t *testing.T) {
	sl := newSourceLocks()
	u1 := sl.lockMany("tenantA", []string{"doc1"})
	defer u1()

	done := make(chan struct{})
	go func() {
		u2 := sl.lockMany("tenantB", []string{"doc1"})
		u2()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("different-tenant same-key lockMany should not block")
	}
}

// TestSourceLocks_EmptyAndDedup: nil input and a list with duplicate entries
// both complete without panic; dedup means only a single logical mutex is
// acquired for {"A","A"}.
func TestSourceLocks_EmptyAndDedup(t *testing.T) {
	sl := newSourceLocks()

	// nil → no-op unlock func, must not panic.
	unlock := sl.lockMany("t", nil)
	unlock()

	// Duplicate entries → deduplicated to one mutex; second lockMany on "A"
	// should block while the first is held.
	u1 := sl.lockMany("t", []string{"A", "A"})

	reached := make(chan struct{})
	go func() {
		u2 := sl.lockMany("t", []string{"A"})
		close(reached)
		u2()
	}()

	select {
	case <-reached:
		t.Fatal("deduped lockMany should still exclude same key")
	case <-time.After(50 * time.Millisecond):
	}

	u1()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("second lockMany never acquired after release of deduped lock")
	}
}
