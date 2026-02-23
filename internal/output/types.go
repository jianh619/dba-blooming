package output

import "time"

// Response is the universal API response envelope.
type Response struct {
	Success   bool        `json:"success"             yaml:"success"`
	Timestamp time.Time   `json:"timestamp"           yaml:"timestamp"`
	Command   string      `json:"command"             yaml:"command"`
	Data      interface{} `json:"data,omitempty"      yaml:"data,omitempty"`
	Error     *string     `json:"error,omitempty"     yaml:"error,omitempty"`
}

// Format represents the output serialization format.
type Format string

const (
	FormatJSON  Format = "json"
	FormatTable Format = "table"
	FormatYAML  Format = "yaml"
)
