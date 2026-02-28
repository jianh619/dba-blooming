package inspect

import "context"

// PGSetting represents a row from pg_settings.
type PGSetting struct {
	Name       string `json:"name"`
	Setting    string `json:"setting"`
	Unit       string `json:"unit,omitempty"`
	Context    string `json:"context"`
	VarType    string `json:"vartype,omitempty"`
	Source     string `json:"source,omitempty"`
	MinVal     string `json:"min_val,omitempty"`
	MaxVal     string `json:"max_val,omitempty"`
	BootVal    string `json:"boot_val,omitempty"`
	ResetVal   string `json:"reset_val,omitempty"`
	SourceFile string `json:"source_file,omitempty"`
	SourceLine int    `json:"source_line,omitempty"`
}

// PGSSRow represents a row from pg_stat_statements.
type PGSSRow struct {
	QueryID   int64   `json:"queryid"`
	Query     string  `json:"query"`
	Calls     int64   `json:"calls"`
	TotalTime float64 `json:"total_time"`
	MeanTime  float64 `json:"mean_time"`
	Rows      int64   `json:"rows"`
	MinTime   float64 `json:"min_time,omitempty"`
	MaxTime   float64 `json:"max_time,omitempty"`
}

// PGActivity represents a row from pg_stat_activity.
type PGActivity struct {
	PID      int    `json:"pid"`
	State    string `json:"state"`
	Query    string `json:"query"`
	WaitType string `json:"wait_event_type,omitempty"`
	WaitName string `json:"wait_event,omitempty"`
	Backend  string `json:"backend_type,omitempty"`
	DatName  string `json:"datname,omitempty"`
	UserName string `json:"usename,omitempty"`
}

// StatBGWriter represents pg_stat_bgwriter fields.
type StatBGWriter struct {
	CheckpointsTimed int64 `json:"checkpoints_timed"`
	CheckpointsReq   int64 `json:"checkpoints_req"`
	BuffersCheckpoint int64 `json:"buffers_checkpoint"`
	BuffersClean     int64 `json:"buffers_clean"`
	BuffersBackend   int64 `json:"buffers_backend"`
}

// StatWal represents pg_stat_wal fields (PG 14+).
type StatWal struct {
	WalRecords int64 `json:"wal_records"`
	WalBytes   int64 `json:"wal_bytes"`
	WalFPI     int64 `json:"wal_fpi"`
	WalBuffers int64 `json:"wal_buffers_full"`
	WalWrite   int64 `json:"wal_write"`
	WalSync    int64 `json:"wal_sync"`
}

// DB abstracts the PostgreSQL queries needed by the collector.
// This interface enables unit testing with mocks.
type DB interface {
	ServerVersionNum(ctx context.Context) (int, error)
	PGSettings(ctx context.Context) ([]PGSetting, error)
	SystemIdentifier(ctx context.Context) (string, error)
	ResolvedAddr(ctx context.Context) (string, int, error)
	CurrentDatID(ctx context.Context) (uint32, error)
	ExtensionLoaded(ctx context.Context, name string) (bool, error)
	PGStatStatements(ctx context.Context, limit int) ([]PGSSRow, error)
	PGStatActivity(ctx context.Context) ([]PGActivity, error)
	StatBGWriter(ctx context.Context) (*StatBGWriter, error)
	StatWal(ctx context.Context) (*StatWal, error)
}
