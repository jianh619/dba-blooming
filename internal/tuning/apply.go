package tuning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/luckyjian/pgdba/internal/inspect"
)

// ApplyDB abstracts the database operations needed for config apply/rollback.
type ApplyDB interface {
	GetSetting(ctx context.Context, name string) (*inspect.PGSetting, error)
	AlterSystem(ctx context.Context, name, value string) error
	AlterSystemReset(ctx context.Context, name string) error
	ReloadConf(ctx context.Context) error
}

// DryRun validates a ChangeSet without applying any changes.
// Returns a DryRunResult indicating whether the apply would succeed.
func DryRun(ctx context.Context, db ApplyDB, cs inspect.ChangeSet) (*inspect.DryRunResult, error) {
	result := &inspect.DryRunResult{OK: true}

	for _, p := range cs.Parameters {
		// Check permission.
		if !p.Permission.Allowed {
			result.OK = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: permission denied (%s)", p.Name, p.Permission.Reason))
			continue
		}

		// Check Patroni override.
		if p.PatroniOverride == inspect.PatroniOverridden {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: parameter is overridden by Patroni DCS — ALTER SYSTEM change may be reverted on next Patroni restart", p.Name))
		}

		// Check restart requirement.
		if p.NeedsRestart || p.Context == "postmaster" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: requires PostgreSQL restart to take effect (context=%s)", p.Name, p.Context))
		}

		// Verify setting exists.
		if _, err := db.GetSetting(ctx, p.Name); err != nil {
			result.OK = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("%s: setting not found — %v", p.Name, err))
		}
	}

	return result, nil
}

// Apply executes the ChangeSet against the database.
// For each parameter: ALTER SYSTEM SET, then pg_reload_conf() for sighup params.
func Apply(ctx context.Context, db ApplyDB, cs *inspect.ChangeSet) error {
	needsReload := false

	for _, p := range cs.Parameters {
		if err := db.AlterSystem(ctx, p.Name, p.NewValue); err != nil {
			return fmt.Errorf("ALTER SYSTEM SET %s = '%s': %w", p.Name, p.NewValue, err)
		}

		if !p.NeedsRestart && p.Context != "postmaster" {
			needsReload = true
		}
	}

	if needsReload {
		if err := db.ReloadConf(ctx); err != nil {
			return fmt.Errorf("pg_reload_conf(): %w", err)
		}
	}

	now := time.Now()
	cs.AppliedAt = &now
	return nil
}

// Rollback reverts a ChangeSet by setting each parameter back to its old value.
func Rollback(ctx context.Context, db ApplyDB, cs *inspect.ChangeSet) error {
	needsReload := false

	for _, p := range cs.Parameters {
		oldVal := strings.TrimSpace(p.OldValue)
		if oldVal == "" {
			// Reset to default if no old value recorded.
			if err := db.AlterSystemReset(ctx, p.Name); err != nil {
				return fmt.Errorf("ALTER SYSTEM RESET %s: %w", p.Name, err)
			}
		} else {
			if err := db.AlterSystem(ctx, p.Name, oldVal); err != nil {
				return fmt.Errorf("ALTER SYSTEM SET %s = '%s' (rollback): %w", p.Name, oldVal, err)
			}
		}

		if !p.NeedsRestart && p.Context != "postmaster" {
			needsReload = true
		}
	}

	if needsReload {
		if err := db.ReloadConf(ctx); err != nil {
			return fmt.Errorf("pg_reload_conf() during rollback: %w", err)
		}
	}

	now := time.Now()
	cs.RolledBackAt = &now
	return nil
}
