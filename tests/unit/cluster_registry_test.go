package unit_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/luckyjian/pgdba/internal/cluster"
)

func newTestRegistry(t *testing.T) *cluster.Registry {
	t.Helper()
	dir := t.TempDir()
	return cluster.NewRegistry(filepath.Join(dir, "clusters.json"))
}

func TestRegistry_AddAndGet(t *testing.T) {
	reg := newTestRegistry(t)
	entry := cluster.Entry{
		Name:       "test-cluster",
		PatroniURL: "http://10.0.0.1:8008",
		PGHost:     "10.0.0.1",
		PGPort:     5432,
		Provider:   "docker",
		Source:     cluster.SourceManaged,
		CreatedAt:  time.Now(),
	}
	if err := reg.Add(entry); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	got, err := reg.Get("test-cluster")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.PatroniURL != "http://10.0.0.1:8008" {
		t.Errorf("expected PatroniURL %q, got %q", "http://10.0.0.1:8008", got.PatroniURL)
	}
	if got.Source != cluster.SourceManaged {
		t.Errorf("expected source 'managed', got %q", got.Source)
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	reg := newTestRegistry(t)
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent cluster")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := newTestRegistry(t)
	for _, name := range []string{"cluster-a", "cluster-b"} {
		if err := reg.Add(cluster.Entry{Name: name, Source: cluster.SourceManaged, CreatedAt: time.Now()}); err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}
	entries, err := reg.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestRegistry_Remove(t *testing.T) {
	reg := newTestRegistry(t)
	if err := reg.Add(cluster.Entry{Name: "to-remove", Source: cluster.SourceManaged, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := reg.Remove("to-remove"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	_, err := reg.Get("to-remove")
	if err == nil {
		t.Error("expected error after remove")
	}
}

func TestRegistry_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.json")

	reg1 := cluster.NewRegistry(path)
	if err := reg1.Add(cluster.Entry{Name: "persisted", Source: cluster.SourceExternal, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Create a new instance using the same path to verify persistence.
	reg2 := cluster.NewRegistry(path)
	got, err := reg2.Get("persisted")
	if err != nil {
		t.Fatalf("expected persisted entry, got error: %v", err)
	}
	if got.Source != cluster.SourceExternal {
		t.Errorf("expected source 'external', got %q", got.Source)
	}
}

func TestRegistry_Add_Overwrite(t *testing.T) {
	reg := newTestRegistry(t)
	if err := reg.Add(cluster.Entry{Name: "c", PatroniURL: "http://old:8008", Source: cluster.SourceManaged, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := reg.Add(cluster.Entry{Name: "c", PatroniURL: "http://new:8008", Source: cluster.SourceManaged, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Add overwrite failed: %v", err)
	}
	got, err := reg.Get("c")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.PatroniURL != "http://new:8008" {
		t.Errorf("expected overwritten URL 'http://new:8008', got %q", got.PatroniURL)
	}
}

func TestRegistry_EmptyFile(t *testing.T) {
	reg := newTestRegistry(t)
	entries, err := reg.List()
	if err != nil {
		t.Fatalf("List on empty registry failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestRegistry_Remove_NonExistent(t *testing.T) {
	reg := newTestRegistry(t)
	err := reg.Remove("does-not-exist")
	if err == nil {
		t.Error("expected error when removing nonexistent cluster")
	}
}

func TestRegistry_Labels(t *testing.T) {
	reg := newTestRegistry(t)
	labels := map[string]string{"env": "prod", "region": "us-east"}
	if err := reg.Add(cluster.Entry{
		Name:      "labeled-cluster",
		Source:    cluster.SourceManaged,
		CreatedAt: time.Now(),
		Labels:    labels,
	}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	got, err := reg.Get("labeled-cluster")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Labels["env"] != "prod" {
		t.Errorf("expected label env=prod, got %q", got.Labels["env"])
	}
	if got.Labels["region"] != "us-east" {
		t.Errorf("expected label region=us-east, got %q", got.Labels["region"])
	}
}

func TestRegistry_ListEmpty_ReturnsEmptySlice(t *testing.T) {
	reg := newTestRegistry(t)
	entries, err := reg.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries == nil {
		t.Error("List should return an empty slice, not nil")
	}
}

func TestEntry_SourceConstants(t *testing.T) {
	if cluster.SourceManaged != "managed" {
		t.Errorf("expected SourceManaged='managed', got %q", cluster.SourceManaged)
	}
	if cluster.SourceExternal != "external" {
		t.Errorf("expected SourceExternal='external', got %q", cluster.SourceExternal)
	}
}
