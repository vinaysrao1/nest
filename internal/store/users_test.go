package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestUsers_CRUD(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "users-crud-org")

	t.Run("create and get by ID", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		user := &domain.User{
			ID:        "user-001",
			OrgID:     orgID,
			Email:     "alice@example.com",
			Name:      "Alice",
			Password:  "$2a$10$hashedpassword",
			Role:      domain.UserRoleAdmin,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := q.CreateUser(ctx, user); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}

		got, err := q.GetUserByID(ctx, orgID, user.ID)
		if err != nil {
			t.Fatalf("GetUserByID: %v", err)
		}

		if got.ID != user.ID {
			t.Errorf("ID: got %q, want %q", got.ID, user.ID)
		}
		if got.OrgID != user.OrgID {
			t.Errorf("OrgID: got %q, want %q", got.OrgID, user.OrgID)
		}
		if got.Email != user.Email {
			t.Errorf("Email: got %q, want %q", got.Email, user.Email)
		}
		if got.Name != user.Name {
			t.Errorf("Name: got %q, want %q", got.Name, user.Name)
		}
		if got.Password != user.Password {
			t.Errorf("Password: got %q, want %q", got.Password, user.Password)
		}
		if got.Role != user.Role {
			t.Errorf("Role: got %q, want %q", got.Role, user.Role)
		}
		if got.IsActive != user.IsActive {
			t.Errorf("IsActive: got %v, want %v", got.IsActive, user.IsActive)
		}
	})

	t.Run("update user", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		user := &domain.User{
			ID:        "user-to-update",
			OrgID:     orgID,
			Email:     "bob@example.com",
			Name:      "Bob",
			Password:  "old-hash",
			Role:      domain.UserRoleModerator,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := q.CreateUser(ctx, user); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}

		user.Name = "Bob Updated"
		user.Password = "new-hash"
		user.Role = domain.UserRoleAnalyst
		user.IsActive = false

		if err := q.UpdateUser(ctx, user); err != nil {
			t.Fatalf("UpdateUser: %v", err)
		}

		got, err := q.GetUserByID(ctx, orgID, user.ID)
		if err != nil {
			t.Fatalf("GetUserByID after update: %v", err)
		}

		if got.Name != "Bob Updated" {
			t.Errorf("Name: got %q, want %q", got.Name, "Bob Updated")
		}
		if got.Password != "new-hash" {
			t.Errorf("Password: got %q, want %q", got.Password, "new-hash")
		}
		if got.Role != domain.UserRoleAnalyst {
			t.Errorf("Role: got %q, want %q", got.Role, domain.UserRoleAnalyst)
		}
		if got.IsActive {
			t.Errorf("IsActive: expected false, got true")
		}
	})

	t.Run("delete user", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		user := &domain.User{
			ID:        "user-to-delete",
			OrgID:     orgID,
			Email:     "delete-me@example.com",
			Name:      "Delete Me",
			Password:  "hash",
			Role:      domain.UserRoleAnalyst,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := q.CreateUser(ctx, user); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}

		if err := q.DeleteUser(ctx, orgID, user.ID); err != nil {
			t.Fatalf("DeleteUser: %v", err)
		}

		_, err := q.GetUserByID(ctx, orgID, user.ID)
		if err == nil {
			t.Fatal("expected NotFoundError after delete, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("list users with pagination", func(t *testing.T) {
		orgID2 := seedOrg(t, q, "list-users-org")
		now := time.Now().UTC().Truncate(time.Microsecond)
		for i, email := range []string{"u1@test.com", "u2@test.com", "u3@test.com"} {
			u := &domain.User{
				ID:        "list-user-" + string(rune('0'+i+1)),
				OrgID:     orgID2,
				Email:     email,
				Name:      "User " + string(rune('0'+i+1)),
				Password:  "hash",
				Role:      domain.UserRoleAnalyst,
				IsActive:  true,
				CreatedAt: now.Add(time.Duration(i) * time.Second),
				UpdatedAt: now.Add(time.Duration(i) * time.Second),
			}
			if err := q.CreateUser(ctx, u); err != nil {
				t.Fatalf("CreateUser(%s): %v", email, err)
			}
		}

		result, err := q.ListUsers(ctx, orgID2, domain.PageParams{Page: 1, PageSize: 2})
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if result.Total != 3 {
			t.Errorf("Total: got %d, want 3", result.Total)
		}
		if len(result.Items) != 2 {
			t.Errorf("Items len: got %d, want 2", len(result.Items))
		}
		if result.TotalPages != 2 {
			t.Errorf("TotalPages: got %d, want 2", result.TotalPages)
		}
	})
}

func TestUsers_GetByEmail(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "get-by-email-org")

	t.Run("get user by email", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		user := &domain.User{
			ID:        "email-user-001",
			OrgID:     orgID,
			Email:     "findme@example.com",
			Name:      "Find Me",
			Password:  "hash",
			Role:      domain.UserRoleAdmin,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := q.CreateUser(ctx, user); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}

		got, err := q.GetUserByEmail(ctx, user.Email)
		if err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		}

		if got.ID != user.ID {
			t.Errorf("ID: got %q, want %q", got.ID, user.ID)
		}
		if got.OrgID != user.OrgID {
			t.Errorf("OrgID: got %q, want %q", got.OrgID, user.OrgID)
		}
		if got.Email != user.Email {
			t.Errorf("Email: got %q, want %q", got.Email, user.Email)
		}
	})

	t.Run("get non-existent email returns NotFoundError", func(t *testing.T) {
		_, err := q.GetUserByEmail(ctx, "nobody@example.com")
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestUsers_Conflict(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "users-conflict-org")

	t.Run("duplicate email in same org returns ConflictError", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		user1 := &domain.User{
			ID:        "conflict-user-1",
			OrgID:     orgID,
			Email:     "dup@example.com",
			Name:      "User One",
			Password:  "hash",
			Role:      domain.UserRoleAdmin,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := q.CreateUser(ctx, user1); err != nil {
			t.Fatalf("CreateUser(first): %v", err)
		}

		user2 := &domain.User{
			ID:        "conflict-user-2",
			OrgID:     orgID,
			Email:     "dup@example.com", // same email, same org
			Name:      "User Two",
			Password:  "hash2",
			Role:      domain.UserRoleModerator,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		err := q.CreateUser(ctx, user2)
		if err == nil {
			t.Fatal("expected ConflictError for duplicate email, got nil")
		}
		if _, ok := err.(*domain.ConflictError); !ok {
			t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
		}
	})
}

func TestUsers_NotFound(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "users-notfound-org")

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "GetUserByID non-existent",
			fn: func() error {
				_, err := q.GetUserByID(ctx, orgID, "no-such-user")
				return err
			},
		},
		{
			name: "UpdateUser non-existent",
			fn: func() error {
				return q.UpdateUser(ctx, &domain.User{
					ID:    "no-such-user",
					OrgID: orgID,
				})
			},
		},
		{
			name: "DeleteUser non-existent",
			fn: func() error {
				return q.DeleteUser(ctx, orgID, "no-such-user")
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatal("expected NotFoundError, got nil")
			}
			var nfe *domain.NotFoundError
			if !isNotFound(err, &nfe) {
				t.Errorf("expected NotFoundError, got %T: %v", err, err)
			}
		})
	}
}
