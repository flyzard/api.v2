package at

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"time"
)

// atTimestampFormat is the ISO 8601 format AT expects for the Created field.
const atTimestampFormat = "2006-01-02T15:04:05.000Z"

// pkcs7Pad adds PKCS7 padding to data for the given block size.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padBytes := make([]byte, padding)
	for i := range padBytes {
		padBytes[i] = byte(padding)
	}
	return append(data, padBytes...)
}

// aesECBEncrypt encrypts plaintext using AES-128-ECB with PKCS7 padding.
// Go stdlib doesn't provide ECB mode, so we process each 16-byte block manually.
func aesECBEncrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))

	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}

	return ciphertext, nil
}

// EncryptATCredentials produces the three encrypted WS-Security header fields
// required by AT's webservices. Each call generates a fresh random AES key for
// replay protection.
//
// Returns (encryptedPassword, nonce, encryptedCreated, error) where:
//   - encryptedPassword = Base64(AES-128-ECB(password_bytes, Ks))
//   - nonce = Base64(RSA-PKCS1v15(Ks, atPublicKey))
//   - encryptedCreated = Base64(AES-128-ECB(timestamp_bytes, Ks))
func EncryptATCredentials(password string, timestamp time.Time, atPublicKey *rsa.PublicKey) (string, string, string, error) {
	// Generate fresh random 16-byte AES key (Ks)
	ks := make([]byte, 16)
	if _, err := rand.Read(ks); err != nil {
		return "", "", "", fmt.Errorf("generating random AES key: %w", err)
	}

	// Encrypt password with AES-128-ECB
	encPassword, err := aesECBEncrypt([]byte(password), ks)
	if err != nil {
		return "", "", "", fmt.Errorf("encrypting password: %w", err)
	}

	// Encrypt Ks with RSA-PKCS1v15 (the nonce)
	//lint:ignore SA1019 PKCS#1 v1.5 is AT's mandated wire format for the
	// WS-Security nonce (like the ECB requirement above) — OAEP would break
	// the SOAP contract.
	encKs, err := rsa.EncryptPKCS1v15(rand.Reader, atPublicKey, ks)
	if err != nil {
		return "", "", "", fmt.Errorf("encrypting AES key with RSA: %w", err)
	}

	// Encrypt timestamp with AES-128-ECB
	tsBytes := []byte(timestamp.UTC().Format(atTimestampFormat))
	encCreated, err := aesECBEncrypt(tsBytes, ks)
	if err != nil {
		return "", "", "", fmt.Errorf("encrypting timestamp: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encPassword),
		base64.StdEncoding.EncodeToString(encKs),
		base64.StdEncoding.EncodeToString(encCreated),
		nil
}

// ParseATPublicKey parses AT's RSA public key from PEM data.
// Handles both raw PKIX public keys and X.509 certificates.
func ParseATPublicKey(pemData string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	switch block.Type {
	case "PUBLIC KEY":
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing PKIX public key: %w", err)
		}
		rsaKey, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("key is not an RSA public key")
		}
		return rsaKey, nil

	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing X.509 certificate: %w", err)
		}
		rsaKey, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("certificate does not contain an RSA public key")
		}
		return rsaKey, nil

	default:
		return nil, fmt.Errorf("unsupported PEM block type: %s", block.Type)
	}
}
