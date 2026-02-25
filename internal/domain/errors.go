package domain

// NotFoundError indicates the requested entity does not exist.
type NotFoundError struct{ Message string }

func (e *NotFoundError) Error() string { return e.Message }

// ForbiddenError indicates the caller lacks permission.
type ForbiddenError struct{ Message string }

func (e *ForbiddenError) Error() string { return e.Message }

// ConflictError indicates a conflicting state (e.g., duplicate name).
type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }

// ValidationError indicates invalid input with optional field-level details.
type ValidationError struct {
	Message string
	Details map[string]string
}

func (e *ValidationError) Error() string { return e.Message }

// ConfigError indicates a configuration problem (missing or invalid env var).
type ConfigError struct{ Message string }

func (e *ConfigError) Error() string { return e.Message }

// CompileError indicates a Starlark source compilation failure.
type CompileError struct {
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Filename string `json:"filename,omitempty"`
}

func (e *CompileError) Error() string { return e.Message }
