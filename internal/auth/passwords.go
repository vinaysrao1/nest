package auth

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

// HashPassword hashes password using bcrypt with cost 12.
//
// Pre-conditions: password must be non-empty.
// Post-conditions: returns a bcrypt hash string suitable for storage.
// Raises: error if bcrypt fails (e.g. password too long).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword returns true if password matches the stored bcrypt hash.
//
// Pre-conditions: hash must be a valid bcrypt hash string.
// Post-conditions: returns true only when the password is correct.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
