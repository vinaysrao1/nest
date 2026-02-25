package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateSessionID creates a cryptographically random 32-byte session ID
// encoded as 64 lowercase hex characters.
//
// Post-conditions: returns a 64-character hex string.
// Panics if crypto/rand is unavailable (system-level failure).
func GenerateSessionID() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(buf)
}
