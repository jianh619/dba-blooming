package unit_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/luckyjian/pgdba/internal/cli"
)

func TestBaselineCmd_RegisteredInHelp(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"help"})
	root.Execute()

	if !bytes.Contains(buf.Bytes(), []byte("baseline")) {
		t.Error("expected 'baseline' in help output")
	}
}

func TestBaselineDiffCmd_MissingFlags(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"baseline", "diff"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing --before and --after")
	}

	var resp map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &resp); jsonErr != nil {
		t.Fatalf("expected JSON error: %v\nraw: %s", jsonErr, buf.String())
	}
	if resp["success"] != false {
		t.Errorf("expected success=false, got %v", resp["success"])
	}
}

func TestBaselineDiffCmd_ValidFiles(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	// Create mock before/after files.
	beforeFile := filepath.Join(tmpDir, "before.json")
	afterFile := filepath.Join(tmpDir, "after.json")
	os.WriteFile(beforeFile, []byte(`{"settings": {"shared_buffers": "128MB"}}`), 0o600)
	os.WriteFile(afterFile, []byte(`{"settings": {"shared_buffers": "4GB"}}`), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"baseline", "diff", "--before", beforeFile, "--after", afterFile})

	err := root.Execute()
	if err != nil {
		t.Fatalf("baseline diff failed: %v\noutput: %s", err, buf.String())
	}

	var resp map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &resp); jsonErr != nil {
		t.Fatalf("expected JSON output: %v\nraw: %s", jsonErr, buf.String())
	}
	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
}
