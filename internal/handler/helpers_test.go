package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vinaysrao1/nest/internal/domain"
)

// ---- PageParamsFromRequest tests --------------------------------------------

func TestPageParamsFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		query            string
		wantPage         int
		wantPageSize     int
	}{
		{
			name:         "defaults when no params",
			query:        "",
			wantPage:     1,
			wantPageSize: 20,
		},
		{
			name:         "explicit valid values",
			query:        "page=3&page_size=50",
			wantPage:     3,
			wantPageSize: 50,
		},
		{
			name:         "page_size capped at 100",
			query:        "page=1&page_size=999",
			wantPage:     1,
			wantPageSize: 100,
		},
		{
			name:         "page_size exactly 100 not capped",
			query:        "page=2&page_size=100",
			wantPage:     2,
			wantPageSize: 100,
		},
		{
			name:         "invalid page falls back to default",
			query:        "page=abc&page_size=10",
			wantPage:     1,
			wantPageSize: 10,
		},
		{
			name:         "invalid page_size falls back to default",
			query:        "page=2&page_size=notanumber",
			wantPage:     2,
			wantPageSize: 20,
		},
		{
			name:         "zero page clamps to 1",
			query:        "page=0",
			wantPage:     1,
			wantPageSize: 20,
		},
		{
			name:         "negative page_size clamps to default",
			query:        "page_size=-5",
			wantPage:     1,
			wantPageSize: 20,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
			p := PageParamsFromRequest(r)
			if p.Page != tc.wantPage {
				t.Errorf("page: got %d, want %d", p.Page, tc.wantPage)
			}
			if p.PageSize != tc.wantPageSize {
				t.Errorf("page_size: got %d, want %d", p.PageSize, tc.wantPageSize)
			}
		})
	}
}

// ---- Decode tests -----------------------------------------------------------

func TestDecode(t *testing.T) {
	t.Parallel()

	type target struct {
		Name string `json:"name"`
	}

	t.Run("valid JSON is decoded", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"hello"}`))
		var v target
		if err := Decode(r, &v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.Name != "hello" {
			t.Errorf("got name=%q, want %q", v.Name, "hello")
		}
	})

	t.Run("empty body returns error", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		var v target
		if err := Decode(r, &v); err == nil {
			t.Fatal("expected error for empty body, got nil")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{bad json}"))
		var v target
		if err := Decode(r, &v); err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})
}

// ---- JSON helper tests ------------------------------------------------------

func TestJSON(t *testing.T) {
	t.Parallel()

	t.Run("sets Content-Type and status", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		JSON(w, http.StatusCreated, map[string]string{"key": "val"})
		if w.Code != http.StatusCreated {
			t.Errorf("status: got %d, want %d", w.Code, http.StatusCreated)
		}
		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("content-type: got %q, want application/json", ct)
		}
	})

	t.Run("body is JSON-encoded", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		JSON(w, http.StatusOK, map[string]int{"count": 42})
		var got map[string]int
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["count"] != 42 {
			t.Errorf("count: got %d, want 42", got["count"])
		}
	})
}

// ---- Error / ErrorWithDetails tests -----------------------------------------

func TestError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Error(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "bad input" {
		t.Errorf("error: got %q, want %q", resp.Error, "bad input")
	}
}

func TestErrorWithDetails(t *testing.T) {
	t.Parallel()

	details := map[string]string{"field": "required"}
	w := httptest.NewRecorder()
	ErrorWithDetails(w, http.StatusBadRequest, "validation failed", details)

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "validation failed" {
		t.Errorf("error: got %q, want %q", resp.Error, "validation failed")
	}
	if resp.Details["field"] != "required" {
		t.Errorf("detail: got %q, want %q", resp.Details["field"], "required")
	}
}

// ---- mapError tests ---------------------------------------------------------

func TestMapError(t *testing.T) {
	t.Parallel()

	logger := newNopLogger()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "NotFoundError -> 404",
			err:        &domain.NotFoundError{Message: "rule not found"},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ValidationError -> 400",
			err: &domain.ValidationError{
				Message: "name required",
				Details: map[string]string{"name": "must not be empty"},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "CompileError -> 422",
			err:        &domain.CompileError{Message: "syntax error", Line: 3},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "ForbiddenError -> 403",
			err:        &domain.ForbiddenError{Message: "access denied"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "ConflictError -> 409",
			err:        &domain.ConflictError{Message: "duplicate name"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "generic error -> 500",
			err:        errSentinel("something went wrong"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			mapError(w, tc.err, logger)
			if w.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

// ---- Helpers ----------------------------------------------------------------

// errSentinel is a plain error value for testing.
type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// newNopLogger returns a logger that discards all output.
func newNopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}
