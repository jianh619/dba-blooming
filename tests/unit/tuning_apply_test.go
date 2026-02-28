package unit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/luckyjian/pgdba/internal/inspect"
	"github.com/luckyjian/pgdba/internal/tuning"
)

// mockApplyDB implements tuning.ApplyDB for testing.
type mockApplyDB struct {
	settings      map[string]inspect.PGSetting
	appliedParams map[string]string
	reloaded      bool
	applyError    map[string]error
}

func newMockApplyDB(settings []inspect.PGSetting) *mockApplyDB {
	m := &mockApplyDB{
		settings:      make(map[string]inspect.PGSetting),
		appliedParams: make(map[string]string),
		applyError:    make(map[string]error),
	}
	for _, s := range settings {
		m.settings[s.Name] = s
	}
	return m
}

func (m *mockApplyDB) GetSetting(ctx context.Context, name string) (*inspect.PGSetting, error) {
	s, ok := m.settings[name]
	if !ok {
		return nil, fmt.Errorf("setting %q not found", name)
	}
	return &s, nil
}

func (m *mockApplyDB) AlterSystem(ctx context.Context, name, value string) error {
	if err, ok := m.applyError[name]; ok {
		return err
	}
	m.appliedParams[name] = value
	return nil
}

func (m *mockApplyDB) AlterSystemReset(ctx context.Context, name string) error {
	delete(m.appliedParams, name)
	return nil
}

func (m *mockApplyDB) ReloadConf(ctx context.Context) error {
	m.reloaded = true
	return nil
}

func TestDryRun_AllPass(t *testing.T) {
	db := newMockApplyDB([]inspect.PGSetting{
		{Name: "work_mem", Setting: "4MB", Context: "user"},
		{Name: "shared_buffers", Setting: "128MB", Context: "postmaster"},
	})

	cs := inspect.ChangeSet{
		ID:          "test-cs-1",
		Fingerprint: "fp1",
		Parameters: []inspect.ParamChange{
			{
				Name: "work_mem", OldValue: "4MB", NewValue: "64MB",
				Context: "user", NeedsRestart: false,
				Permission: inspect.ParamPermission{Allowed: true},
			},
		},
	}

	result, err := tuning.DryRun(context.Background(), db, cs)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if !result.OK {
		t.Errorf("expected OK=true, got errors: %v, warnings: %v", result.Errors, result.Warnings)
	}
}

func TestDryRun_PermissionDenied(t *testing.T) {
	db := newMockApplyDB([]inspect.PGSetting{
		{Name: "shared_buffers", Setting: "128MB", Context: "postmaster"},
	})

	cs := inspect.ChangeSet{
		ID:          "test-cs-2",
		Fingerprint: "fp1",
		Parameters: []inspect.ParamChange{
			{
				Name: "shared_buffers", OldValue: "128MB", NewValue: "4GB",
				Context: "postmaster", NeedsRestart: true,
				Permission: inspect.ParamPermission{Allowed: false, Reason: "requires superuser"},
			},
		},
	}

	result, err := tuning.DryRun(context.Background(), db, cs)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for permission denied")
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors for permission denied")
	}
}

func TestDryRun_PatroniOverrideWarning(t *testing.T) {
	db := newMockApplyDB([]inspect.PGSetting{
		{Name: "work_mem", Setting: "4MB", Context: "user"},
	})

	cs := inspect.ChangeSet{
		Parameters: []inspect.ParamChange{
			{
				Name: "work_mem", OldValue: "4MB", NewValue: "64MB",
				Context: "user", NeedsRestart: false,
				Permission:      inspect.ParamPermission{Allowed: true},
				PatroniOverride: inspect.PatroniOverridden,
			},
		},
	}

	result, err := tuning.DryRun(context.Background(), db, cs)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for Patroni override")
	}
}

func TestDryRun_RestartRequired(t *testing.T) {
	db := newMockApplyDB([]inspect.PGSetting{
		{Name: "shared_buffers", Setting: "128MB", Context: "postmaster"},
	})

	cs := inspect.ChangeSet{
		Parameters: []inspect.ParamChange{
			{
				Name: "shared_buffers", OldValue: "128MB", NewValue: "4GB",
				Context: "postmaster", NeedsRestart: true,
				Permission: inspect.ParamPermission{Allowed: true},
			},
		},
	}

	result, err := tuning.DryRun(context.Background(), db, cs)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	// Should warn about restart requirement.
	hasRestartWarning := false
	for _, w := range result.Warnings {
		if contains(w, "restart") {
			hasRestartWarning = true
			break
		}
	}
	if !hasRestartWarning {
		t.Error("expected restart warning for postmaster context param")
	}
}

func TestApply_Success(t *testing.T) {
	db := newMockApplyDB([]inspect.PGSetting{
		{Name: "work_mem", Setting: "4MB", Context: "user"},
	})

	cs := inspect.ChangeSet{
		ID:          "cs-apply-1",
		Fingerprint: "fp1",
		Parameters: []inspect.ParamChange{
			{
				Name: "work_mem", OldValue: "4MB", NewValue: "64MB",
				Context: "user", NeedsRestart: false,
				Permission: inspect.ParamPermission{Allowed: true},
			},
		},
	}

	err := tuning.Apply(context.Background(), db, &cs)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Check ALTER SYSTEM was called.
	if db.appliedParams["work_mem"] != "64MB" {
		t.Errorf("expected work_mem=64MB applied, got %q", db.appliedParams["work_mem"])
	}
	// Reload should have been called (sighup param).
	if !db.reloaded {
		t.Error("expected pg_reload_conf() to be called")
	}
	// AppliedAt should be set.
	if cs.AppliedAt == nil {
		t.Error("expected AppliedAt to be set")
	}
}

func TestApply_AlterSystemError(t *testing.T) {
	db := newMockApplyDB([]inspect.PGSetting{
		{Name: "work_mem", Setting: "4MB", Context: "user"},
	})
	db.applyError["work_mem"] = fmt.Errorf("permission denied")

	cs := inspect.ChangeSet{
		Parameters: []inspect.ParamChange{
			{
				Name: "work_mem", OldValue: "4MB", NewValue: "64MB",
				Context: "user", Permission: inspect.ParamPermission{Allowed: true},
			},
		},
	}

	err := tuning.Apply(context.Background(), db, &cs)
	if err == nil {
		t.Fatal("expected error for ALTER SYSTEM failure")
	}
}

func TestRollback_Success(t *testing.T) {
	db := newMockApplyDB([]inspect.PGSetting{
		{Name: "work_mem", Setting: "64MB", Context: "user"},
	})

	cs := inspect.ChangeSet{
		ID: "cs-rollback-1",
		Parameters: []inspect.ParamChange{
			{
				Name: "work_mem", OldValue: "4MB", NewValue: "64MB",
				Context: "user", Permission: inspect.ParamPermission{Allowed: true},
			},
		},
	}

	err := tuning.Rollback(context.Background(), db, &cs)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Should reset to old value.
	if db.appliedParams["work_mem"] != "4MB" {
		t.Errorf("expected work_mem=4MB after rollback, got %q", db.appliedParams["work_mem"])
	}
	if !db.reloaded {
		t.Error("expected reload after rollback")
	}
	if cs.RolledBackAt == nil {
		t.Error("expected RolledBackAt to be set")
	}
}
