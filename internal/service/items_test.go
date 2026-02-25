package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

// TestSubmitSync_StoresItem verifies that SubmitSync persists each item to the database.
func TestSubmitSync_StoresItem(t *testing.T) {
	svc, q, cleanup := setupItemService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestSubmitSync_StoresItem")
	it := seedItemType(t, q, orgID, "content")

	itemID := generateTestID("item")
	params := []service.SubmitItemParams{
		{
			ItemID:     itemID,
			ItemTypeID: it.ID,
			OrgID:      orgID,
			Payload:    map[string]any{"text": "hello world"},
		},
	}

	results, err := svc.SubmitSync(ctx, orgID, params)
	if err != nil {
		t.Fatalf("SubmitSync returned unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ItemID != itemID {
		t.Errorf("result.ItemID = %q, want %q", results[0].ItemID, itemID)
	}
	// Default verdict is "approve" since no rules are loaded.
	if results[0].Verdict != domain.VerdictApprove {
		t.Errorf("expected verdict %q, got %q", domain.VerdictApprove, results[0].Verdict)
	}
}

// TestSubmitSync_InvalidItemType verifies that SubmitSync returns a NotFoundError when
// the item type does not exist.
func TestSubmitSync_InvalidItemType(t *testing.T) {
	svc, q, cleanup := setupItemService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestSubmitSync_InvalidItemType")

	params := []service.SubmitItemParams{
		{
			ItemID:     generateTestID("item"),
			ItemTypeID: "ity_nonexistent",
			OrgID:      orgID,
			Payload:    map[string]any{"text": "hello"},
		},
	}

	_, err := svc.SubmitSync(ctx, orgID, params)
	if err == nil {
		t.Fatal("expected error for nonexistent item type, got nil")
	}

	var nfErr *domain.NotFoundError
	if !isNotFoundError(err, &nfErr) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// TestSubmitSync_MultipleItems verifies that SubmitSync handles a batch and returns
// one result per item.
func TestSubmitSync_MultipleItems(t *testing.T) {
	svc, q, cleanup := setupItemService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestSubmitSync_MultipleItems")
	it := seedItemType(t, q, orgID, "content")

	const count = 3
	params := make([]service.SubmitItemParams, count)
	for i := range count {
		params[i] = service.SubmitItemParams{
			ItemID:     generateTestID("item"),
			ItemTypeID: it.ID,
			OrgID:      orgID,
			Payload:    map[string]any{"index": i},
		}
		// Small sleep to avoid identical UnixNano IDs.
		time.Sleep(time.Millisecond)
	}

	results, err := svc.SubmitSync(ctx, orgID, params)
	if err != nil {
		t.Fatalf("SubmitSync returned unexpected error: %v", err)
	}
	if len(results) != count {
		t.Fatalf("expected %d results, got %d", count, len(results))
	}
	for i, r := range results {
		if r.ItemID != params[i].ItemID {
			t.Errorf("result[%d].ItemID = %q, want %q", i, r.ItemID, params[i].ItemID)
		}
	}
}

// TestSubmitSync_ValidationError verifies that SubmitSync returns a ValidationError
// when ItemTypeID is empty.
func TestSubmitSync_ValidationError(t *testing.T) {
	svc, q, cleanup := setupItemService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestSubmitSync_ValidationError")

	params := []service.SubmitItemParams{
		{
			ItemID:     generateTestID("item"),
			ItemTypeID: "", // missing
			OrgID:      orgID,
			Payload:    map[string]any{"text": "hello"},
		},
	}

	_, err := svc.SubmitSync(ctx, orgID, params)
	if err == nil {
		t.Fatal("expected ValidationError for empty ItemTypeID, got nil")
	}
	var valErr *domain.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

// TestSubmitSync_NilPayloadValidation verifies that SubmitSync returns a ValidationError
// when Payload is nil.
func TestSubmitSync_NilPayloadValidation(t *testing.T) {
	svc, q, cleanup := setupItemService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestSubmitSync_NilPayloadValidation")
	it := seedItemType(t, q, orgID, "content")

	params := []service.SubmitItemParams{
		{
			ItemID:     generateTestID("item"),
			ItemTypeID: it.ID,
			OrgID:      orgID,
			Payload:    nil, // missing
		},
	}

	_, err := svc.SubmitSync(ctx, orgID, params)
	if err == nil {
		t.Fatal("expected ValidationError for nil Payload, got nil")
	}
	var valErr *domain.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

// TestSubmitAsync_ValidatesAndStores verifies that SubmitAsync validates item types,
// stores the items, and returns one submission ID per item.
func TestSubmitAsync_ValidatesAndStores(t *testing.T) {
	svc, q, cleanup := setupItemService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestSubmitAsync_ValidatesAndStores")
	it := seedItemType(t, q, orgID, "content")

	const count = 2
	params := make([]service.SubmitItemParams, count)
	for i := range count {
		params[i] = service.SubmitItemParams{
			ItemID:     generateTestID("item"),
			ItemTypeID: it.ID,
			OrgID:      orgID,
			Payload:    map[string]any{"index": i},
		}
		time.Sleep(time.Millisecond)
	}

	subIDs, err := svc.SubmitAsync(ctx, orgID, params)
	if err != nil {
		t.Fatalf("SubmitAsync returned unexpected error: %v", err)
	}
	if len(subIDs) != count {
		t.Fatalf("expected %d submission IDs, got %d", count, len(subIDs))
	}
	for i, id := range subIDs {
		if id == "" {
			t.Errorf("submission ID at index %d is empty", i)
		}
	}
}

// TestSubmitAsync_InvalidItemType verifies that SubmitAsync returns a NotFoundError
// when an item's type does not exist.
func TestSubmitAsync_InvalidItemType(t *testing.T) {
	svc, q, cleanup := setupItemService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestSubmitAsync_InvalidItemType")

	params := []service.SubmitItemParams{
		{
			ItemID:     generateTestID("item"),
			ItemTypeID: "ity_does_not_exist",
			OrgID:      orgID,
			Payload:    map[string]any{"text": "hello"},
		},
	}

	_, err := svc.SubmitAsync(ctx, orgID, params)
	if err == nil {
		t.Fatal("expected error for nonexistent item type, got nil")
	}
	var nfErr *domain.NotFoundError
	if !isNotFoundError(err, &nfErr) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// isNotFoundError checks whether err is or wraps a *domain.NotFoundError.
func isNotFoundError(err error, target **domain.NotFoundError) bool {
	var nfErr *domain.NotFoundError
	if asErr, ok := err.(*domain.NotFoundError); ok {
		*target = asErr
		return true
	}
	// Walk wrapped errors.
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if asErr, ok := err.(*domain.NotFoundError); ok {
			*target = asErr
			return true
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	_ = nfErr
	return false
}

// isValidationError checks whether err is a *domain.ValidationError.
func isValidationError(err error, target **domain.ValidationError) bool {
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if asErr, ok := err.(*domain.ValidationError); ok {
			*target = asErr
			return true
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
