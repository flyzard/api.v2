package at

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestPKCS7Pad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		inputLen  int
		blockSize int
		wantLen   int
	}{
		{"empty", 0, 16, 16},
		{"1 byte", 1, 16, 16},
		{"15 bytes", 15, 16, 16},
		{"16 bytes (full block)", 16, 16, 32},
		{"17 bytes", 17, 16, 32},
		{"32 bytes (full block)", 32, 16, 48},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make([]byte, tt.inputLen)
			result := pkcs7Pad(input, tt.blockSize)
			if len(result)%tt.blockSize != 0 {
				t.Errorf("result length %d is not a multiple of block size %d", len(result), tt.blockSize)
			}
			if len(result) != tt.wantLen {
				t.Errorf("got length %d, want %d", len(result), tt.wantLen)
			}
			// Verify padding value
			padVal := result[len(result)-1]
			for i := len(result) - int(padVal); i < len(result); i++ {
				if result[i] != padVal {
					t.Errorf("padding byte at index %d is %d, want %d", i, result[i], padVal)
				}
			}
		})
	}
}

func TestAESECBEncrypt(t *testing.T) {
	t.Parallel()

	key := make([]byte, 16)
	for i := range key {
		key[i] = byte(i)
	}

	t.Run("known input produces consistent output", func(t *testing.T) {
		t.Parallel()
		plaintext := []byte("hello world 1234") // exactly 16 bytes
		enc1, err := aesECBEncrypt(plaintext, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		enc2, err := aesECBEncrypt(plaintext, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// ECB: same input+key = same output
		if string(enc1) != string(enc2) {
			t.Error("same input+key should produce same ciphertext in ECB mode")
		}
	})

	t.Run("different keys produce different output", func(t *testing.T) {
		t.Parallel()
		plaintext := []byte("test data")
		key2 := make([]byte, 16)
		for i := range key2 {
			key2[i] = byte(i + 100)
		}
		enc1, err := aesECBEncrypt(plaintext, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		enc2, err := aesECBEncrypt(plaintext, key2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(enc1) == string(enc2) {
			t.Error("different keys should produce different ciphertext")
		}
	})

	t.Run("output is padded to block boundary", func(t *testing.T) {
		t.Parallel()
		plaintext := []byte("short")
		enc, err := aesECBEncrypt(plaintext, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(enc)%aes.BlockSize != 0 {
			t.Errorf("ciphertext length %d is not a multiple of block size", len(enc))
		}
	})

	t.Run("invalid key size", func(t *testing.T) {
		t.Parallel()
		_, err := aesECBEncrypt([]byte("test"), []byte("short"))
		if err == nil {
			t.Error("expected error for invalid key size")
		}
	})
}

func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	return key
}

// aesECBDecrypt decrypts AES-128-ECB ciphertext and removes PKCS7 padding (test helper).
func aesECBDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	// Remove PKCS7 padding
	padLen := int(plaintext[len(plaintext)-1])
	return plaintext[:len(plaintext)-padLen], nil
}

func TestEncryptATCredentials(t *testing.T) {
	t.Parallel()
	privKey := generateTestRSAKey(t)
	pubKey := &privKey.PublicKey

	password := "mySecretPass123"
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	encPass, nonce, encCreated, err := EncryptATCredentials(password, ts, pubKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Decode nonce and decrypt with private key to recover Ks
	nonceBytes, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		t.Fatalf("decoding nonce: %v", err)
	}
	//lint:ignore SA1019 AT spec requires PKCS#1 v1.5 — test mirrors the wire format.
	ks, err := rsa.DecryptPKCS1v15(rand.Reader, privKey, nonceBytes)
	if err != nil {
		t.Fatalf("decrypting nonce: %v", err)
	}
	if len(ks) != 16 {
		t.Fatalf("recovered AES key is %d bytes, want 16", len(ks))
	}

	// Decrypt password
	encPassBytes, err := base64.StdEncoding.DecodeString(encPass)
	if err != nil {
		t.Fatalf("decoding password: %v", err)
	}
	decPass, err := aesECBDecrypt(encPassBytes, ks)
	if err != nil {
		t.Fatalf("decrypting password: %v", err)
	}
	if string(decPass) != password {
		t.Errorf("decrypted password = %q, want %q", string(decPass), password)
	}

	// Decrypt created timestamp
	encCreatedBytes, err := base64.StdEncoding.DecodeString(encCreated)
	if err != nil {
		t.Fatalf("decoding created: %v", err)
	}
	decCreated, err := aesECBDecrypt(encCreatedBytes, ks)
	if err != nil {
		t.Fatalf("decrypting created: %v", err)
	}
	expectedTS := ts.UTC().Format(atTimestampFormat)
	if string(decCreated) != expectedTS {
		t.Errorf("decrypted created = %q, want %q", string(decCreated), expectedTS)
	}
}

func TestEncryptATCredentials_FreshKeyPerCall(t *testing.T) {
	t.Parallel()
	privKey := generateTestRSAKey(t)
	pubKey := &privKey.PublicKey

	ts := time.Now()
	_, nonce1, _, err := EncryptATCredentials("pass", ts, pubKey)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, nonce2, _, err := EncryptATCredentials("pass", ts, pubKey)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if nonce1 == nonce2 {
		t.Error("two calls with same inputs should produce different nonces (fresh AES key each time)")
	}
}

func TestParseATPublicKey_PKIX(t *testing.T) {
	t.Parallel()
	privKey := generateTestRSAKey(t)

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyBytes})

	key, err := ParseATPublicKey(string(pemBlock))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.N.Cmp(privKey.N) != 0 {
		t.Error("parsed key does not match original")
	}
}

func TestParseATPublicKey_Certificate(t *testing.T) {
	t.Parallel()
	privKey := generateTestRSAKey(t)

	// Create a self-signed certificate
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "AT Test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	key, err := ParseATPublicKey(string(pemBlock))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.N.Cmp(privKey.N) != 0 {
		t.Error("parsed key does not match original")
	}
}

func TestParseATPublicKey_Errors(t *testing.T) {
	t.Parallel()

	t.Run("invalid PEM", func(t *testing.T) {
		t.Parallel()
		_, err := ParseATPublicKey("not a pem block")
		if err == nil {
			t.Error("expected error for invalid PEM")
		}
	})

	t.Run("unsupported block type", func(t *testing.T) {
		t.Parallel()
		pemBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("fake")})
		_, err := ParseATPublicKey(string(pemBlock))
		if err == nil {
			t.Error("expected error for unsupported block type")
		}
	})

	t.Run("invalid PKIX data", func(t *testing.T) {
		t.Parallel()
		pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("not a key")})
		_, err := ParseATPublicKey(string(pemBlock))
		if err == nil {
			t.Error("expected error for invalid PKIX data")
		}
	})
}
