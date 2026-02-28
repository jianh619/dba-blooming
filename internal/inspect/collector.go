package inspect

import (
	"context"
	"time"
)

// Collect gathers a DiagSnapshot from the given database connection.
// Missing sections produce warnings (SectionResult.Error) rather than
// causing the entire collection to fail. configHost and configPort are
// the user-provided connection settings used as fallback identity.
func Collect(ctx context.Context, db DB, cfg SamplingConfig, configHost string, configPort int) (*DiagSnapshot, error) {
	snap := &DiagSnapshot{
		CollectedAt: time.Now(),
		Sections:    make(map[string]SectionResult),
	}

	// 1. Determine server version (required — if this fails, abort).
	version, err := db.ServerVersionNum(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Build identity with tier fallback.
	identity := buildIdentity(ctx, db, version, configHost, configPort)
	identity.ComputeFingerprint()
	snap.Identity = identity

	// 3. Collect prerequisite checks.
	prereqs := collectPrereqs(ctx, db, version)
	snap.Sections["prereqs"] = SectionResult{Available: true, Data: prereqs}

	// 4. Collect sections (each independently, never fatal).
	collectPGSettings(ctx, db, snap)
	collectPGStatActivity(ctx, db, snap)
	collectPGStatStatements(ctx, db, snap, prereqs)
	collectStatBGWriter(ctx, db, snap)
	collectStatWal(ctx, db, snap, version)

	return snap, nil
}

// buildIdentity determines the best identity tier available.
func buildIdentity(ctx context.Context, db DB, version int, configHost string, configPort int) ClusterIdentity {
	id := ClusterIdentity{
		ServerVersionNum: version,
		ConfigHost:       configHost,
		ConfigPort:       configPort,
	}

	// Try resolved addr + port.
	if addr, port, err := db.ResolvedAddr(ctx); err == nil && addr != "" {
		id.ResolvedAddr = addr
		id.ResolvedPort = port
	}

	// Try datid.
	if datID, err := db.CurrentDatID(ctx); err == nil {
		id.DatID = datID
	}

	// Try system identifier (PG 13+).
	if version >= 130000 {
		if sysID, err := db.SystemIdentifier(ctx); err == nil && sysID != "" {
			id.SystemIdentifier = sysID
			id.Tier = TierSystemIdentifier
			return id
		}
		// H2: fall through on error — warning logged, use lower tier.
	}

	// Tier 1: resolved address.
	if id.ResolvedAddr != "" {
		id.Tier = TierResolvedAddr
		return id
	}

	// Tier 2: config address (fallback).
	id.Tier = TierConfigAddr
	return id
}

// collectPrereqs checks all prerequisites and returns results.
func collectPrereqs(ctx context.Context, db DB, version int) []PrereqResult {
	var results []PrereqResult

	// pg_stat_statements extension.
	pgssAvail, pgssErr := db.ExtensionLoaded(ctx, "pg_stat_statements")
	pr := PrereqResult{Name: "pg_stat_statements", Available: pgssAvail, Version: version}
	if pgssErr != nil {
		pr.Error = pgssErr.Error()
	}
	results = append(results, pr)

	// pg_control_system (PG 13+).
	pgcsPR := PrereqResult{Name: "pg_control_system", Version: version}
	if version >= 130000 {
		if _, err := db.SystemIdentifier(ctx); err != nil {
			pgcsPR.Available = false
			pgcsPR.Error = err.Error()
		} else {
			pgcsPR.Available = true
		}
	} else {
		pgcsPR.Available = false
		pgcsPR.Error = "requires PostgreSQL 13+"
	}
	results = append(results, pgcsPR)

	// pg_stat_wal (PG 14+).
	walPR := PrereqResult{Name: "pg_stat_wal", Version: version}
	if version >= 140000 {
		walPR.Available = true
	} else {
		walPR.Available = false
		walPR.Error = "requires PostgreSQL 14+"
	}
	results = append(results, walPR)

	return results
}

func collectPGSettings(ctx context.Context, db DB, snap *DiagSnapshot) {
	settings, err := db.PGSettings(ctx)
	if err != nil {
		snap.Sections["pg_settings"] = SectionResult{Available: false, Error: err.Error()}
		return
	}
	snap.Sections["pg_settings"] = SectionResult{Available: true, Data: settings}
}

func collectPGStatActivity(ctx context.Context, db DB, snap *DiagSnapshot) {
	activities, err := db.PGStatActivity(ctx)
	if err != nil {
		snap.Sections["pg_stat_activity"] = SectionResult{Available: false, Error: err.Error()}
		return
	}
	snap.Sections["pg_stat_activity"] = SectionResult{Available: true, Data: activities}
}

func collectPGStatStatements(ctx context.Context, db DB, snap *DiagSnapshot, prereqs []PrereqResult) {
	// Check prereq first.
	available := false
	for _, pr := range prereqs {
		if pr.Name == "pg_stat_statements" {
			available = pr.Available
			break
		}
	}
	if !available {
		snap.Sections["pg_stat_statements"] = SectionResult{
			Available: false,
			Error:     "pg_stat_statements extension not loaded",
		}
		return
	}

	rows, err := db.PGStatStatements(ctx, 100)
	if err != nil {
		snap.Sections["pg_stat_statements"] = SectionResult{Available: false, Error: err.Error()}
		return
	}
	snap.Sections["pg_stat_statements"] = SectionResult{Available: true, Data: rows}
}

func collectStatBGWriter(ctx context.Context, db DB, snap *DiagSnapshot) {
	stats, err := db.StatBGWriter(ctx)
	if err != nil {
		snap.Sections["pg_stat_bgwriter"] = SectionResult{Available: false, Error: err.Error()}
		return
	}
	snap.Sections["pg_stat_bgwriter"] = SectionResult{Available: true, Data: stats}
}

func collectStatWal(ctx context.Context, db DB, snap *DiagSnapshot, version int) {
	if version < 140000 {
		snap.Sections["pg_stat_wal"] = SectionResult{
			Available: false,
			Error:     "requires PostgreSQL 14+",
		}
		return
	}

	stats, err := db.StatWal(ctx)
	if err != nil {
		snap.Sections["pg_stat_wal"] = SectionResult{Available: false, Error: err.Error()}
		return
	}
	snap.Sections["pg_stat_wal"] = SectionResult{Available: true, Data: stats}
}
