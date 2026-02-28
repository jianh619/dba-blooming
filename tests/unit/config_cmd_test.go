package unit_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/luckyjian/pgdba/internal/cli"
)

func TestConfigCmd_RegisteredInHelp(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"help"})
	root.Execute()

	if !bytes.Contains(buf.Bytes(), []byte("config")) {
		t.Error("expected 'config' in help output")
	}
}

func TestConfigShowCmd_MissingCluster(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"config", "show", "--name", "nonexistent"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent cluster")
	}

	var resp map[string]interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &resp); jsonErr != nil {
		t.Fatalf("expected JSON error output: %v\nraw: %s", jsonErr, buf.String())
	}
	if resp["success"] != false {
		t.Errorf("expected success=false, got %v", resp["success"])
	}
}

func TestConfigDiffCmd_SubcommandExists(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"config", "diff", "--help"})
	root.Execute()

	if !bytes.Contains(buf.Bytes(), []byte("diff")) {
		t.Error("expected 'diff' in config diff help")
	}
}

func TestConfigTuneCmd_SubcommandExists(t *testing.T) {
	tmpDir := t.TempDir()
	regPath := filepath.Join(tmpDir, "clusters.json")
	os.WriteFile(regPath, []byte("{}"), 0o600)

	root := cli.NewRootCmdWithRegistry(regPath)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"config", "tune", "--help"})
	root.Execute()

	if !bytes.Contains(buf.Bytes(), []byte("tune")) {
		t.Error("expected 'tune' in config tune help")
	}
}
