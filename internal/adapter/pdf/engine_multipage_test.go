package pdf

import "testing"

func TestPageCount_NotFoundIsZero(t *testing.T) {
	if got := pageCount([]byte("%PDF-1.7 nothing here")); got != 0 {
		t.Fatalf("pageCount = %d, want 0", got)
	}
}
