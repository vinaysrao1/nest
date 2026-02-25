package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vinaysrao1/nest/internal/service"
)

// createTextBankRequest is the decoded body for POST /api/v1/text-banks.
type createTextBankRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// addTextBankEntryRequest is the decoded body for POST /api/v1/text-banks/{id}/entries.
type addTextBankEntryRequest struct {
	Value   string `json:"value"`
	IsRegex bool   `json:"is_regex"`
}

// handleListTextBanks returns all text banks for the authenticated org.
//
// GET /api/v1/text-banks
func handleListTextBanks(svc *service.TextBankService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)

		banks, err := svc.List(r.Context(), orgID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"text_banks": banks,
		})
	}
}

// handleCreateTextBank creates a new text bank.
//
// POST /api/v1/text-banks
func handleCreateTextBank(svc *service.TextBankService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTextBankRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		bank, err := svc.Create(r.Context(), orgID, req.Name, req.Description)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusCreated, bank)
	}
}

// handleGetTextBank returns a single text bank by ID.
//
// GET /api/v1/text-banks/{id}
func handleGetTextBank(svc *service.TextBankService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		bankID := chi.URLParam(r, "id")

		bank, err := svc.Get(r.Context(), orgID, bankID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, bank)
	}
}

// handleListTextBankEntries returns all entries for a text bank.
//
// GET /api/v1/text-banks/{id}/entries
func handleListTextBankEntries(svc *service.TextBankService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		bankID := chi.URLParam(r, "id")

		entries, err := svc.ListEntries(r.Context(), orgID, bankID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"entries": entries,
		})
	}
}

// handleAddTextBankEntry appends an entry to a text bank.
//
// POST /api/v1/text-banks/{id}/entries
func handleAddTextBankEntry(svc *service.TextBankService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addTextBankEntryRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		bankID := chi.URLParam(r, "id")

		entry, err := svc.AddEntry(r.Context(), orgID, bankID, req.Value, req.IsRegex)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusCreated, entry)
	}
}

// handleDeleteTextBankEntry removes a single entry from a text bank.
//
// DELETE /api/v1/text-banks/{id}/entries/{entryId}
func handleDeleteTextBankEntry(svc *service.TextBankService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		bankID := chi.URLParam(r, "id")
		entryID := chi.URLParam(r, "entryId")

		if err := svc.DeleteEntry(r.Context(), orgID, bankID, entryID); err != nil {
			mapError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
