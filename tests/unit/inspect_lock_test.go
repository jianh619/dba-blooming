package unit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/luckyjian/pgdba/internal/inspect"
)

func TestAcquireAndReleaseLock(t *testing.T) {
	dir := t.TempDir()
	fp := "test-fingerprint-abc123"
	snapDir := filepath.Join(dir, fp)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatal(err)
	}

	lock := inspect.ApplyLock{
		ChangeSetID: "cs-001",
		PID:         12345,
		StartedAt:   time.Now(),
		Operation:   "apply",
	}

	// Acquire should succeed on empty dir.
	err := inspect.AcquireLock(dir, fp, lock)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	// CheckLock should return the lock.
	got, err := inspect.CheckLock(dir, fp)
	if err != nil {
		t.Fatalf("CheckLock failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil lock")
	}
	if got.ChangeSetID != "cs-001" {
		t.Errorf("expected changeset_id=cs-001, got %q", got.ChangeSetID)
	}
	if got.Operation != "apply" {
		t.Errorf("expected operation=apply, got %q", got.Operation)
	}

	// Release should succeed.
	err = inspect.ReleaseLock(dir, fp)
	if err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}

	// After release, CheckLock should return nil.
	got, err = inspect.CheckLock(dir, fp)
	if err != nil {
		t.Fatalf("CheckLock after release failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil lock after release, got %+v", got)
	}
}

func TestAcquireLock_Contention(t *testing.T) {
	dir := t.TempDir()
	fp := "contention-fp"
	snapDir := filepath.Join(dir, fp)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatal(err)
	}

	lock1 := inspect.ApplyLock{
		ChangeSetID: "cs-001",
		PID:         100,
		StartedAt:   time.Now(),
		Operation:   "apply",
	}
	lock2 := inspect.ApplyLock{
		ChangeSetID: "cs-002",
		PID:         200,
		StartedAt:   time.Now(),
		Operation:   "rollback",
	}

	// First acquire succeeds.
	if err := inspect.AcquireLock(dir, fp, lock1); err != nil {
		t.Fatalf("first AcquireLock failed: %v", err)
	}

	// Second acquire should fail with contention error.
	err := inspect.AcquireLock(dir, fp, lock2)
	if err == nil {
		t.Fatal("expected contention error, got nil")
	}
	t.Logf("contention error (expected): %v", err)
}

func TestReleaseLock_NoLock(t *testing.T) {
	dir := t.TempDir()
	fp := "no-lock-fp"
	snapDir := filepath.Join(dir, fp)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Releasing when no lock exists should not error.
	err := inspect.ReleaseLock(dir, fp)
	if err != nil {
		t.Errorf("ReleaseLock on empty dir should not error, got: %v", err)
	}
}

func TestCheckLock_NoDir(t *testing.T) {
	dir := t.TempDir()
	fp := "nonexistent"

	got, err := inspect.CheckLock(dir, fp)
	if err != nil {
		t.Fatalf("CheckLock on nonexistent dir should not error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent dir, got %+v", got)
	}
}
