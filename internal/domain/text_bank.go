package domain

import "time"

// TextBank is a named collection of text patterns used by the text-bank signal adapter.
type TextBank struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TextBankEntry is a single entry in a text bank.
type TextBankEntry struct {
	ID         string    `json:"id"`
	TextBankID string    `json:"text_bank_id"`
	Value      string    `json:"value"`
	IsRegex    bool      `json:"is_regex"`
	CreatedAt  time.Time `json:"created_at"`
}
