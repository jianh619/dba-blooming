# Phase 4: Config Tuning + Query Analysis — Detailed Implementation Plan

## Overview

Phase 4 delivers database inspection, configuration tuning, and query analysis capabilities.
It requires only a PG connection (no Patroni dependency for core features), making it usable
in both Scenario 4 (managed Patroni) and Scenario 5 (standalone PG).

---

## Architecture: Core Types

### ClusterIdentity — Stable Fingerprint (H1, H2, M1)

```go
// internal/inspect/identity.go

type IdentityTier int
const (
    TierSystemIdentifier IdentityTier = iota // pg_control_system() — PG 13+
    TierResolvedAddr                          // inet_server_addr():inet_server_port():datid
    TierConfigAddr                            // config_host:config_port (fallback)
)

type ClusterIdentity struct {
    Tier             IdentityTier
    SystemIdentifier string   // from pg_control_system(); empty if PG <13 or unavailable
    ResolvedAddr     string   // inet_server_addr() — IP only
    ResolvedPort     int      // inet_server_port() — separate field (H1 fix)
    ConfigHost       string   // user-provided host (display/audit only)
    ConfigPort       int      // user-provided port (display/audit only)
    DatID            uint32   // pg_backend_pid() → pg_stat_activity.datid
    ServerVersionNum int      // from server_version_num setting (H2: drives feature availability)
    Fingerprint      string   // computed: SHA256(best-tier fields)
}

// ComputeFingerprint uses the highest available tier:
//   Tier 0: SHA256(system_identifier)
//   Tier 1: SHA256(resolved_addr + ":" + resolved_port + ":" + datid)  ← H1 fix
//   Tier 2: SHA256(config_host + ":" + config_port)
func (id *ClusterIdentity) ComputeFingerprint() string
```

### DiagSnapshot vs ChangeSet Separation (#2)

```go
// DiagSnapshot — read-only, degradable (missing sections = warning, not error)
type DiagSnapshot struct {
    Identity    ClusterIdentity
    CollectedAt time.Time
    Sections    map[string]SectionResult  // "pg_settings", "pg_stat_statements", etc.
}

type SectionResult struct {
    Available bool
    Error     string      // non-fatal: logged as warning
    Data      interface{}
}

// ChangeSet — must be complete, rollback-capable, never degradable
type ChangeSet struct {
    ID          string            // UUID
    Fingerprint string            // must match target cluster
    Parameters  []ParamChange
    CreatedAt   time.Time
    AppliedAt   *time.Time
    RolledBackAt *time.Time
    PreSnapshot  *DiagSnapshot    // state before apply
    DryRunResult *DryRunResult
}

type ParamChange struct {
    Name        string
    OldValue    string
    NewValue    string
    Context     string  // "sighup", "postmaster", "user", "backend" (#4)
    NeedsRestart bool
    Permission  ParamPermission  // M2: per-parameter permission check
    PatroniOverride PatroniOverrideLevel  // M3: Patroni DCS conflict detection
}

type ParamPermission struct {
    Allowed   bool
    Reason    string   // e.g. "requires superuser", "requires pg_write_all_settings"
    MinRole   string   // "superuser", "pg_write_all_settings", etc.
}

type PatroniOverrideLevel string
const (
    PatroniOverridden       PatroniOverrideLevel = "overridden"        // exists in DCS postgresql.parameters
    PatroniEphemeral        PatroniOverrideLevel = "not_set_but_ephemeral" // Patroni manages but not set
    PatroniUnknown          PatroniOverrideLevel = "unknown"           // no Patroni or /config unreachable
    PatroniNotManaged       PatroniOverrideLevel = "not_managed"       // standalone PG
)
```

### Prerequisite System (#1: single source of truth in inspect/)

```go
// internal/inspect/prereq.go — single source of truth for all prerequisites

type PrereqResult struct {
    Name      string // "pg_stat_statements", "pg_control_system", etc.
    Available bool
    Version   int    // server_version_num when relevant
    Error     string
}

// CheckPrereqs runs all prerequisite checks against the connection.
// Each check is independent and non-fatal — callers decide what to do.
func CheckPrereqs(ctx context.Context, db *pgx.Conn) []PrereqResult
```

### Remote Conservative Profile (#5)

```go
type ConfidenceLevel string
const (
    ConfidenceHigh   ConfidenceLevel = "high"   // safe to apply without deep review
    ConfidenceMedium ConfidenceLevel = "medium"  // review recommended
    ConfidenceLow    ConfidenceLevel = "low"     // expert review required
)

type Recommendation struct {
    Parameter   string
    Current     string
    Recommended string
    Confidence  ConfidenceLevel
    Rationale   string          // human-readable explanation (#C)
    Source      string          // "pgtune", "pgdba-heuristic", etc.
}
```

### Baseline Sampling (M4)

```go
type SamplingMode string
const (
    SamplingInstant SamplingMode = "instant"  // single-point snapshot
    SamplingDelta   SamplingMode = "delta"    // two-point with interval
)

type SamplingConfig struct {
    Mode     SamplingMode
    Interval time.Duration   // only for delta mode (default 30s)
}

// For cumulative stats (pg_stat_bgwriter, pg_stat_wal PG14+), delta mode
// captures stats_reset timestamp and computes per-second rates.
type BaselineSection struct {
    Name       string
    Mode       SamplingMode
    StatsReset *time.Time     // #8: stats_since for cumulative views
    Sample1    interface{}
    Sample2    interface{}    // nil for instant mode
    Computed   interface{}    // per-second rates for delta mode
}
```

### Apply/Rollback File Lock (H3)

```go
// internal/inspect/lock.go

type ApplyLock struct {
    ChangeSetID string    `json:"changeset_id"`
    PID         int       `json:"pid"`
    StartedAt   time.Time `json:"started_at"`
    Operation   string    `json:"operation"` // "apply" or "rollback"
}

// AcquireLock attempts to create a lock file under ~/.pgdba/snapshots/<fingerprint>/.lock
// Returns error if lock already exists (with details of the holder).
func AcquireLock(fingerprint string, lock ApplyLock) error

// ReleaseLock removes the lock file.
func ReleaseLock(fingerprint string) error

// CheckLock returns the current lock holder, or nil if unlocked.
func CheckLock(fingerprint string) (*ApplyLock, error)
```

---

## pg_control_system() Degradation Strategy (H2)

```
Priority: pg_control_system() → warning + empty SystemIdentifier → fall through to Tier 1

Feature availability driven by server_version_num:
  PG 12-:  basic pg_settings, pg_stat_activity, pg_stat_user_tables
  PG 13+:  pg_control_system() for SystemIdentifier
  PG 14+:  pg_stat_wal
  PG 16+:  pg_stat_io

Each inspect section checks version before querying. Missing = warning in DiagSnapshot, not error.
```

---

## Capability Packs (CP)

### CP-1: Inspect Foundation (Steps 1-3)

**Step 1: Core types + ClusterIdentity + Prereqs**

Files:
- `internal/inspect/identity.go` — ClusterIdentity, ComputeFingerprint
- `internal/inspect/prereq.go` — PrereqResult, CheckPrereqs
- `internal/inspect/types.go` — DiagSnapshot, SectionResult, ChangeSet, ParamChange
- `internal/inspect/lock.go` — ApplyLock, AcquireLock, ReleaseLock, CheckLock

Tests (write first):
- `tests/unit/inspect_identity_test.go` — fingerprint tier fallback, H1 port separation
- `tests/unit/inspect_prereq_test.go` — prereq check with mock PG responses
- `tests/unit/inspect_lock_test.go` — lock acquire/release/contention

**Step 2: DiagSnapshot collector**

Files:
- `internal/inspect/collector.go` — Collect(ctx, db, SamplingConfig) → DiagSnapshot

Sections collected:
- `pg_settings` (all versions)
- `pg_stat_activity` (all versions)
- `pg_stat_user_tables` (all versions)
- `pg_stat_statements` (if extension loaded — prereq check)
- `pg_stat_bgwriter` (all versions, cumulative — supports delta)
- `pg_stat_wal` (PG 14+ — version gated)
- `pg_stat_io` (PG 16+ — version gated)
- `pg_control_system()` (PG 13+ — version gated, H2 degradation)
- system info: `pg_postmaster_start_time()`, `current_setting('data_directory')`

Tests:
- `tests/unit/inspect_collector_test.go` — mock PG returning various versions, verify degradation

**Step 3: CLI `pgdba inspect` command**

```bash
pgdba inspect --name <cluster>          # instant snapshot
pgdba inspect --name <cluster> --delta --interval 30s  # delta mode (M4)
pgdba inspect --name <cluster> --sections pg_settings,pg_stat_statements  # selective
```

Output: JSON DiagSnapshot with all available sections.

---

### CP-2: Config Tuning Engine (Steps 4-7)

**Step 4: Tuning recommendation engine**

Files:
- `internal/tuning/engine.go` — GenerateRecommendations(snapshot, workload, profile)
- `internal/tuning/profiles.go` — OLTP, OLAP, Mixed, RemoteConservative profiles
- `internal/tuning/pgtune.go` — PGTune-equivalent heuristics

Each recommendation includes:
- Parameter name, current value, recommended value
- Confidence level (high/medium/low) (#5)
- Rationale string (#C: explainable)
- pg_settings.context for restart detection (#4)

Tests:
- `tests/unit/tuning_engine_test.go` — various RAM/CPU combos, workload types

**Step 5: Config show/diff commands**

```bash
pgdba config show --name <cluster>              # current pg_settings
pgdba config diff --name <cluster> --workload oltp  # current vs recommended
```

**Step 6: Config apply with safety pipeline (#A, H3, M2, M3)**

```bash
pgdba config apply --name <cluster> --file changeset.json --dry-run   # preview only
pgdba config apply --name <cluster> --file changeset.json             # apply with lock
pgdba config rollback --name <cluster> --changeset-id <uuid>          # rollback
```

Safety pipeline:
1. **Lock** — AcquireLock (H3); reject if already locked
2. **Dry-run / Precheck** — for each parameter:
   - Check ParamPermission (M2: not just superuser binary)
   - Check PatroniOverrideLevel (M3: query Patroni /config if available)
   - Check pg_settings.context (#4: flag postmaster params needing restart)
   - Validate value range/type
3. **Pre-snapshot** — capture DiagSnapshot as rollback baseline
4. **Apply** — ALTER SYSTEM SET for each parameter; pg_reload_conf() for sighup params
5. **Verify** — re-read pg_settings, confirm values match
6. **Record** — save ChangeSet to `~/.pgdba/snapshots/<fingerprint>/`
7. **Unlock** — ReleaseLock

Tests:
- `tests/unit/tuning_apply_test.go` — dry-run, permission denied, Patroni override, lock contention

**Step 7: Config tune (all-in-one)**

```bash
pgdba config tune --name <cluster> --workload oltp --apply --dry-run
```

Combines: inspect → generate recommendations → show diff → optionally apply.

---

### CP-3: Query Analysis (Steps 8-11)

**Step 8: query top + query slow-log**

```bash
pgdba query top --name <cluster> --limit 20 --sort total_time
pgdba query slow-log --name <cluster> --threshold 1s --limit 50
```

Source: `pg_stat_statements` (prereq check first).
Output includes: queryid, query (truncated), calls, mean_time, total_time, rows.

**Step 9: query analyze**

```bash
pgdba query analyze --name <cluster> --sql "SELECT ..." --format json
```

Runs `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` and returns structured plan.

**Step 10: query index-suggest**

```bash
pgdba query index-suggest --name <cluster> --table orders
pgdba query index-suggest --name <cluster>  # all tables
```

Heuristics:
- Sequential scans on large tables (pg_stat_user_tables.seq_scan high, n_live_tup > threshold)
- Missing index on FK columns
- Anti-recommendation: don't suggest for tables < 10k rows (#9)

**Step 11: query locks + query bloat + query vacuum-health**

```bash
pgdba query locks --name <cluster>          # active locks with wait chains (#10)
pgdba query bloat --name <cluster>          # table/index bloat from pg_stat_user_tables (#11: catalog default)
pgdba query vacuum-health --name <cluster>  # autovacuum status, dead tuples, last vacuum times
```

Tests for all query commands:
- `tests/unit/query_top_test.go`
- `tests/unit/query_analyze_test.go`
- `tests/unit/query_index_test.go`
- `tests/unit/query_locks_test.go`
- `tests/unit/query_bloat_test.go`

---

### CP-4: Baseline Report (Step 12)

**Step 12: baseline report**

```bash
pgdba baseline --name <cluster>                           # full instant baseline
pgdba baseline --name <cluster> --delta --interval 60s    # with delta sampling (M4)
pgdba baseline --name <cluster> --sections config,queries  # partial sections (#12)
pgdba baseline diff --before snapshot1.json --after snapshot2.json  # before/after comparison
```

Baseline sections:
- `config` — pg_settings snapshot + recommendations
- `queries` — pg_stat_statements top N
- `tables` — table sizes, bloat, vacuum health
- `replication` — lag, slots, WAL position
- `system` — connections, uptime, version, identity
- `locks` — current lock chains
- `io` — pg_stat_bgwriter (+ pg_stat_wal PG14+, pg_stat_io PG16+)

Each section includes `stats_reset` timestamp for cumulative views (#8).

Output: single JSON document with all sections, suitable for:
- AI analysis input (skill abstraction)
- Before/after comparison for config changes
- Historical tracking in `~/.pgdba/snapshots/<fingerprint>/`

---

## TDD Implementation Order

| Step | What | CP | Key Files |
|------|------|----|-----------|
| 1 | Core types, ClusterIdentity, Prereqs, Lock | CP-1 | inspect/ |
| 2 | DiagSnapshot collector with version gating | CP-1 | inspect/collector.go |
| 3 | CLI `pgdba inspect` command | CP-1 | cli/inspect.go |
| 4 | Tuning engine + profiles | CP-2 | tuning/ |
| 5 | CLI `config show` + `config diff` | CP-2 | cli/config.go |
| 6 | Config apply pipeline (lock, dry-run, verify) | CP-2 | tuning/apply.go |
| 7 | CLI `config tune` (all-in-one) | CP-2 | cli/config.go |
| 8 | query top + slow-log | CP-3 | query/ |
| 9 | query analyze (EXPLAIN wrapper) | CP-3 | query/ |
| 10 | query index-suggest | CP-3 | query/ |
| 11 | query locks + bloat + vacuum-health | CP-3 | query/ |
| 12 | baseline report + diff | CP-4 | baseline/ |

Each step follows strict TDD: write failing test → implement → verify → refactor.

---

## Feedback Tracking

All incorporated feedback with cross-references:

| ID | Description | Status |
|----|-------------|--------|
| #1 | Prereq dedup: single source of truth in inspect/ | Incorporated in CP-1 Step 1 |
| #2 | DiagSnapshot (degradable) vs ChangeSet (must-complete) separation | Incorporated in core types |
| #3 | Patroni ALTER SYSTEM override detection | Incorporated in ParamChange.PatroniOverride |
| #4 | Verify by pg_settings.context (sighup/postmaster/user) | Incorporated in apply pipeline |
| #5 | Remote Conservative Profile with confidence levels | Incorporated in Recommendation type |
| #6 | ClusterIdentity with stable fingerprint | Incorporated in CP-1 Step 1 |
| #7 | Output envelope consistency | All commands use existing JSON envelope |
| #8 | stats_since for cumulative views | Incorporated in BaselineSection.StatsReset |
| #9 | Index anti-recommend for small tables | Incorporated in Step 10 heuristics |
| #10 | Lock wait chains | Incorporated in Step 11 |
| #11 | Bloat from catalog (default, no extensions) | Incorporated in Step 11 |
| #12 | Baseline partial sections | Incorporated in Step 12 --sections flag |
| M1 | Fingerprint upgrade: system_identifier → resolved → config | Incorporated in ClusterIdentity tiers |
| M2 | Per-parameter permission checking | Incorporated in ParamPermission type |
| M3 | Patroni /config for deterministic override detection | Incorporated in PatroniOverrideLevel |
| M4 | Baseline sampling window (instant vs delta) | Incorporated in SamplingConfig |
| H1 | ResolvedPort separate field in fingerprint | Incorporated: fingerprint uses ResolvedAddr + ResolvedPort |
| H2 | pg_control_system() PG 13+ degradation path | Incorporated: version-gated with warning on failure |
| H3 | File lock for apply/rollback mutual exclusion | Incorporated in ApplyLock type + acquire/release |

---

## Dependencies on Existing Code

- `internal/output/` — JSON envelope (already exists from Phase 1)
- `internal/config/` — global config, cluster registry (already exists)
- `internal/patroni/client.go` — Patroni REST API client (already exists, needs `/config` endpoint)
- New dependency: `github.com/jackc/pgx/v5` for direct PG queries (already in go.mod)
