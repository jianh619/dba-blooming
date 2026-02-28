package unit_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/luckyjian/pgdba/internal/cli"
	"github.com/luckyjian/pgdba/internal/cluster"
)

// TestInspectCmd_MissingCluster verifies that `inspect` returns an error when
// the cluster name is not found in the registry.
func TestInspectCmd_MissingCluster(t *testing.T) {
	// Create a temporary registry with no clusters.
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"inspect", "--name", "nonexistent"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent cluster")
	}

	// Should produce valid JSON error output.
	output := buf.String()
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(output), &resp); jsonErr != nil {
		t.Logf("output: %s", output)
		t.Fatalf("expected JSON error output, got: %v", jsonErr)
	}
	if resp["success"] != false {
		t.Errorf("expected success=false, got %v", resp["success"])
	}
}

// TestInspectCmd_RegisteredInHelp verifies inspect appears in help output.
func TestInspectCmd_RegisteredInHelp(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	reg := cluster.NewRegistry(regPath)
	// Write empty registry.
	os.WriteFile(regPath, []byte("{}"), 0o600)
	_ = reg

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"help"})
	root.Execute()

	help := buf.String()
	if !contains(help, "inspect") {
		t.Errorf("expected 'inspect' in help output:\n%s", help)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && bytes.Contains([]byte(s), []byte(substr))
}
