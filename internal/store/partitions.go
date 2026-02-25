package store

import (
	"context"
	"fmt"
)

// CreatePartitionsForMonth creates monthly partitions for rule_executions and
// action_executions tables for the given year and month, if they do not already exist.
//
// Pre-conditions: year > 0, 1 <= month <= 12.
// Post-conditions: partitions exist for the specified month.
// Raises: error on DDL failure.
func (q *Queries) CreatePartitionsForMonth(ctx context.Context, year int, month int) error {
	suffix := fmt.Sprintf("%d_%02d", year, month)
	// Calculate the start of the given month and the start of the next month.
	start := fmt.Sprintf("%d-%02d-01", year, month)

	// Next month calculation.
	nextYear, nextMonth := year, month+1
	if nextMonth > 12 {
		nextYear++
		nextMonth = 1
	}
	end := fmt.Sprintf("%d-%02d-01", nextYear, nextMonth)

	tables := []string{"rule_executions", "action_executions"}
	for _, table := range tables {
		partName := fmt.Sprintf("%s_%s", table, suffix)
		ddl := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
			partName, table, start, end,
		)
		if _, err := q.dbtx.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("create partition %s: %w", partName, err)
		}
	}
	return nil
}
