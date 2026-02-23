package unit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luckyjian/pgdba/internal/cli"
	"github.com/luckyjian/pgdba/internal/cluster"
)

// executeClusterCmd is a helper that runs the root command with the given args
// and captures the combined output string and any error.
func executeClusterCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewRootCmd()
	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestClusterCmd_HelpAvailable(t *testing.T) {
	out, err := executeClusterCmd(t, "cluster", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "cluster") {
		t.Error("help output should contain 'cluster'")
	}
}

func TestClusterConnect_MissingPatroniURL(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "connect",
		"--name", "my-cluster",
		"--pg-host", "10.0.0.1",
	)
	if err == nil {
		t.Error("expected error when --patroni-url is missing")
	}
}

func TestClusterConnect_MissingName(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "connect",
		"--patroni-url", "http://10.0.0.1:8008",
		"--pg-host", "10.0.0.1",
	)
	if err == nil {
		t.Error("expected error when --name is missing")
	}
}

func TestClusterStatus_RequiresNameOrURL(t *testing.T) {
	_, err := executeClusterCmd(t, "cluster", "status")
	if err == nil {
		t.Error("expected error when neither --name nor --patroni-url provided")
	}
}

func TestClusterStatus_WithPatroniURL(t *testing.T) {
	// Spin up a mock Patroni server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cluster" {
			status := struct {
				Members []struct {
					Name string `json:"name"`
					Role string `json:"role"`
				} `json:"members"`
			}{
				Members: []struct {
					Name string `json:"name"`
					Role string `json:"role"`
				}{
					{Name: "pg-primary", Role: "master"},
				},
			}
			json.NewEncoder(w).Encode(status)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	out, err := executeClusterCmd(t,
		"cluster", "status",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "pg-primary") {
		t.Errorf("expected output to contain 'pg-primary', got: %s", out)
	}
}

func TestClusterDestroy_RequiresConfirm(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "destroy",
		"--name", "some-cluster",
	)
	if err == nil {
		t.Error("expected error when --confirm flag is missing")
	}
}

func TestClusterDestroy_RefusesExternal(t *testing.T) {
	// Write an external cluster entry to a temp registry.
	dir := t.TempDir()
	regPath := filepath.Join(dir, "clusters.json")
	reg := cluster.NewRegistry(regPath)
	if err := reg.Add(cluster.Entry{
		Name:      "ext-cluster",
		Source:    cluster.SourceExternal,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Use the CLI cluster destroy. The command should refuse because source=external.
	cmd := cli.NewRootCmdWithRegistry(regPath)
	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"cluster", "destroy", "--name", "ext-cluster", "--confirm"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error: cluster connect clusters must not be destroyable")
	}
	combined := buf.String()
	if !strings.Contains(combined, "ext-cluster") && !strings.Contains(err.Error(), "ext-cluster") {
		t.Errorf("error should mention cluster name, got: %v / %s", err, combined)
	}
}

func TestClusterInit_MissingPrimaryHost(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "init",
		"--name", "new-cluster",
	)
	if err == nil {
		t.Error("expected error when --primary-host is missing")
	}
}

func TestClusterConnect_PatroniUnreachable(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "connect",
		"--name", "bad-cluster",
		"--patroni-url", "http://10.255.255.1:8008",
		"--pg-host", "10.255.255.1",
		"--pg-port", "5432",
		"--provider", "baremetal",
	)
	if err == nil {
		t.Error("expected error when Patroni is unreachable")
	}
}

func TestClusterStatus_WithName_NotFound(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "status",
		"--name", "nonexistent-cluster",
	)
	if err == nil {
		t.Error("expected error when cluster not found in registry")
	}
}

func TestClusterDestroy_NotFound(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "destroy",
		"--name", "missing-cluster",
		"--confirm",
	)
	if err == nil {
		t.Error("expected error when cluster not found in registry")
	}
}
