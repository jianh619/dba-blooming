package unit_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/luckyjian/pgdba/internal/output"
)

func TestSuccessResponse(t *testing.T) {
	r := output.Success("cluster status", map[string]string{"key": "value"})
	if !r.Success {
		t.Error("expected Success=true")
	}
	if r.Error != nil {
		t.Error("expected Error=nil")
	}
	if r.Command != "cluster status" {
		t.Errorf("expected command 'cluster status', got %q", r.Command)
	}
	if r.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestFailureResponse(t *testing.T) {
	importErr := errors.New("connection refused")
	r := output.Failure("health check", importErr)
	if r.Success {
		t.Error("expected Success=false")
	}
	if r.Error == nil {
		t.Error("expected non-nil Error")
	}
	if !strings.Contains(*r.Error, "connection refused") {
		t.Errorf("expected error message to contain 'connection refused', got %q", *r.Error)
	}
}

func TestFormatJSON(t *testing.T) {
	r := output.Success("test", map[string]int{"count": 42})
	out, err := output.FormatResponse(r, output.FormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if parsed["success"] != true {
		t.Error("JSON: expected success=true")
	}
	if _, ok := parsed["timestamp"]; !ok {
		t.Error("JSON: expected timestamp field")
	}
}

func TestFormatJSON_NilData(t *testing.T) {
	r := output.Success("test", nil)
	out, err := output.FormatResponse(r, output.FormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// data field should be omitted (omitempty)
	if strings.Contains(out, `"data"`) {
		t.Error("expected data field to be omitted when nil")
	}
}

func TestFormatYAML(t *testing.T) {
	r := output.Success("test", map[string]string{"host": "localhost"})
	out, err := output.FormatResponse(r, output.FormatYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "success: true") {
		t.Errorf("YAML output missing 'success: true', got: %s", out)
	}
}

func TestFormatTable(t *testing.T) {
	r := output.Success("test", map[string]string{"status": "healthy"})
	out, err := output.FormatResponse(r, output.FormatTable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty table output")
	}
}

func TestUnknownFormat(t *testing.T) {
	r := output.Success("test", nil)
	_, err := output.FormatResponse(r, output.Format("xml"))
	if err == nil {
		t.Error("expected error for unknown format")
	}
}
