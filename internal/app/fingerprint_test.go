package app_test

import (
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
)

func TestFingerprint(t *testing.T) {
	a := app.Fingerprint([]byte("payload-1"))
	b := app.Fingerprint([]byte("payload-1"))
	c := app.Fingerprint([]byte("payload-2"))

	if a != b {
		t.Errorf("Fingerprint not stable: %q != %q", a, b)
	}
	if a == c {
		t.Errorf("Fingerprint must differ on different payloads, both = %q", a)
	}
	if len(a) != 64 { // hex(sha256) is 64 chars
		t.Errorf("Fingerprint length = %d, want 64", len(a))
	}
}
