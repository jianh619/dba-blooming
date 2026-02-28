package inspect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lockFileName = ".lock"

// ApplyLock represents an in-progress apply or rollback operation.
type ApplyLock struct {
	ChangeSetID string    `json:"changeset_id"`
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	Operation   string    `json:"operation"` // "apply" or "rollback"
}

// AcquireLock creates a lock file under baseDir/<fingerprint>/.lock.
// Returns an error if a lock already exists (contention).
func AcquireLock(baseDir, fingerprint string, lock ApplyLock) error {
	lockPath := filepath.Join(baseDir, fingerprint, lockFileName)

	// Check for existing lock.
	if existing, err := CheckLock(baseDir, fingerprint); err != nil {
		return fmt.Errorf("check existing lock: %w", err)
	} else if existing != nil {
		return fmt.Errorf(
			"lock contention: %s operation (changeset %s, pid %d) started at %s",
			existing.Operation,
			existing.ChangeSetID,
			existing.PID,
			existing.StartedAt.Format(time.RFC3339),
		)
	}

	data, err := json.Marshal(lock)
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}

	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return nil
}

// ReleaseLock removes the lock file. No error if lock does not exist.
func ReleaseLock(baseDir, fingerprint string) error {
	lockPath := filepath.Join(baseDir, fingerprint, lockFileName)
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove lock file: %w", err)
	}
	return nil
}

// CheckLock returns the current lock holder, or nil if no lock exists.
func CheckLock(baseDir, fingerprint string) (*ApplyLock, error) {
	lockPath := filepath.Join(baseDir, fingerprint, lockFileName)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read lock file: %w", err)
	}

	var lock ApplyLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("unmarshal lock file: %w", err)
	}
	return &lock, nil
}
