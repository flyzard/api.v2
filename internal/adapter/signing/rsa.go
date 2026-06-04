// Package signing implements the AT document signature
package signing

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // SHA-1 mandated by Despacho 8632/2014 §4.1; not negotiable.
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strconv"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// RSASigner signs canonical document lines with the producer's AT-registered private key.
type RSASigner struct {
	key     *rsa.PrivateKey
	control string
}

var _ domain.Signer = (*RSASigner)(nil)

// NewRSASigner parses a PEM private key and gates it to exactly 1024 bits.
func NewRSASigner(pemBytes []byte, keyVersion int) (*RSASigner, error) {
	if keyVersion < 1 {
		return nil, fmt.Errorf("key version must be >= 1, got %d", keyVersion)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in key material")
	}
	var (
		parsed any
		err    error
	)
	switch block.Type {
	case "RSA PRIVATE KEY": // PKCS#1
		parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY": // PKCS#8
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key: %T", parsed)
	}
	if bits := key.N.BitLen(); bits != 1024 {
		return nil, fmt.Errorf("AT signing key must be exactly 1024 bits, got %d (Despacho 8632/2014 §4.2)", bits)
	}
	return &RSASigner{key: key, control: strconv.Itoa(keyVersion)}, nil
}

// Sign implements domain.Signer.
func (s *RSASigner) Sign(canonical string) (hash, control string, err error) {
	digest := sha1.Sum([]byte(canonical))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA1, digest[:])
	if err != nil {
		return "", "", fmt.Errorf("rsa sign: %w", err)
	}
	h := base64.StdEncoding.EncodeToString(sig)
	if len(h) != domain.MaxLenHash {
		return "", "", fmt.Errorf("signature is %d base64 chars, want %d", len(h), domain.MaxLenHash)
	}
	return h, s.control, nil
}

// Verify checks a stored 172-char hash against its reconstructed canonical line
func Verify(pub *rsa.PublicKey, canonical, hash string) error {
	sig, err := base64.StdEncoding.DecodeString(hash)
	if err != nil {
		return fmt.Errorf("decode hash: %w", err)
	}
	digest := sha1.Sum([]byte(canonical))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA1, digest[:], sig); err != nil {
		return fmt.Errorf("signature does not verify: %w", err)
	}
	return nil
}
