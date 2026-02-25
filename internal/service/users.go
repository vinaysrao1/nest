package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// UserUpdateParams holds the optional fields that may be changed on a user update.
// A nil pointer means "do not change this field".
type UserUpdateParams struct {
	Name     *string
	Role     *domain.UserRole
	IsActive *bool
}

// UserService manages the lifecycle of users: invitation, update, deactivation,
// listing, and password reset flows.
type UserService struct {
	store  *store.Queries
	logger *slog.Logger
}

// NewUserService constructs a UserService with the required dependencies.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned UserService is ready for use.
func NewUserService(st *store.Queries, logger *slog.Logger) *UserService {
	return &UserService{store: st, logger: logger}
}

// InviteUser creates a new user in the org with a random temporary password.
// The plaintext temporary password is not returned; the user must use the
// password reset flow to set their own password.
//
// Pre-conditions: orgID, email, name non-empty; role is a valid UserRole.
// Post-conditions: user is persisted with IsActive=true and a bcrypt-hashed temp password.
// Raises: *domain.ValidationError for invalid params; *domain.ConflictError if email already exists.
func (s *UserService) InviteUser(ctx context.Context, orgID, email, name string, role domain.UserRole) (*domain.User, error) {
	if err := validateInviteUser(email, name, role); err != nil {
		return nil, err
	}

	tempPassword := auth.GenerateSessionID()[:16]
	passwordHash, err := auth.HashPassword(tempPassword)
	if err != nil {
		return nil, fmt.Errorf("users.InviteUser: hash password: %w", err)
	}

	now := time.Now().UTC()
	user := domain.User{
		ID:        fmt.Sprintf("usr_%d", now.UnixNano()),
		OrgID:     orgID,
		Email:     email,
		Name:      name,
		Password:  passwordHash,
		Role:      role,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.store.CreateUser(ctx, &user); err != nil {
		return nil, err
	}

	s.logger.Info("user invited", "org_id", orgID, "user_id", user.ID, "email", email, "role", role)
	return &user, nil
}

// UpdateUser applies non-nil fields from params to the existing user.
//
// Pre-conditions: orgID, userID non-empty.
// Post-conditions: user is updated in the store.
// Raises: *domain.NotFoundError if user does not exist.
func (s *UserService) UpdateUser(ctx context.Context, orgID, userID string, params UserUpdateParams) (*domain.User, error) {
	existing, err := s.store.GetUserByID(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}

	if params.Name != nil {
		existing.Name = *params.Name
	}
	if params.Role != nil {
		existing.Role = *params.Role
	}
	if params.IsActive != nil {
		existing.IsActive = *params.IsActive
	}

	if err := s.store.UpdateUser(ctx, existing); err != nil {
		return nil, fmt.Errorf("users.UpdateUser: %w", err)
	}

	s.logger.Info("user updated", "org_id", orgID, "user_id", userID)
	return existing, nil
}

// DeactivateUser sets IsActive=false on the user.
//
// Pre-conditions: orgID, userID non-empty.
// Post-conditions: user.IsActive is false.
// Raises: *domain.NotFoundError if user does not exist.
func (s *UserService) DeactivateUser(ctx context.Context, orgID, userID string) error {
	user, err := s.store.GetUserByID(ctx, orgID, userID)
	if err != nil {
		return err
	}

	user.IsActive = false
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("users.DeactivateUser: %w", err)
	}

	s.logger.Info("user deactivated", "org_id", orgID, "user_id", userID)
	return nil
}

// ListUsers returns a paginated list of users for an org.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns paginated result.
// Raises: error on database failure.
func (s *UserService) ListUsers(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.User], error) {
	return s.store.ListUsers(ctx, orgID, page)
}

// GetUser returns a single user by org and user ID.
//
// Pre-conditions: orgID and userID non-empty.
// Post-conditions: returns the user if found.
// Raises: *domain.NotFoundError if not found.
func (s *UserService) GetUser(ctx context.Context, orgID, userID string) (*domain.User, error) {
	return s.store.GetUserByID(ctx, orgID, userID)
}

// RequestPasswordReset generates a password reset token and stores it.
// If the email does not exist, returns nil silently to avoid leaking
// whether an email is registered.
//
// Pre-conditions: email non-empty.
// Post-conditions: a one-time password reset token is persisted (if email found).
// Raises: error on unexpected store failure.
func (s *UserService) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		var nfErr *domain.NotFoundError
		if isNotFound(err, &nfErr) {
			// Do not reveal whether the email exists.
			return nil
		}
		return fmt.Errorf("users.RequestPasswordReset: %w", err)
	}

	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		return fmt.Errorf("users.RequestPasswordReset: generate token: %w", err)
	}
	_ = plaintext // In production, sent via email; not returned here.

	now := time.Now().UTC()
	token := domain.PasswordResetToken{
		ID:        fmt.Sprintf("prt_%d", now.UnixNano()),
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	}

	if err := s.store.CreatePasswordResetToken(ctx, token); err != nil {
		return fmt.Errorf("users.RequestPasswordReset: %w", err)
	}

	s.logger.Info("password reset requested", "org_id", user.OrgID, "user_id", user.ID)
	return nil
}

// ResetPassword uses a valid reset token to update the user's password.
// The token is marked as used within the same transaction as the password update.
//
// Pre-conditions: token and newPassword non-empty.
// Post-conditions: user's password is updated; token is marked used.
// Raises: *domain.NotFoundError if token is invalid, expired, or already used;
//
//	error on unexpected store failure.
func (s *UserService) ResetPassword(ctx context.Context, token, newPassword string) error {
	tokenHash := auth.HashAPIKey(token)

	resetToken, err := s.store.GetPasswordResetToken(ctx, tokenHash)
	if err != nil {
		return err
	}

	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("users.ResetPassword: hash password: %w", err)
	}

	user, err := s.store.GetUserByIDGlobal(ctx, resetToken.UserID)
	if err != nil {
		return fmt.Errorf("users.ResetPassword: %w", err)
	}

	user.Password = passwordHash

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.UpdateUser(ctx, user); err != nil {
			return err
		}
		return tx.MarkPasswordResetTokenUsed(ctx, resetToken.ID)
	}); err != nil {
		return fmt.Errorf("users.ResetPassword: %w", err)
	}

	s.logger.Info("password reset completed", "user_id", user.ID)
	return nil
}

// validateInviteUser validates the required fields for user invitation.
func validateInviteUser(email, name string, role domain.UserRole) error {
	if email == "" {
		return &domain.ValidationError{
			Message: "email is required",
			Details: map[string]string{"email": "must not be empty"},
		}
	}
	if name == "" {
		return &domain.ValidationError{
			Message: "name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}
	switch role {
	case domain.UserRoleAdmin, domain.UserRoleModerator, domain.UserRoleAnalyst:
		// valid
	default:
		return &domain.ValidationError{
			Message: fmt.Sprintf("invalid role %q", role),
			Details: map[string]string{"role": "must be ADMIN, MODERATOR, or ANALYST"},
		}
	}
	return nil
}

// Login authenticates a user by email and password, creates a server-side session,
// and returns the user, session, and CSRF token.
//
// Pre-conditions: email and password must be non-empty.
// Post-conditions: session is persisted in store; returned session has 24-hour expiry.
// Raises: *domain.ValidationError if email or password is empty.
// Raises: *domain.NotFoundError if email does not exist.
// Raises: *domain.ForbiddenError if password is wrong or user is inactive.
func (s *UserService) Login(ctx context.Context, email, password string) (*domain.User, *domain.Session, string, error) {
	if email == "" || password == "" {
		return nil, nil, "", &domain.ValidationError{
			Message: "email and password are required",
			Details: map[string]string{"email": "must not be empty", "password": "must not be empty"},
		}
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, nil, "", err
	}

	if !auth.CheckPassword(user.Password, password) {
		return nil, nil, "", &domain.ForbiddenError{Message: "invalid credentials"}
	}

	if !user.IsActive {
		return nil, nil, "", &domain.ForbiddenError{Message: "user account is inactive"}
	}

	sid := auth.GenerateSessionID()
	csrfToken := auth.GenerateSessionID()[:32]
	now := time.Now().UTC()

	session := domain.Session{
		SID:    sid,
		UserID: user.ID,
		Data: map[string]any{
			"org_id":     user.OrgID,
			"role":       string(user.Role),
			"csrf_token": csrfToken,
		},
		ExpiresAt: now.Add(24 * time.Hour),
	}

	if err := s.store.CreateSession(ctx, session); err != nil {
		return nil, nil, "", fmt.Errorf("users.Login: create session: %w", err)
	}

	s.logger.Info("user logged in", "org_id", user.OrgID, "user_id", user.ID)
	return user, &session, csrfToken, nil
}

// Logout deletes the session identified by sid.
//
// Pre-conditions: sid must be non-empty.
// Post-conditions: session is deleted (no-op if already absent).
// Raises: error on database failure.
func (s *UserService) Logout(ctx context.Context, sid string) error {
	if err := s.store.DeleteSession(ctx, sid); err != nil {
		return fmt.Errorf("users.Logout: %w", err)
	}
	return nil
}

// isNotFound reports whether err is a *domain.NotFoundError and assigns the pointer.
func isNotFound(err error, target **domain.NotFoundError) bool {
	return errors.As(err, target)
}
