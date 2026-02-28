package inspect

import "time"

// SamplingMode controls how baseline data is collected.
type SamplingMode string

const (
	// SamplingInstant takes a single-point snapshot.
	SamplingInstant SamplingMode = "instant"
	// SamplingDelta takes two snapshots separated by an interval.
	SamplingDelta SamplingMode = "delta"
)

// SamplingConfig controls the baseline data collection strategy.
type SamplingConfig struct {
	Mode     SamplingMode
	Interval time.Duration // used only for SamplingDelta
}

// SectionResult represents the outcome of collecting a single diagnostic section.
// Unavailable sections carry a warning (Error) but do not block the snapshot.
type SectionResult struct {
	Available bool
	Error     string
	Data      interface{}
}

// DiagSnapshot is a read-only, degradable collection of diagnostic data.
// Missing sections produce warnings, not errors.
type DiagSnapshot struct {
	Identity    ClusterIdentity
	CollectedAt time.Time
	Sections    map[string]SectionResult
}

// ConfidenceLevel indicates how safe a recommendation is to apply.
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// PatroniOverrideLevel describes how Patroni DCS interacts with a parameter.
type PatroniOverrideLevel string

const (
	// PatroniOverridden means the parameter exists in DCS postgresql.parameters.
	PatroniOverridden PatroniOverrideLevel = "overridden"
	// PatroniEphemeral means Patroni manages PG but the parameter is not in DCS.
	PatroniEphemeral PatroniOverrideLevel = "not_set_but_ephemeral"
	// PatroniUnknown means Patroni /config is unreachable or not configured.
	PatroniUnknown PatroniOverrideLevel = "unknown"
	// PatroniNotManaged means standalone PG without Patroni.
	PatroniNotManaged PatroniOverrideLevel = "not_managed"
)

// ParamPermission describes whether a parameter can be modified with current privileges.
type ParamPermission struct {
	Allowed bool
	Reason  string
	MinRole string
}

// ParamChange represents a single parameter modification in a ChangeSet.
type ParamChange struct {
	Name            string
	OldValue        string
	NewValue        string
	Context         string // pg_settings.context: "sighup", "postmaster", "user", "backend"
	NeedsRestart    bool
	Permission      ParamPermission
	PatroniOverride PatroniOverrideLevel
}

// DryRunResult holds the outcome of a dry-run apply.
type DryRunResult struct {
	OK       bool
	Warnings []string
	Errors   []string
}

// ChangeSet represents a set of parameter changes to apply.
// It must be complete (no degradation) and supports rollback.
type ChangeSet struct {
	ID           string
	Fingerprint  string
	Parameters   []ParamChange
	CreatedAt    time.Time
	AppliedAt    *time.Time
	RolledBackAt *time.Time
	PreSnapshot  *DiagSnapshot
	DryRunResult *DryRunResult
}

// PrereqResult describes whether a prerequisite (extension, function, version) is available.
type PrereqResult struct {
	Name      string
	Available bool
	Version   int
	Error     string
}

// Recommendation represents a tuning suggestion for a single PG parameter.
type Recommendation struct {
	Parameter   string
	Current     string
	Recommended string
	Confidence  ConfidenceLevel
	Rationale   string
	Source      string
}

// BaselineSection holds data for a single baseline section, supporting both
// instant and delta sampling modes.
type BaselineSection struct {
	Name       string
	Mode       SamplingMode
	StatsReset *time.Time
	Sample1    interface{}
	Sample2    interface{} // nil for instant mode
	Computed   interface{} // per-second rates for delta mode
}
