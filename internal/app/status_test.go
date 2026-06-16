package app_test

import (
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
)

func TestStatus(t *testing.T) {
	cases := []struct {
		kind app.Kind
		want int
	}{
		{app.KindInternal, 500},
		{app.KindInvalid, 422},
		{app.KindNotFound, 404},
		{app.KindConflict, 409},
		{app.KindAT, 502},
	}
	for _, c := range cases {
		if got := app.Status(c.kind); got != c.want {
			t.Errorf("Status(%v) = %d, want %d", c.kind, got, c.want)
		}
	}
	if got := app.Status(app.Kind(99)); got != 500 {
		t.Errorf("Status(unknown) = %d, want 500", got)
	}
}
