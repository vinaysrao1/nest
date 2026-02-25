// Package handler implements HTTP handlers and chi router construction for the
// Nest content moderation rules engine. All handlers operate through service
// interfaces; this package never imports the store layer directly.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
	maxBodyBytes    = 1 << 20 // 1 MiB
)

// ErrorResponse is the standard JSON error shape returned by all endpoints.
type ErrorResponse struct {
	Error   string            `json:"error"`
	Details map[string]string `json:"details,omitempty"`
}

// JSON writes v as a JSON response with the given HTTP status code.
// Sets Content-Type to application/json.
//
// Pre-conditions: v must be JSON-serializable.
// Post-conditions: response body is JSON-encoded v; status code is set.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().Error("handler: failed to encode JSON response", "error", err)
	}
}

// Decode reads and JSON-decodes the request body into v.
// The body is limited to 1MB to prevent abuse.
//
// Pre-conditions: r.Body must not be nil; v must be a pointer.
// Post-conditions: v is populated from the request body.
// Raises: error if body is empty, too large, or not valid JSON.
func Decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Error writes a JSON ErrorResponse with the given status code and message.
//
// Pre-conditions: msg must be non-empty.
// Post-conditions: response is a JSON ErrorResponse.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, ErrorResponse{Error: msg})
}

// ErrorWithDetails writes a JSON ErrorResponse with field-level details.
//
// Pre-conditions: msg must be non-empty.
// Post-conditions: response is a JSON ErrorResponse with Details populated.
func ErrorWithDetails(w http.ResponseWriter, status int, msg string, details map[string]string) {
	JSON(w, status, ErrorResponse{Error: msg, Details: details})
}

// OrgID extracts the org ID from the request context (set by auth middleware).
//
// Pre-conditions: auth middleware has run and set AuthContext.
// Post-conditions: returns the org ID or empty string if not authenticated.
func OrgID(r *http.Request) string {
	return auth.OrgIDFromContext(r.Context())
}

// UserID extracts the user ID from the request context.
//
// Pre-conditions: auth middleware has run and set AuthContext.
// Post-conditions: returns the user ID or empty string if not authenticated.
func UserID(r *http.Request) string {
	return auth.UserIDFromContext(r.Context())
}

// PageParamsFromRequest extracts page and page_size from query string.
// Defaults: page=1, page_size=20. Maximum page_size=100.
//
// Pre-conditions: none.
// Post-conditions: returns valid PageParams with page >= 1, 1 <= page_size <= 100.
func PageParamsFromRequest(r *http.Request) domain.PageParams {
	page := parseIntQuery(r, "page", defaultPage)
	pageSize := parseIntQuery(r, "page_size", defaultPageSize)

	if page < 1 {
		page = defaultPage
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	return domain.PageParams{
		Page:     page,
		PageSize: pageSize,
	}
}

// parseIntQuery reads a query parameter as an int, returning defaultVal on absence or error.
func parseIntQuery(r *http.Request, key string, defaultVal int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return v
}

// mapError inspects err and writes the appropriate HTTP error response.
// Maps domain error types to HTTP status codes:
//   - *domain.NotFoundError     -> 404
//   - *domain.ValidationError   -> 400 (with details if present)
//   - *domain.CompileError      -> 422
//   - *domain.ForbiddenError    -> 403
//   - *domain.ConflictError     -> 409
//   - all others                -> 500
//
// Pre-conditions: err must be non-nil.
// Post-conditions: appropriate HTTP error response is written.
func mapError(w http.ResponseWriter, err error, logger *slog.Logger) {
	var nfErr *domain.NotFoundError
	var valErr *domain.ValidationError
	var compErr *domain.CompileError
	var forbErr *domain.ForbiddenError
	var confErr *domain.ConflictError

	switch {
	case errors.As(err, &nfErr):
		Error(w, http.StatusNotFound, nfErr.Message)
	case errors.As(err, &valErr):
		ErrorWithDetails(w, http.StatusBadRequest, valErr.Message, valErr.Details)
	case errors.As(err, &compErr):
		JSON(w, http.StatusUnprocessableEntity, compErr)
	case errors.As(err, &forbErr):
		Error(w, http.StatusForbidden, forbErr.Message)
	case errors.As(err, &confErr):
		Error(w, http.StatusConflict, confErr.Message)
	default:
		logger.Error("internal error", "error", err)
		Error(w, http.StatusInternalServerError, "internal server error")
	}
}
