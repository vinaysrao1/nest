package domain

import "time"

// UserRole represents the RBAC role of a user within an organization.
type UserRole string

const (
	UserRoleAdmin     UserRole = "ADMIN"
	UserRoleModerator UserRole = "MODERATOR"
	UserRoleAnalyst   UserRole = "ANALYST"
)

// User is a user within an organization.
type User struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Password  string    `json:"-"`
	Role      UserRole  `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
