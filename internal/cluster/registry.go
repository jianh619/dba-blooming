package cluster

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Source marks the origin of a cluster entry.
type Source string

const (
	SourceManaged  Source = "managed"  // created by pgdba cluster init; can be destroyed
	SourceExternal Source = "external" // imported by pgdba cluster connect; cannot be destroyed
)

// Entry represents a cluster record in the registry.
type Entry struct {
	Name       string            `json:"name"`
	PatroniURL string            `json:"patroni_url"`
	PGHost     string            `json:"pg_host"`
	PGPort     int               `json:"pg_port"`
	Provider   string            `json:"provider"`
	Source     Source            `json:"source"`
	CreatedAt  time.Time         `json:"created_at"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// Registry manages cluster entries persisted to a JSON file.
type Registry struct {
	path string
}

// DefaultRegistry returns the registry backed by ~/.pgdba/clusters.json.
func DefaultRegistry() (*Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".pgdba")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create pgdba dir: %w", err)
	}
	return &Registry{path: filepath.Join(dir, "clusters.json")}, nil
}

// NewRegistry creates a Registry backed by the given file path.
func NewRegistry(path string) *Registry {
	return &Registry{path: path}
}

// Add inserts or overwrites a cluster entry (keyed by Name).
func (r *Registry) Add(e Entry) error {
	entries, err := r.load()
	if err != nil {
		return err
	}
	entries[e.Name] = e
	return r.save(entries)
}

// Get returns the cluster entry with the given name.
func (r *Registry) Get(name string) (*Entry, error) {
	entries, err := r.load()
	if err != nil {
		return nil, err
	}
	e, ok := entries[name]
	if !ok {
		return nil, fmt.Errorf("cluster %q not found in registry", name)
	}
	return &e, nil
}

// List returns all cluster entries.
func (r *Registry) List() ([]Entry, error) {
	entries, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}
	return result, nil
}

// Remove deletes the cluster entry with the given name.
func (r *Registry) Remove(name string) error {
	entries, err := r.load()
	if err != nil {
		return err
	}
	if _, ok := entries[name]; !ok {
		return fmt.Errorf("cluster %q not found in registry", name)
	}
	delete(entries, name)
	return r.save(entries)
}

// load reads the registry file and returns a name-to-entry map.
// If the file does not exist an empty map is returned.
func (r *Registry) load() (map[string]Entry, error) {
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return make(map[string]Entry), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", r.path, err)
	}
	if len(data) == 0 {
		return make(map[string]Entry), nil
	}
	var entries map[string]Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse registry %s: %w", r.path, err)
	}
	return entries, nil
}

// save writes the name-to-entry map back to the registry file atomically.
func (r *Registry) save(entries map[string]Entry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(r.path, data, 0600); err != nil {
		return fmt.Errorf("write registry %s: %w", r.path, err)
	}
	return nil
}
