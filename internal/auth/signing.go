package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"github.com/vinaysrao1/nest/internal/store"
)

// Signer signs webhook payloads using the org's active RSA-PSS signing key.
type Signer struct {
	store *store.Queries
}

// NewSigner creates a Signer backed by the given store.
//
// Pre-conditions: st must be non-nil.
func NewSigner(st *store.Queries) *Signer {
	return &Signer{store: st}
}

// Sign retrieves the active RSA signing key for orgID, signs payload with
// RSA-PSS (SHA-256, salt length == hash length), and returns the
// base64-encoded signature.
//
// Pre-conditions: orgID must be non-empty; payload may be empty.
// Post-conditions: returns a valid base64 RSA-PSS signature string.
// Raises: error if the signing key cannot be retrieved, decoded, or parsed;
//
//	error if signing fails.
func (s *Signer) Sign(ctx context.Context, orgID string, payload []byte) (string, error) {
	sk, err := s.store.GetActiveSigningKey(ctx, orgID)
	if err != nil {
		return "", fmt.Errorf("get active signing key: %w", err)
	}

	privateKey, err := parseRSAPrivateKey(sk.PrivateKey)
	if err != nil {
		return "", err
	}

	digest := sha256.Sum256(payload)
	sig, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, digest[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		return "", fmt.Errorf("sign payload: %w", err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

// GenerateRSAKeyPair generates a 2048-bit RSA key pair and returns the
// PEM-encoded public and private keys.
//
// Post-conditions: publicPEM is PKCS#1 "RSA PUBLIC KEY", privatePEM is PKCS#1 "RSA PRIVATE KEY".
// Raises: error if key generation fails.
func GenerateRSAKeyPair() (publicPEM string, privatePEM string, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate RSA key: %w", err)
	}

	privBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	pubBlock := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	}

	return string(pem.EncodeToMemory(pubBlock)), string(pem.EncodeToMemory(privBlock)), nil
}

// parseRSAPrivateKey decodes a PEM block and parses either a PKCS#1 or PKCS#8
// RSA private key.
//
// Pre-conditions: pemData must be a valid PEM-encoded RSA private key.
// Post-conditions: returns a non-nil *rsa.PrivateKey.
// Raises: error if PEM decoding or key parsing fails.
func parseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Attempt PKCS#1 first (RSA PRIVATE KEY header).
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Fall back to PKCS#8 (PRIVATE KEY header).
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return rsaKey, nil
}
