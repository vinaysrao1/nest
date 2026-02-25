package domain

// PaginatedResult wraps a page of results with pagination metadata.
type PaginatedResult[T any] struct {
	Items      []T `json:"items"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

// PageParams carries pagination parameters for list queries.
type PageParams struct {
	Page     int
	PageSize int
}
