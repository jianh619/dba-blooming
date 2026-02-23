package output

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Success constructs a successful Response with the given command name and data payload.
func Success(command string, data interface{}) Response {
	return Response{
		Success:   true,
		Timestamp: time.Now().UTC(),
		Command:   command,
		Data:      data,
	}
}

// Failure constructs a failed Response capturing the error message.
func Failure(command string, err error) Response {
	msg := err.Error()
	return Response{
		Success:   false,
		Timestamp: time.Now().UTC(),
		Command:   command,
		Error:     &msg,
	}
}

// FormatResponse serializes a Response into the requested format string.
func FormatResponse(r Response, format Format) (string, error) {
	switch format {
	case FormatJSON:
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return "", fmt.Errorf("json marshal: %w", err)
		}
		return string(b), nil
	case FormatYAML:
		b, err := yaml.Marshal(r)
		if err != nil {
			return "", fmt.Errorf("yaml marshal: %w", err)
		}
		return string(b), nil
	case FormatTable:
		return formatTable(r), nil
	default:
		return "", fmt.Errorf("unsupported format: %q", format)
	}
}

func formatTable(r Response) string {
	status := "SUCCESS"
	if !r.Success {
		status = "FAILURE"
	}
	result := fmt.Sprintf("%-12s %-20s %s\n", "STATUS", "COMMAND", "TIMESTAMP")
	result += fmt.Sprintf("%-12s %-20s %s\n", status, r.Command, r.Timestamp.Format(time.RFC3339))
	if r.Error != nil {
		result += fmt.Sprintf("ERROR: %s\n", *r.Error)
	}
	return result
}
