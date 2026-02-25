package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// noopMiddleware is a pass-through middleware used in router construction tests.
func noopMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// TestHandleHealth verifies the health endpoint returns 200 {"status":"ok"}.
func TestHandleHealth(t *testing.T) {
	t.Parallel()

	handler := handleHealth()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status field: got %q, want %q", resp["status"], "ok")
	}
}

// TestHandleListUDFs verifies the UDFs endpoint returns the hardcoded list.
func TestHandleListUDFs(t *testing.T) {
	t.Parallel()

	handler := handleListUDFs()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/udfs", nil)

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	udfs, ok := resp["udfs"].([]any)
	if !ok {
		t.Fatalf("expected 'udfs' array, got %T", resp["udfs"])
	}

	const wantCount = 9
	if len(udfs) != wantCount {
		t.Errorf("udf count: got %d, want %d", len(udfs), wantCount)
	}
}

// TestHandleListUDFs_Names checks that all expected UDF names are present.
func TestHandleListUDFs_Names(t *testing.T) {
	t.Parallel()

	handler := handleListUDFs()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/udfs", nil)

	handler.ServeHTTP(w, r)

	var resp struct {
		UDFs []udfDescriptor `json:"udfs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	expectedNames := map[string]bool{
		"verdict":     false,
		"signal":      false,
		"counter":     false,
		"memo":        false,
		"log":         false,
		"now":         false,
		"hash":        false,
		"regex_match": false,
		"enqueue":     false,
	}

	for _, udf := range resp.UDFs {
		if _, known := expectedNames[udf.Name]; !known {
			t.Errorf("unexpected UDF name: %q", udf.Name)
		}
		expectedNames[udf.Name] = true
	}

	for name, seen := range expectedNames {
		if !seen {
			t.Errorf("missing UDF: %q", name)
		}
	}
}

// TestHandleMe_NoAuth verifies handleMe returns 401 when auth context is absent.
func TestHandleMe_NoAuth(t *testing.T) {
	t.Parallel()

	handler := handleMe()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
