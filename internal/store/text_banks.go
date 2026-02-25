package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

// ListTextBanks returns all text banks for an org, ordered by name ASC.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all text banks for the org.
// Raises: error on database failure.
func (q *Queries) ListTextBanks(ctx context.Context, orgID string) ([]domain.TextBank, error) {
	const sql = `
		SELECT id, org_id, name, COALESCE(description, ''), created_at, updated_at
		FROM text_banks
		WHERE org_id = $1
		ORDER BY name ASC`

	rows, err := q.dbtx.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("list text banks: %w", err)
	}
	defer rows.Close()

	banks := make([]domain.TextBank, 0)
	for rows.Next() {
		b, err := scanTextBank(rows)
		if err != nil {
			return nil, fmt.Errorf("scan text bank: %w", err)
		}
		banks = append(banks, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error listing text banks: %w", err)
	}
	return banks, nil
}

// GetTextBank returns a single text bank by org and bank ID.
//
// Pre-conditions: orgID, bankID must be non-empty.
// Post-conditions: returns the matching text bank.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetTextBank(ctx context.Context, orgID, bankID string) (*domain.TextBank, error) {
	const sql = `
		SELECT id, org_id, name, COALESCE(description, ''), created_at, updated_at
		FROM text_banks
		WHERE org_id = $1 AND id = $2`

	row := q.dbtx.QueryRow(ctx, sql, orgID, bankID)
	b, err := scanTextBank(row)
	if err != nil {
		return nil, notFound(err, "text bank", bankID)
	}
	return b, nil
}

// CreateTextBank inserts a new text bank.
//
// Pre-conditions: bank.ID, bank.OrgID, bank.Name must be set.
// Post-conditions: text bank is persisted.
// Raises: error on database failure.
func (q *Queries) CreateTextBank(ctx context.Context, bank *domain.TextBank) error {
	const sql = `
		INSERT INTO text_banks (id, org_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := q.dbtx.Exec(ctx, sql,
		bank.ID,
		bank.OrgID,
		bank.Name,
		bank.Description,
		bank.CreatedAt,
		bank.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create text bank: %w", err)
	}
	return nil
}

// AddTextBankEntry adds an entry to a text bank.
// Enforces org isolation by verifying the bank belongs to the org via a subquery.
//
// Pre-conditions: orgID, entry.ID, entry.TextBankID must be set.
// Post-conditions: entry is persisted.
// Raises: domain.NotFoundError if the bank does not exist in the org.
func (q *Queries) AddTextBankEntry(ctx context.Context, orgID string, entry *domain.TextBankEntry) error {
	const sql = `
		INSERT INTO text_bank_entries (id, text_bank_id, value, is_regex, created_at)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (
			SELECT 1 FROM text_banks WHERE id = $2 AND org_id = $6
		)`

	tag, err := q.dbtx.Exec(ctx, sql,
		entry.ID,
		entry.TextBankID,
		entry.Value,
		entry.IsRegex,
		entry.CreatedAt,
		orgID,
	)
	if err != nil {
		return fmt.Errorf("add text bank entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("text bank %s not found in org %s", entry.TextBankID, orgID)}
	}
	return nil
}

// DeleteTextBankEntry removes an entry from a text bank.
// Enforces org isolation by joining through text_banks.
//
// Pre-conditions: orgID, bankID, entryID must be non-empty.
// Post-conditions: entry is deleted.
// Raises: domain.NotFoundError if entry or bank not found in org.
func (q *Queries) DeleteTextBankEntry(ctx context.Context, orgID, bankID, entryID string) error {
	const sql = `
		DELETE FROM text_bank_entries
		WHERE id = $1 AND text_bank_id = $2
		AND EXISTS (
			SELECT 1 FROM text_banks WHERE id = $2 AND org_id = $3
		)`

	tag, err := q.dbtx.Exec(ctx, sql, entryID, bankID, orgID)
	if err != nil {
		return fmt.Errorf("delete text bank entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("text bank entry %s not found", entryID)}
	}
	return nil
}

// GetTextBankEntries returns all entries for a text bank.
// Enforces org isolation by joining through text_banks.
//
// Pre-conditions: orgID, bankID must be non-empty.
// Post-conditions: returns all entries ordered by created_at ASC.
// Raises: error on database failure.
func (q *Queries) GetTextBankEntries(ctx context.Context, orgID, bankID string) ([]domain.TextBankEntry, error) {
	const sql = `
		SELECT tbe.id, tbe.text_bank_id, tbe.value, tbe.is_regex, tbe.created_at
		FROM text_bank_entries tbe
		JOIN text_banks tb ON tb.id = tbe.text_bank_id
		WHERE tb.org_id = $1 AND tbe.text_bank_id = $2
		ORDER BY tbe.created_at ASC`

	rows, err := q.dbtx.Query(ctx, sql, orgID, bankID)
	if err != nil {
		return nil, fmt.Errorf("get text bank entries: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.TextBankEntry, 0)
	for rows.Next() {
		var e domain.TextBankEntry
		if err := rows.Scan(
			&e.ID,
			&e.TextBankID,
			&e.Value,
			&e.IsRegex,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan text bank entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error getting text bank entries: %w", err)
	}
	return entries, nil
}

// scanTextBank scans a row into a domain.TextBank.
func scanTextBank(row rowScanner) (*domain.TextBank, error) {
	var b domain.TextBank
	err := row.Scan(
		&b.ID,
		&b.OrgID,
		&b.Name,
		&b.Description,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
