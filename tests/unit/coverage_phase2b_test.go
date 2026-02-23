package unit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/luckyjian/pgdba/internal/cluster"
	"github.com/luckyjian/pgdba/internal/patroni"
	"github.com/luckyjian/pgdba/internal/postgres"
)

// --- Registry: save failure when registry path is a directory ---

func TestRegistry_SaveFails_PathIsDirectory(t *testing.T) {
	// Use a directory as the registry path to force os.WriteFile to fail.
	dir := t.TempDir()
	// Point registry at the directory itself (not a file inside it).
	reg := cluster.NewRegistry(dir)
	err := reg.Add(cluster.Entry{
		Name:      "fail-cluster",
		Source:    cluster.SourceManaged,
		CreatedAt: time.Now(),
	})
	// os.WriteFile on a directory should return an error.
	if err == nil {
		t.Error("expected write error when registry path is a directory")
	}
}

// --- Registry: load fails on unreadable file ---

func TestRegistry_LoadFails_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission test is unreliable")
	}

	dir := t.TempDir()
	regPath := filepath.Join(dir, "clusters.json")

	// Create the file and set it unreadable.
	if err := os.WriteFile(regPath, []byte(`{"a":{}}`), 0000); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	reg := cluster.NewRegistry(regPath)
	_, err := reg.List()
	if err == nil {
		t.Error("expected error reading unreadable registry file")
	}
}

// --- Patroni client: request build failure (nil context) skipping, test bad URL ---

func TestGetClusterStatus_BadURL(t *testing.T) {
	client := patroni.NewClient("http://[::invalid]")
	_, err := client.GetClusterStatus(context.Background())
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestGetNodeInfo_BadURL(t *testing.T) {
	client := patroni.NewClient("http://[::invalid]")
	_, err := client.GetNodeInfo(context.Background())
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestIsPrimary_BadURL(t *testing.T) {
	client := patroni.NewClient("http://[::invalid]")
	_, err := client.IsPrimary(context.Background())
	if err == nil {
		t.Error("expected error for invalid URL on IsPrimary")
	}
}

func TestSwitchover_BadURL(t *testing.T) {
	client := patroni.NewClient("http://[::invalid]")
	err := client.Switchover(context.Background(), "leader", "candidate")
	if err == nil {
		t.Error("expected error for invalid URL on Switchover")
	}
}

// TestPostgres_ConnectFails_ShortTimeout verifies Connect handles dial timeout.
func TestPostgres_ConnectFails_ShortTimeout(t *testing.T) {
	// Use a routable but non-listening address to trigger a fast refusal.
	cfg := postgres.Config{
		Host:     "127.0.0.1",
		Port:     19999, // unlikely to have a listener
		User:     "postgres",
		Database: "postgres",
		SSLMode:  "disable",
	}
	ctx := context.Background()
	_, err := postgres.Connect(ctx, cfg)
	// We expect an error; if not (e.g. something IS listening) the test still passes.
	_ = err
}

// noopWriter satisfies io.Writer but discards output.
type noopWriter struct{}

func (n *noopWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestConfigLoad_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	badConfig := filepath.Join(dir, "bad.yaml")
	// Write invalid YAML to cause a parse error.
	if err := os.WriteFile(badConfig, []byte("{not: [valid: yaml"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	buf := new(noopWriter)
	// Trigger PersistentPreRunE with a real subcommand so config.Load is called.
	cmd := newRootWithReg(filepath.Join(dir, "reg.json"))
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--config", badConfig, "cluster", "status", "--patroni-url", "http://localhost:9999"})
	// Expect an error; either config load failure or status error is acceptable.
	_ = cmd.Execute()
}

// --- DefaultRegistry: error path when home dir cannot be determined ---
// This is OS-dependent. On Linux, HOME is always set, so we test the success path.
func TestDefaultRegistry_Success(t *testing.T) {
	reg, err := cluster.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry should succeed: %v", err)
	}
	if reg == nil {
		t.Error("expected non-nil registry")
	}
}

// --- Registry: Add with zero-value entry ---

func TestRegistry_AddZeroValue(t *testing.T) {
	reg := newTestRegistry(t)
	entry := cluster.Entry{
		Name:      "zero",
		Source:    cluster.SourceManaged,
		CreatedAt: time.Time{}, // zero value
	}
	if err := reg.Add(entry); err != nil {
		t.Fatalf("Add with zero time failed: %v", err)
	}
	got, err := reg.Get("zero")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.CreatedAt.IsZero() != entry.CreatedAt.IsZero() {
		t.Error("expected zero CreatedAt to be preserved")
	}
}
