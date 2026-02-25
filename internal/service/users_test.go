package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

func TestInviteUser_HashesPassword(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-invite-hash")
	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	user, err := svc.InviteUser(ctx, orgID, "alice@example.com", "Alice", domain.UserRoleAdmin)
	if err != nil {
		t.Fatalf("InviteUser: unexpected error: %v", err)
	}

	// The stored value must be a bcrypt hash, not the 16-char hex temp password.
	if user.Password == "" {
		t.Fatal("InviteUser: password field is empty")
	}
	if len(user.Password) <= 16 {
		t.Errorf("InviteUser: password appears to be plaintext (len=%d), expected bcrypt hash", len(user.Password))
	}
	// bcrypt hashes start with $2 and have length >= 60.
	if len(user.Password) < 60 {
		t.Errorf("InviteUser: password too short for bcrypt hash (len=%d)", len(user.Password))
	}
}

func TestInviteUser_DuplicateEmail(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-invite-dup")
	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	if _, err := svc.InviteUser(ctx, orgID, "dup@example.com", "First", domain.UserRoleAdmin); err != nil {
		t.Fatalf("InviteUser first: %v", err)
	}

	_, err := svc.InviteUser(ctx, orgID, "dup@example.com", "Second", domain.UserRoleModerator)
	if err == nil {
		t.Fatal("InviteUser: expected ConflictError for duplicate email, got nil")
	}
	if _, ok := err.(*domain.ConflictError); !ok {
		t.Errorf("InviteUser: expected *domain.ConflictError, got %T: %v", err, err)
	}
}

func TestInviteUser_ValidationErrors(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	tests := []struct {
		name  string
		email string
		uname string
		role  domain.UserRole
	}{
		{"empty email", "", "Alice", domain.UserRoleAdmin},
		{"empty name", "a@b.com", "", domain.UserRoleAdmin},
		{"invalid role", "a@b.com", "Alice", "SUPERUSER"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.InviteUser(ctx, "org1", tc.email, tc.uname, tc.role)
			if err == nil {
				t.Fatal("expected ValidationError, got nil")
			}
			if _, ok := err.(*domain.ValidationError); !ok {
				t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
			}
		})
	}
}

func TestUpdateUser_PartialUpdate(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-update-user")
	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	user, err := svc.InviteUser(ctx, orgID, "bob@example.com", "Bob", domain.UserRoleAnalyst)
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	newName := "Bobby"
	updated, err := svc.UpdateUser(ctx, orgID, user.ID, service.UserUpdateParams{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	if updated.Name != newName {
		t.Errorf("UpdateUser: Name want %q, got %q", newName, updated.Name)
	}
	// Role should be unchanged.
	if updated.Role != domain.UserRoleAnalyst {
		t.Errorf("UpdateUser: Role should be unchanged, got %q", updated.Role)
	}
	// IsActive should be unchanged.
	if !updated.IsActive {
		t.Error("UpdateUser: IsActive should still be true")
	}
}

func TestDeactivateUser(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-deactivate")
	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	user, err := svc.InviteUser(ctx, orgID, "charlie@example.com", "Charlie", domain.UserRoleAdmin)
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	if err := svc.DeactivateUser(ctx, orgID, user.ID); err != nil {
		t.Fatalf("DeactivateUser: %v", err)
	}

	fetched, err := svc.GetUser(ctx, orgID, user.ID)
	if err != nil {
		t.Fatalf("GetUser after deactivate: %v", err)
	}
	if fetched.IsActive {
		t.Error("DeactivateUser: IsActive should be false after deactivation")
	}
}

func TestListUsers(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-list-users")
	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	emails := []string{"u1@example.com", "u2@example.com", "u3@example.com"}
	for _, e := range emails {
		if _, err := svc.InviteUser(ctx, orgID, e, "User", domain.UserRoleAnalyst); err != nil {
			t.Fatalf("InviteUser %s: %v", e, err)
		}
	}

	result, err := svc.ListUsers(ctx, orgID, domain.PageParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if result.Total < len(emails) {
		t.Errorf("ListUsers: want at least %d users, got %d", len(emails), result.Total)
	}
}

func TestRequestPasswordReset_NonexistentEmail(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	// Should return nil — do not leak whether email exists.
	err := svc.RequestPasswordReset(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset with nonexistent email: expected nil, got %v", err)
	}
}

func TestResetPassword_ValidToken(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-reset-pw")
	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	user, err := svc.InviteUser(ctx, orgID, "dave@example.com", "Dave", domain.UserRoleAdmin)
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}
	originalHash := user.Password

	// Directly create a reset token (simulates what RequestPasswordReset persists).
	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	now := time.Now().UTC()
	token := domain.PasswordResetToken{
		ID:        generateTestID("prt"),
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	}
	if err := q.CreatePasswordResetToken(ctx, token); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}

	newPassword := "new-secure-password-123"
	if err := svc.ResetPassword(ctx, plaintext, newPassword); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// Verify new password is stored.
	updated, err := svc.GetUser(ctx, orgID, user.ID)
	if err != nil {
		t.Fatalf("GetUser after reset: %v", err)
	}
	if updated.Password == originalHash {
		t.Error("ResetPassword: password was not changed")
	}
	if !auth.CheckPassword(updated.Password, newPassword) {
		t.Error("ResetPassword: new password hash does not match expected")
	}

	// Second use of the same token should fail (token is marked used).
	err = svc.ResetPassword(ctx, plaintext, "another-password")
	if err == nil {
		t.Fatal("ResetPassword: second use of same token should fail, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("ResetPassword: second use: expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-expired-token")
	svc := service.NewUserService(q, testLogger())
	ctx := context.Background()

	user, err := svc.InviteUser(ctx, orgID, "expired@example.com", "Expired", domain.UserRoleAdmin)
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	now := time.Now().UTC()
	token := domain.PasswordResetToken{
		ID:        generateTestID("prt"),
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: now.Add(-time.Hour), // already expired
		CreatedAt: now.Add(-2 * time.Hour),
	}
	if err := q.CreatePasswordResetToken(ctx, token); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}

	err = svc.ResetPassword(ctx, plaintext, "new-password")
	if err == nil {
		t.Fatal("ResetPassword: expected error for expired token, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("ResetPassword: expected *domain.NotFoundError for expired token, got %T: %v", err, err)
	}
}
