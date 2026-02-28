package unit_test

import (
	"testing"
	"time"

	"github.com/luckyjian/pgdba/internal/inspect"
)

func TestDiagSnapshot_SectionDegradation(t *testing.T) {
	snap := inspect.DiagSnapshot{
		Identity:    inspect.ClusterIdentity{Tier: inspect.TierConfigAddr, ConfigHost: "h", ConfigPort: 5432},
		CollectedAt: time.Now(),
		Sections: map[string]inspect.SectionResult{
			"pg_settings": {Available: true, Data: "mock-settings"},
			"pg_stat_wal": {Available: false, Error: "requires PG 14+"},
		},
	}

	// pg_settings should be available.
	if s, ok := snap.Sections["pg_settings"]; !ok || !s.Available {
		t.Error("expected pg_settings to be available")
	}

	// pg_stat_wal should be degraded with warning.
	if s, ok := snap.Sections["pg_stat_wal"]; !ok {
		t.Error("expected pg_stat_wal section to exist")
	} else {
		if s.Available {
			t.Error("expected pg_stat_wal to be unavailable")
		}
		if s.Error == "" {
			t.Error("expected non-empty error for unavailable section")
		}
	}
}

func TestChangeSet_Fields(t *testing.T) {
	cs := inspect.ChangeSet{
		ID:          "test-uuid",
		Fingerprint: "abc123",
		Parameters: []inspect.ParamChange{
			{
				Name:     "shared_buffers",
				OldValue: "128MB",
				NewValue: "2GB",
				Context:  "postmaster",
				NeedsRestart: true,
				Permission: inspect.ParamPermission{
					Allowed: true,
					MinRole: "superuser",
				},
				PatroniOverride: inspect.PatroniNotManaged,
			},
		},
		CreatedAt: time.Now(),
	}

	if cs.ID != "test-uuid" {
		t.Errorf("expected ID=test-uuid, got %q", cs.ID)
	}
	if len(cs.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(cs.Parameters))
	}

	p := cs.Parameters[0]
	if p.Name != "shared_buffers" {
		t.Errorf("expected name=shared_buffers, got %q", p.Name)
	}
	if !p.NeedsRestart {
		t.Error("expected NeedsRestart=true for postmaster param")
	}
	if p.PatroniOverride != inspect.PatroniNotManaged {
		t.Errorf("expected PatroniNotManaged, got %q", p.PatroniOverride)
	}
}

func TestPatroniOverrideLevel_Values(t *testing.T) {
	levels := []inspect.PatroniOverrideLevel{
		inspect.PatroniOverridden,
		inspect.PatroniEphemeral,
		inspect.PatroniUnknown,
		inspect.PatroniNotManaged,
	}
	seen := make(map[inspect.PatroniOverrideLevel]bool)
	for _, l := range levels {
		if seen[l] {
			t.Errorf("duplicate PatroniOverrideLevel: %q", l)
		}
		seen[l] = true
		if l == "" {
			t.Error("PatroniOverrideLevel should not be empty")
		}
	}
}

func TestConfidenceLevel_Values(t *testing.T) {
	levels := []inspect.ConfidenceLevel{
		inspect.ConfidenceHigh,
		inspect.ConfidenceMedium,
		inspect.ConfidenceLow,
	}
	seen := make(map[inspect.ConfidenceLevel]bool)
	for _, l := range levels {
		if seen[l] {
			t.Errorf("duplicate ConfidenceLevel: %q", l)
		}
		seen[l] = true
	}
}

func TestSamplingConfig_Defaults(t *testing.T) {
	// Instant mode.
	cfg := inspect.SamplingConfig{Mode: inspect.SamplingInstant}
	if cfg.Mode != inspect.SamplingInstant {
		t.Errorf("expected instant mode")
	}

	// Delta mode.
	cfg2 := inspect.SamplingConfig{
		Mode:     inspect.SamplingDelta,
		Interval: 30 * time.Second,
	}
	if cfg2.Mode != inspect.SamplingDelta {
		t.Error("expected delta mode")
	}
	if cfg2.Interval != 30*time.Second {
		t.Errorf("expected 30s interval, got %v", cfg2.Interval)
	}
}

func TestPrereqResult_Fields(t *testing.T) {
	pr := inspect.PrereqResult{
		Name:      "pg_stat_statements",
		Available: true,
		Version:   150000,
	}
	if pr.Name != "pg_stat_statements" {
		t.Errorf("unexpected name: %q", pr.Name)
	}
	if !pr.Available {
		t.Error("expected available=true")
	}

	pr2 := inspect.PrereqResult{
		Name:      "pg_control_system",
		Available: false,
		Version:   120000,
		Error:     "function does not exist",
	}
	if pr2.Available {
		t.Error("expected available=false for PG 12")
	}
	if pr2.Error == "" {
		t.Error("expected non-empty error")
	}
}
