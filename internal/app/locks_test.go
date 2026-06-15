package app

import (
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
