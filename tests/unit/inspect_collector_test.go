package unit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/luckyjian/pgdba/internal/inspect"
)

// mockDB implements inspect.DB for testing.
type mockDB struct {
	versionNum     int
	settings       []inspect.PGSetting
	sysIdentifier  string
	sysIDError     error
	resolvedAddr   string
	resolvedPort   int
	datID          uint32
	pgssAvailable  bool
	pgssRows       []inspect.PGSSRow
	activities     []inspect.PGActivity
	statBGWriter   *inspect.StatBGWriter
	statWal        *inspect.StatWal
}

func (m *mockDB) ServerVersionNum(ctx context.Context) (int, error) {
	return m.versionNum, nil
}

func (m *mockDB) PGSettings(ctx context.Context) ([]inspect.PGSetting, error) {
	return m.settings, nil
}

func (m *mockDB) SystemIdentifier(ctx context.Context) (string, error) {
	if m.sysIDError != nil {
		return "", m.sysIDError
	}
	return m.sysIdentifier, nil
}

func (m *mockDB) ResolvedAddr(ctx context.Context) (string, int, error) {
	return m.resolvedAddr, m.resolvedPort, nil
}

func (m *mockDB) CurrentDatID(ctx context.Context) (uint32, error) {
	return m.datID, nil
}

func (m *mockDB) ExtensionLoaded(ctx context.Context, name string) (bool, error) {
	if name == "pg_stat_statements" {
		return m.pgssAvailable, nil
	}
	return false, nil
}

func (m *mockDB) PGStatStatements(ctx context.Context, limit int) ([]inspect.PGSSRow, error) {
	return m.pgssRows, nil
}

func (m *mockDB) PGStatActivity(ctx context.Context) ([]inspect.PGActivity, error) {
	return m.activities, nil
}

func (m *mockDB) StatBGWriter(ctx context.Context) (*inspect.StatBGWriter, error) {
	if m.statBGWriter == nil {
		return &inspect.StatBGWriter{}, nil
	}
	return m.statBGWriter, nil
}

func (m *mockDB) StatWal(ctx context.Context) (*inspect.StatWal, error) {
	if m.statWal == nil {
		return nil, fmt.Errorf("pg_stat_wal not available")
	}
	return m.statWal, nil
}

func TestCollect_PG15_AllSections(t *testing.T) {
	db := &mockDB{
		versionNum:    150000,
		sysIdentifier: "6789012345678901234",
		resolvedAddr:  "10.0.0.1",
		resolvedPort:  5432,
		datID:         16384,
		settings: []inspect.PGSetting{
			{Name: "shared_buffers", Setting: "128MB", Context: "postmaster"},
			{Name: "work_mem", Setting: "4MB", Context: "user"},
		},
		pgssAvailable: true,
		pgssRows: []inspect.PGSSRow{
			{QueryID: 123, Query: "SELECT 1", Calls: 100, TotalTime: 50.0},
		},
		activities: []inspect.PGActivity{
			{PID: 1234, State: "active", Query: "SELECT 1"},
		},
		statBGWriter: &inspect.StatBGWriter{CheckpointsTimed: 10, CheckpointsReq: 2},
		statWal:      &inspect.StatWal{WalRecords: 1000, WalBytes: 65536},
	}

	cfg := inspect.SamplingConfig{Mode: inspect.SamplingInstant}
	snap, err := inspect.Collect(context.Background(), db, cfg, "localhost", 5432)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Identity should be Tier 0 (system_identifier available on PG 15).
	if snap.Identity.Tier != inspect.TierSystemIdentifier {
		t.Errorf("expected tier 0, got %v", snap.Identity.Tier)
	}
	if snap.Identity.SystemIdentifier != "6789012345678901234" {
		t.Errorf("unexpected system_identifier: %q", snap.Identity.SystemIdentifier)
	}
	if snap.Identity.Fingerprint == "" {
		t.Error("fingerprint should be computed")
	}

	// All standard sections should be available.
	for _, section := range []string{"pg_settings", "pg_stat_activity", "pg_stat_statements", "pg_stat_bgwriter", "pg_stat_wal"} {
		s, ok := snap.Sections[section]
		if !ok {
			t.Errorf("missing section %q", section)
			continue
		}
		if !s.Available {
			t.Errorf("section %q should be available, error: %s", section, s.Error)
		}
	}

	// pg_settings data should contain our mock settings.
	settings, ok := snap.Sections["pg_settings"].Data.([]inspect.PGSetting)
	if !ok {
		t.Fatalf("pg_settings data wrong type: %T", snap.Sections["pg_settings"].Data)
	}
	if len(settings) != 2 {
		t.Errorf("expected 2 settings, got %d", len(settings))
	}
}

func TestCollect_PG12_Degradation(t *testing.T) {
	db := &mockDB{
		versionNum:    120000,
		sysIDError:    fmt.Errorf("function pg_control_system() does not exist"),
		resolvedAddr:  "10.0.0.1",
		resolvedPort:  5432,
		datID:         16384,
		settings:      []inspect.PGSetting{{Name: "max_connections", Setting: "100", Context: "postmaster"}},
		pgssAvailable: false,
	}

	cfg := inspect.SamplingConfig{Mode: inspect.SamplingInstant}
	snap, err := inspect.Collect(context.Background(), db, cfg, "localhost", 5432)
	if err != nil {
		t.Fatalf("Collect should not fail on PG 12, got: %v", err)
	}

	// H2: Identity should fall to Tier 1 (resolved addr) since pg_control_system unavailable.
	if snap.Identity.Tier != inspect.TierResolvedAddr {
		t.Errorf("expected tier 1 for PG 12, got %v", snap.Identity.Tier)
	}

	// pg_settings should be available.
	if s := snap.Sections["pg_settings"]; !s.Available {
		t.Error("pg_settings should be available even on PG 12")
	}

	// pg_stat_statements should be unavailable (extension not loaded).
	if s := snap.Sections["pg_stat_statements"]; s.Available {
		t.Error("pg_stat_statements should be unavailable when extension not loaded")
	}

	// pg_stat_wal should be unavailable (PG 14+).
	if s := snap.Sections["pg_stat_wal"]; s.Available {
		t.Error("pg_stat_wal should be unavailable on PG 12")
	}

	// Snapshot should still succeed overall (degradation, not failure).
	if snap.CollectedAt.IsZero() {
		t.Error("collected_at should be set")
	}
}

func TestCollect_ConfigAddrFallback(t *testing.T) {
	db := &mockDB{
		versionNum:   120000,
		sysIDError:   fmt.Errorf("not available"),
		resolvedAddr: "", // empty — can't resolve
		resolvedPort: 0,
		datID:        0,
		settings:     []inspect.PGSetting{},
	}

	cfg := inspect.SamplingConfig{Mode: inspect.SamplingInstant}
	snap, err := inspect.Collect(context.Background(), db, cfg, "myhost", 5432)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Should fall to Tier 2 (config addr).
	if snap.Identity.Tier != inspect.TierConfigAddr {
		t.Errorf("expected tier 2 when resolved addr is empty, got %v", snap.Identity.Tier)
	}
	if snap.Identity.ConfigHost != "myhost" {
		t.Errorf("expected config host=myhost, got %q", snap.Identity.ConfigHost)
	}
}

func TestCollect_PrereqResults(t *testing.T) {
	db := &mockDB{
		versionNum:    150000,
		sysIdentifier: "123",
		resolvedAddr:  "10.0.0.1",
		resolvedPort:  5432,
		datID:         1,
		pgssAvailable: true,
	}

	cfg := inspect.SamplingConfig{Mode: inspect.SamplingInstant}
	snap, err := inspect.Collect(context.Background(), db, cfg, "h", 5432)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Prereqs section should exist.
	s, ok := snap.Sections["prereqs"]
	if !ok {
		t.Fatal("missing prereqs section")
	}
	if !s.Available {
		t.Error("prereqs section should be available")
	}
	prereqs, ok := s.Data.([]inspect.PrereqResult)
	if !ok {
		t.Fatalf("prereqs data wrong type: %T", s.Data)
	}
	if len(prereqs) == 0 {
		t.Error("expected at least one prereq result")
	}

	// pg_stat_statements should be in prereqs.
	found := false
	for _, pr := range prereqs {
		if pr.Name == "pg_stat_statements" {
			found = true
			if !pr.Available {
				t.Error("pg_stat_statements prereq should be available")
			}
		}
	}
	if !found {
		t.Error("expected pg_stat_statements in prereqs")
	}
}
