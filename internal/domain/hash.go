package domain

import (
	"fmt"
	"regexp"
)

// Hash is the document signature (base64 of RSA-SHA1 of the canonical line) per
// Portaria 363/2010. Domain stores it; the signing implementation lives in infra
// and satisfies the Signer interface. Max length 172 chars per XSD.
type Hash string

const (
	MaxLenHash        = 172
	MaxLenHashControl = 70
)

// hashCharsPattern: standard base64 alphabet with optional padding — the only
// shape a base64-encoded RSA signature can have.
var hashCharsPattern = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)

func (h Hash) Validate() error {
	if h == "" {
		return fmt.Errorf("hash is required")
	}
	if len(h) > MaxLenHash {
		return fmt.Errorf("hash exceeds %d chars: len=%d", MaxLenHash, len(h))
	}
	// FourChars reads 1-based position 31; anything shorter cannot be a real
	// RSA signature and would corrupt QR field Q.
	if len(h) < 32 {
		return fmt.Errorf("hash implausibly short for an RSA signature: len=%d", len(h))
	}
	if !hashCharsPattern.MatchString(string(h)) {
		return fmt.Errorf("hash is not base64: %q", h)
	}
	return nil
}

// HashControl identifies the signing-key version and any manual/recovery prefix.
var hashControlPattern = regexp.MustCompile(
	`^([0-9]+|[0-9]+\.[0-9]+|[0-9]+-[A-Z]{2}(M )([^/^ ]+/[0-9]+)|[0-9]+-[A-Z]{2}(D )([^ ]+ [^/^ ]+/[0-9]+))$`,
)

type HashControl string

// FourChars returns the hash characters at 1-based positions 1, 11, 21, 31 —
// NOT the first four. Used by QR field Q and the fatcorews HashCharacters
// field. Bounds-guarded against short hashes.
func (h Hash) FourChars() string {
	s := string(h)
	var b []byte
	for _, pos := range []int{1, 11, 21, 31} {
		if pos-1 < len(s) {
			b = append(b, s[pos-1])
		}
	}
	return string(b)
}

func (c HashControl) Validate() error {
	if c == "" {
		return fmt.Errorf("hash control is required")
	}
	if len(c) > MaxLenHashControl {
		return fmt.Errorf("hash control exceeds %d chars: %q", MaxLenHashControl, c)
	}
	if !hashControlPattern.MatchString(string(c)) {
		return fmt.Errorf("hash control does not match SAF-T pattern: %q", c)
	}
	return nil
}
