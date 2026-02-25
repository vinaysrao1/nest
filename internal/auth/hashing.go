package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// HashAPIKey returns the SHA-256 hex digest of key.
// Deterministic: same input always produces same output.
//
// Pre-conditions: key must be non-empty.
// Post-conditions: returns a 64-character lowercase hex string.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// GenerateAPIKey creates a new API key and returns the plaintext key,
// its 8-character prefix (used for display), and the SHA-256 hash
// (the value persisted to the database).
//
// Post-conditions: hash == HashAPIKey(key), prefix == key[:8].
// Panics if crypto/rand is unavailable (system-level failure).
func GenerateAPIKey() (key string, prefix string, hash string) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	key = hex.EncodeToString(buf)
	prefix = key[:8]
	hash = HashAPIKey(key)
	return
}

// GenerateToken creates a random 32-byte token and returns the plaintext
// hex string and its SHA-256 hash.
//
// Post-conditions: hash == HashAPIKey(plaintext).
// Raises: error if crypto/rand fails.
func GenerateToken() (plaintext string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	plaintext = hex.EncodeToString(buf)
	hash = HashAPIKey(plaintext)
	return
}
