package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/vinaysrao1/nest/internal/domain"
)

// ---- paginationOffset -------------------------------------------------------

func Test_paginationOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		page     domain.PageParams
		expected int
	}{
		{
			name:     "page 1 size 20 gives offset 0",
			page:     domain.PageParams{Page: 1, PageSize: 20},
			expected: 0,
		},
		{
			name:     "page 2 size 20 gives offset 20",
			page:     domain.PageParams{Page: 2, PageSize: 20},
			expected: 20,
		},
		{
			name:     "page 3 size 10 gives offset 20",
			page:     domain.PageParams{Page: 3, PageSize: 10},
			expected: 20,
		},
		{
			name:     "page 0 is clamped to 1 giving offset 0",
			page:     domain.PageParams{Page: 0, PageSize: 20},
			expected: 0,
		},
		{
			name:     "page -1 is clamped to 1 giving offset 0",
			page:     domain.PageParams{Page: -1, PageSize: 20},
			expected: 0,
		},
		{
			name:     "page 0 size 0 both clamped to 1 and 20",
			page:     domain.PageParams{Page: 0, PageSize: 0},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := paginationOffset(tc.page)
			if got != tc.expected {
				t.Errorf("paginationOffset(%+v) = %d, want %d", tc.page, got, tc.expected)
			}
		})
	}
}

// ---- paginationLimit --------------------------------------------------------

func Test_paginationLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		page     domain.PageParams
		expected int
	}{
		{
			name:     "normal size 20",
			page:     domain.PageParams{PageSize: 20},
			expected: 20,
		},
		{
			name:     "size 0 defaults to 20",
			page:     domain.PageParams{PageSize: 0},
			expected: 20,
		},
		{
			name:     "size -1 defaults to 20",
			page:     domain.PageParams{PageSize: -1},
			expected: 20,
		},
		{
			name:     "size 200 capped to 100",
			page:     domain.PageParams{PageSize: 200},
			expected: 100,
		},
		{
			name:     "size 100 is at cap",
			page:     domain.PageParams{PageSize: 100},
			expected: 100,
		},
		{
			name:     "size 50 is within range",
			page:     domain.PageParams{PageSize: 50},
			expected: 50,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := paginationLimit(tc.page)
			if got != tc.expected {
				t.Errorf("paginationLimit(%+v) = %d, want %d", tc.page, got, tc.expected)
			}
		})
	}
}

// ---- buildPaginatedResult ---------------------------------------------------

func Test_buildPaginatedResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		total          int
		itemCount      int
		page           domain.PageParams
		expectedPage   int
		expectedPages  int
		expectedSize   int
		expectedTotal  int
		itemsMustExist bool
	}{
		{
			name:          "25 total page 1 size 10 gives 3 pages",
			total:         25,
			itemCount:     10,
			page:          domain.PageParams{Page: 1, PageSize: 10},
			expectedPage:  1,
			expectedPages: 3,
			expectedSize:  10,
			expectedTotal: 25,
		},
		{
			name:          "0 total page 1 size 10 gives 1 page minimum",
			total:         0,
			itemCount:     0,
			page:          domain.PageParams{Page: 1, PageSize: 10},
			expectedPage:  1,
			expectedPages: 1,
			expectedSize:  10,
			expectedTotal: 0,
		},
		{
			name:          "10 total page 1 size 10 gives exactly 1 page",
			total:         10,
			itemCount:     10,
			page:          domain.PageParams{Page: 1, PageSize: 10},
			expectedPage:  1,
			expectedPages: 1,
			expectedSize:  10,
			expectedTotal: 10,
		},
		{
			name:          "nil items become empty slice",
			total:         0,
			itemCount:     -1, // signal nil items
			page:          domain.PageParams{Page: 1, PageSize: 10},
			expectedPage:  1,
			expectedPages: 1,
			expectedSize:  10,
			expectedTotal: 0,
		},
		{
			name:          "page 0 is clamped to 1",
			total:         5,
			itemCount:     5,
			page:          domain.PageParams{Page: 0, PageSize: 10},
			expectedPage:  1,
			expectedPages: 1,
			expectedSize:  10,
			expectedTotal: 5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var items []string
			if tc.itemCount >= 0 {
				items = make([]string, tc.itemCount)
			}
			// tc.itemCount == -1 means pass nil

			result := buildPaginatedResult(items, tc.total, tc.page)

			if result.Page != tc.expectedPage {
				t.Errorf("Page = %d, want %d", result.Page, tc.expectedPage)
			}
			if result.TotalPages != tc.expectedPages {
				t.Errorf("TotalPages = %d, want %d", result.TotalPages, tc.expectedPages)
			}
			if result.PageSize != tc.expectedSize {
				t.Errorf("PageSize = %d, want %d", result.PageSize, tc.expectedSize)
			}
			if result.Total != tc.expectedTotal {
				t.Errorf("Total = %d, want %d", result.Total, tc.expectedTotal)
			}
			if result.Items == nil {
				t.Error("Items must not be nil (should be empty slice)")
			}
		})
	}
}

// ---- notFound ---------------------------------------------------------------

func Test_notFound(t *testing.T) {
	t.Parallel()

	t.Run("pgx.ErrNoRows becomes NotFoundError", func(t *testing.T) {
		t.Parallel()
		err := notFound(pgx.ErrNoRows, "rule", "abc-123")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
		if nfe.Message == "" {
			t.Error("NotFoundError.Message must not be empty")
		}
	})

	t.Run("other error is returned unchanged", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("some database error")
		err := notFound(sentinel, "rule", "abc-123")
		if !errors.Is(err, sentinel) {
			t.Errorf("expected sentinel error, got %v", err)
		}
	})

	t.Run("nil error is returned unchanged", func(t *testing.T) {
		t.Parallel()
		err := notFound(nil, "rule", "abc-123")
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
}

// ---- conflict ---------------------------------------------------------------

func Test_conflict(t *testing.T) {
	t.Parallel()

	t.Run("non-unique-violation error is returned unchanged", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("some database error")
		err := conflict(sentinel, "already exists")
		if !errors.Is(err, sentinel) {
			t.Errorf("expected sentinel error, got %v", err)
		}
	})

	t.Run("nil error is returned unchanged", func(t *testing.T) {
		t.Parallel()
		err := conflict(nil, "already exists")
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})
}
