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

func (h Hash) Validate() error {
	if h == "" {
		return fmt.Errorf("hash is required")
	}
	if len(h) > MaxLenHash {
		return fmt.Errorf("hash exceeds %d chars: len=%d", MaxLenHash, len(h))
	}
	return nil
}

// HashControl identifies the signing-key version and any manual/recovery prefix.
var hashControlPattern = regexp.MustCompile(
	`^([0-9]+|[0-9]+\.[0-9]+|[0-9]+-[A-Z]{2}(M )([^/^ ]+/[0-9]+)|[0-9]+-[A-Z]{2}(D )([^ ]+ [^/^ ]+/[0-9]+))$`,
)

type HashControl string

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
