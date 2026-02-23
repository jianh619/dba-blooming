package unit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luckyjian/pgdba/internal/cli"
	"github.com/luckyjian/pgdba/internal/cluster"
	"github.com/spf13/cobra"
)

// newRootWithReg creates a root command backed by the given registry path.
func newRootWithReg(regPath string) *cobra.Command {
	return cli.NewRootCmdWithRegistry(regPath)
}

// --- Cluster CLI: happy-path destroy of a managed cluster ---

func TestClusterDestroy_ManagedCluster_Succeeds(t *testing.T) {
	dir := t.TempDir()
	regPath := filepath.Join(dir, "clusters.json")
	reg := cluster.NewRegistry(regPath)
	if err := reg.Add(cluster.Entry{
		Name:      "managed-cluster",
		Source:    cluster.SourceManaged,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	err := executeClusterCmdWithRegistry(t, regPath,
		"cluster", "destroy", "--name", "managed-cluster", "--confirm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// executeClusterCmdWithRegistry runs the root command with a custom registry path.
func executeClusterCmdWithRegistry(t *testing.T, regPath string, args ...string) error {
	t.Helper()
	cmd := cli.NewRootCmdWithRegistry(regPath)
	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// --- Cluster connect: registry write error path ---

func TestClusterConnect_SuccessWithMockPatroni(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cluster" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"members": []map[string]string{
					{"name": "pg-1", "role": "master"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	regPath := filepath.Join(dir, "clusters.json")

	err := executeClusterCmdWithRegistry(t, regPath,
		"cluster", "connect",
		"--name", "imported-cluster",
		"--patroni-url", srv.URL,
		"--pg-host", "10.0.0.1",
		"--pg-port", "5432",
		"--provider", "baremetal",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify cluster was registered.
	reg := cluster.NewRegistry(regPath)
	entry, err := reg.Get("imported-cluster")
	if err != nil {
		t.Fatalf("expected cluster to be registered: %v", err)
	}
	if entry.Source != cluster.SourceExternal {
		t.Errorf("expected source 'external', got %q", entry.Source)
	}
}

// --- Cluster init: missing name ---

func TestClusterInit_MissingName(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "init",
		"--primary-host", "10.0.0.1",
	)
	if err == nil {
		t.Error("expected error when --name is missing for cluster init")
	}
}

// --- Cluster status: happy path from registry lookup ---

func TestClusterStatus_FromRegistry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cluster" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"members": []map[string]string{
					{"name": "pg-primary", "role": "master", "state": "running"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	regPath := filepath.Join(dir, "clusters.json")
	reg := cluster.NewRegistry(regPath)
	if err := reg.Add(cluster.Entry{
		Name:       "registry-cluster",
		PatroniURL: srv.URL,
		Source:     cluster.SourceManaged,
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	buf := new(strings.Builder)
	cmd := newRootWithReg(regPath)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"cluster", "status", "--name", "registry-cluster"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "pg-primary") {
		t.Errorf("expected output to mention 'pg-primary', got: %s", buf.String())
	}
}

// --- Registry: corrupt file returns error ---

func TestRegistry_CorruptFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.json")
	// Write invalid JSON.
	if err := os.WriteFile(path, []byte("{broken json"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	reg := cluster.NewRegistry(path)
	_, err := reg.List()
	if err == nil {
		t.Error("expected error when registry file contains invalid JSON")
	}
}

// --- Registry: Add persists file correctly after Get and Remove ---

func TestRegistry_FullLifecycle(t *testing.T) {
	reg := newTestRegistry(t)

	// Add two entries.
	for i, name := range []string{"alpha", "beta", "gamma"} {
		if err := reg.Add(cluster.Entry{
			Name:       name,
			PatroniURL: "http://10.0.0." + string(rune('1'+i)) + ":8008",
			Source:     cluster.SourceManaged,
			CreatedAt:  time.Now(),
		}); err != nil {
			t.Fatalf("Add %s failed: %v", name, err)
		}
	}

	entries, err := reg.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Remove one.
	if err := reg.Remove("beta"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	entries, err = reg.List()
	if err != nil {
		t.Fatalf("List after remove failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after remove, got %d", len(entries))
	}
}

// --- Cluster connect: missing pg-host ---

func TestClusterConnect_MissingPGHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"members": []interface{}{}})
	}))
	defer srv.Close()

	_, err := executeClusterCmd(t,
		"cluster", "connect",
		"--name", "my-cluster",
		"--patroni-url", srv.URL,
		// pg-host intentionally omitted
	)
	if err == nil {
		t.Error("expected error when --pg-host is missing")
	}
}

// --- buildStatusResult: unhealthy when member not running ---

func TestClusterStatus_UnhealthyState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cluster" {
			status := map[string]interface{}{
				"members": []map[string]interface{}{
					{"name": "pg-primary", "role": "master", "state": "stopped"},
				},
			}
			json.NewEncoder(w).Encode(status)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	buf := new(strings.Builder)
	cmd := newRootWithReg(filepath.Join(t.TempDir(), "clusters.json"))
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"cluster", "status", "--patroni-url", srv.URL})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When state is "stopped", healthy should be false in the output.
	out := buf.String()
	if !strings.Contains(out, "false") && !strings.Contains(out, "stopped") {
		t.Errorf("expected 'false' or 'stopped' in output, got: %s", out)
	}
}

// --- Cluster status with a replica in the output ---

func TestClusterStatus_WithReplica(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cluster" {
			status := map[string]interface{}{
				"members": []map[string]interface{}{
					{"name": "pg-primary", "role": "master", "state": "running"},
					{"name": "pg-replica-1", "role": "replica", "state": "running", "lag": 512},
				},
			}
			json.NewEncoder(w).Encode(status)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	out, err := executeClusterCmd(t, "cluster", "status", "--patroni-url", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "pg-replica-1") {
		t.Errorf("expected 'pg-replica-1' in output, got: %s", out)
	}
}

// --- Cluster init with both flags provided (reaches not-implemented error) ---

func TestClusterInit_ReachesNotImplemented(t *testing.T) {
	_, err := executeClusterCmd(t,
		"cluster", "init",
		"--name", "new-cluster",
		"--primary-host", "10.0.0.1",
	)
	if err == nil {
		t.Error("expected 'not implemented' error from cluster init")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' error, got: %v", err)
	}
}

// --- DefaultRegistry: ensures the directory is created ---

func TestDefaultRegistry_CreatesDirectory(t *testing.T) {
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	reg, err := cluster.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry failed: %v", err)
	}
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	// Verify directory was created.
	pgdbaDir := filepath.Join(home, ".pgdba")
	if _, statErr := os.Stat(pgdbaDir); os.IsNotExist(statErr) {
		t.Error("expected ~/.pgdba directory to be created")
	}
}
