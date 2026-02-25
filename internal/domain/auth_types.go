package domain

import "time"

// Session is a server-side session for UI authentication.
type Session struct {
	SID       string         `json:"sid"`
	UserID    string         `json:"user_id"`
	Data      map[string]any `json:"data"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// ApiKey is API key metadata. The plaintext key is never stored;
// only the hash is persisted.
type ApiKey struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// SigningKey is an RSA key pair for webhook payload signing.
type SigningKey struct {
	ID         string    `json:"id"`
	OrgID      string    `json:"org_id"`
	PublicKey  string    `json:"public_key"`
	PrivateKey string    `json:"-"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

// PasswordResetToken is a one-time password reset token.
type PasswordResetToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
