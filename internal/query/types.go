package query

import "context"

// TopQuery represents a row from pg_stat_statements ordered by a metric.
type TopQuery struct {
	QueryID   int64   `json:"queryid"`
	Query     string  `json:"query"`
	Calls     int64   `json:"calls"`
	TotalTime float64 `json:"total_time_ms"`
	MeanTime  float64 `json:"mean_time_ms"`
	Rows      int64   `json:"rows"`
	MinTime   float64 `json:"min_time_ms,omitempty"`
	MaxTime   float64 `json:"max_time_ms,omitempty"`
}

// LockInfo represents a lock with wait chain information.
type LockInfo struct {
	PID          int    `json:"pid"`
	Mode         string `json:"mode"`
	Granted      bool   `json:"granted"`
	Relation     string `json:"relation"`
	WaitingPIDs  []int  `json:"waiting_pids,omitempty"`
	BlockedByPID int    `json:"blocked_by_pid,omitempty"`
}

// LockChain represents a lock dependency chain with a root holder.
type LockChain struct {
	RootPID     int    `json:"root_pid"`
	Mode        string `json:"mode"`
	Relation    string `json:"relation"`
	WaitingPIDs []int  `json:"waiting_pids"`
}

// TableBloat represents bloat estimation for a single table.
type TableBloat struct {
	Schema     string  `json:"schema"`
	Table      string  `json:"table"`
	TableBytes int64   `json:"table_bytes"`
	BloatBytes int64   `json:"bloat_bytes"`
	BloatRatio float64 `json:"bloat_ratio"`
}

// VacuumHealth represents vacuum status for a table.
type VacuumHealth struct {
	Schema          string `json:"schema"`
	Table           string `json:"table"`
	DeadTuples      int64  `json:"dead_tuples"`
	LiveTuples      int64  `json:"live_tuples"`
	LastVacuum      string `json:"last_vacuum,omitempty"`
	LastAutoVacuum  string `json:"last_autovacuum,omitempty"`
	AutoVacuumCount int64  `json:"autovacuum_count"`
}

// DeadTupleRatio returns the ratio of dead tuples to total tuples.
func (v VacuumHealth) DeadTupleRatio() float64 {
	total := v.LiveTuples + v.DeadTuples
	if total == 0 {
		return 0
	}
	return float64(v.DeadTuples) / float64(total)
}

// TableStat represents pg_stat_user_tables data relevant to index suggestions.
type TableStat struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	SeqScan    int64  `json:"seq_scan"`
	SeqTupRead int64  `json:"seq_tup_read"`
	IdxScan    int64  `json:"idx_scan"`
	NLiveTup   int64  `json:"n_live_tup"`
}

// IndexSuggestion represents a missing index recommendation.
type IndexSuggestion struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Reason string `json:"reason"`
	SeqScan int64 `json:"seq_scan"`
	NLiveTup int64 `json:"n_live_tup"`
}

// DB abstracts the database queries needed by the query analysis package.
type DB interface {
	TopQueries(ctx context.Context, limit int, sortBy string) ([]TopQuery, error)
	ActiveLocks(ctx context.Context) ([]LockInfo, error)
	TableBloat(ctx context.Context) ([]TableBloat, error)
	VacuumHealthCheck(ctx context.Context) ([]VacuumHealth, error)
	TableStats(ctx context.Context) ([]TableStat, error)
}
