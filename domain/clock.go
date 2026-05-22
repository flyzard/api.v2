package domain

import "time"

// Clock is the seam through which deadline-bearing operations (e.g.
// IssuedDocument.Cancel) read the current time. Issuance keeps its
// explicit now time.Time argument — only flows that need to enforce
// a wall-clock deadline depend on this.
type Clock interface {
	Now() time.Time
}

// SystemClock returns time.Now(). Use in production code; tests inject a
// fake Clock to make deadline tests deterministic.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
