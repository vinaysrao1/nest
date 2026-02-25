package store

// rowScanner is satisfied by both pgx.Row and pgx.Rows, enabling shared scan
// helper functions that work for both QueryRow (single row) and Query (multiple rows).
type rowScanner interface {
	Scan(dest ...any) error
}
