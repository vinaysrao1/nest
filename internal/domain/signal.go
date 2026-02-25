package domain

// SignalInputType identifies the type of input a signal adapter accepts.
type SignalInputType string

// SignalInput is the input to a signal adapter.
type SignalInput struct {
	Type  SignalInputType
	Value string
}

// SignalOutput is the result of a signal adapter evaluation.
type SignalOutput struct {
	Score    float64        `json:"score"`
	Label    string         `json:"label"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
