package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

// wantKAT is the openssl reference signature of the official §7.1 message
// under testdata/sign_key.pem:
//
//	printf '%s' '2010-05-18;2010-05-18T11:22:19;FAC 001/14;3.12;' > msg.txt
//	openssl dgst -sha1 -sign signing/testdata/sign_key.pem msg.txt | openssl enc -base64 -A
const wantKAT = `ubv6TL+ypqJqOYoeX6gnXweIYB2yXARPgmPgZGKhf0FtYhu29uf9pMmXIVrC7RVQz1BiQxAhO9fjYml0oGUrQ5rWou8Zg3Au46fYGh/sRF0tofNvHj7abesL9RtJijdNht/ig8CDFnQemg+0J6kwHmETRwtCaiDwHWrE88iOWuc=`

const officialMsg = "2010-05-18;2010-05-18T11:22:19;FAC 001/14;3.12;"

func loadSigner(t *testing.T, path string) *RSASigner {
	t.Helper()
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewRSASigner(pemBytes, 1)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// §12.1/§12.2: Go signature must match the openssl reference byte-for-byte.
func TestSign_KnownAnswer(t *testing.T) {
	s := loadSigner(t, "testdata/sign_key.pem")
	hash, control, err := s.Sign(officialMsg)
	if err != nil {
		t.Fatal(err)
	}
	if hash != wantKAT {
		t.Errorf("signature mismatch with openssl reference:\n got: %s\nwant: %s", hash, wantKAT)
	}
	if control != "1" {
		t.Errorf("control = %q, want \"1\"", control)
	}
	if len(hash) != 172 { // §12.3
		t.Errorf("len(hash) = %d, want 172", len(hash))
	}
}

// PKCS#8 encoding of the same key must load and sign identically.
func TestNewRSASigner_PKCS8(t *testing.T) {
	s := loadSigner(t, "testdata/sign_key_pkcs8.pem")
	hash, _, err := s.Sign(officialMsg)
	if err != nil {
		t.Fatal(err)
	}
	if hash != wantKAT {
		t.Errorf("PKCS#8-loaded key produced different signature")
	}
}

// §12.2: round-trip with the extracted public key.
func TestSign_VerifyRoundTrip(t *testing.T) {
	s := loadSigner(t, "testdata/sign_key.pem")
	hash, _, err := s.Sign(officialMsg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(&s.key.PublicKey, officialMsg, hash); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := Verify(&s.key.PublicKey, officialMsg+"x", hash); err == nil {
		t.Fatal("verify accepted a tampered message")
	}
}

// §12.4: key size gate — anything but exactly 1024 bits is rejected.
func TestNewRSASigner_Rejects2048(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if _, err := NewRSASigner(pemBytes, 1); err == nil {
		t.Fatal("2048-bit key accepted; want rejection")
	}
}

func TestNewRSASigner_RejectsGarbage(t *testing.T) {
	if _, err := NewRSASigner([]byte("not pem"), 1); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestNewRSASigner_RejectsBadVersion(t *testing.T) {
	pemBytes, err := os.ReadFile("testdata/sign_key.pem")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRSASigner(pemBytes, 0); err == nil {
		t.Fatal("key version 0 accepted; spec §2.1.3 versions start at 1")
	}
}

// A well-formed PEM block of an unsupported type must be rejected (default
// branch of the block.Type switch).
func TestNewRSASigner_RejectsUnsupportedBlockType(t *testing.T) {
	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte{0x01}}
	if _, err := NewRSASigner(pem.EncodeToMemory(block), 1); err == nil {
		t.Fatal("unsupported PEM block type accepted; want rejection")
	}
}

// A valid PKCS#8 key that is not RSA must be rejected by the type assertion.
func TestNewRSASigner_RejectsNonRSAKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if _, err := NewRSASigner(pemBytes, 1); err == nil {
		t.Fatal("non-RSA PKCS#8 key accepted; want rejection")
	}
}

// Key version flows into HashControl (spec §8, SourceBilling=P row).
func TestSign_ControlIsKeyVersion(t *testing.T) {
	pemBytes, err := os.ReadFile("testdata/sign_key.pem")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewRSASigner(pemBytes, 3)
	if err != nil {
		t.Fatal(err)
	}
	_, control, err := s.Sign(officialMsg)
	if err != nil {
		t.Fatal(err)
	}
	if control != "3" {
		t.Errorf("control = %q, want \"3\"", control)
	}
}
