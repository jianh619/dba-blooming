package inspect

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PgxDB implements the DB interface using a real pgx.Conn.
type PgxDB struct {
	conn *pgx.Conn
}

// NewPgxDB wraps a pgx connection as an inspect.DB.
func NewPgxDB(conn *pgx.Conn) *PgxDB {
	return &PgxDB{conn: conn}
}

func (p *PgxDB) ServerVersionNum(ctx context.Context) (int, error) {
	var v int
	err := p.conn.QueryRow(ctx, "SHOW server_version_num").Scan(&v)
	return v, err
}

func (p *PgxDB) PGSettings(ctx context.Context) ([]PGSetting, error) {
	rows, err := p.conn.Query(ctx,
		`SELECT name, setting, COALESCE(unit,''), context, COALESCE(vartype,''),
		        COALESCE(source,''), COALESCE(min_val,''), COALESCE(max_val,''),
		        COALESCE(boot_val,''), COALESCE(reset_val,''),
		        COALESCE(sourcefile,''), COALESCE(sourceline,0)
		 FROM pg_settings ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []PGSetting
	for rows.Next() {
		var s PGSetting
		if err := rows.Scan(&s.Name, &s.Setting, &s.Unit, &s.Context, &s.VarType,
			&s.Source, &s.MinVal, &s.MaxVal, &s.BootVal, &s.ResetVal,
			&s.SourceFile, &s.SourceLine); err != nil {
			return nil, err
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}

func (p *PgxDB) SystemIdentifier(ctx context.Context) (string, error) {
	var id string
	err := p.conn.QueryRow(ctx,
		"SELECT system_identifier::text FROM pg_control_system()").Scan(&id)
	return id, err
}

func (p *PgxDB) ResolvedAddr(ctx context.Context) (string, int, error) {
	var addr string
	var port int
	err := p.conn.QueryRow(ctx,
		"SELECT COALESCE(inet_server_addr()::text,''), COALESCE(inet_server_port(),0)").Scan(&addr, &port)
	return addr, port, err
}

func (p *PgxDB) CurrentDatID(ctx context.Context) (uint32, error) {
	var id uint32
	err := p.conn.QueryRow(ctx,
		"SELECT d.oid FROM pg_stat_activity a JOIN pg_database d ON d.datname = a.datname WHERE a.pid = pg_backend_pid()").Scan(&id)
	return id, err
}

func (p *PgxDB) ExtensionLoaded(ctx context.Context, name string) (bool, error) {
	var count int
	err := p.conn.QueryRow(ctx,
		"SELECT count(*) FROM pg_extension WHERE extname = $1", name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *PgxDB) PGStatStatements(ctx context.Context, limit int) ([]PGSSRow, error) {
	query := fmt.Sprintf(
		`SELECT queryid, query, calls, total_exec_time, mean_exec_time, rows,
		        min_exec_time, max_exec_time
		 FROM pg_stat_statements
		 ORDER BY total_exec_time DESC
		 LIMIT %d`, limit)
	rows, err := p.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PGSSRow
	for rows.Next() {
		var r PGSSRow
		if err := rows.Scan(&r.QueryID, &r.Query, &r.Calls, &r.TotalTime,
			&r.MeanTime, &r.Rows, &r.MinTime, &r.MaxTime); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (p *PgxDB) PGStatActivity(ctx context.Context) ([]PGActivity, error) {
	rows, err := p.conn.Query(ctx,
		`SELECT pid, COALESCE(state,''), COALESCE(query,''),
		        COALESCE(wait_event_type,''), COALESCE(wait_event,''),
		        COALESCE(backend_type,''), COALESCE(datname,''), COALESCE(usename,'')
		 FROM pg_stat_activity
		 WHERE pid <> pg_backend_pid()
		 ORDER BY state, pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PGActivity
	for rows.Next() {
		var a PGActivity
		if err := rows.Scan(&a.PID, &a.State, &a.Query, &a.WaitType,
			&a.WaitName, &a.Backend, &a.DatName, &a.UserName); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (p *PgxDB) StatBGWriter(ctx context.Context) (*StatBGWriter, error) {
	var s StatBGWriter
	err := p.conn.QueryRow(ctx,
		`SELECT checkpoints_timed, checkpoints_req,
		        buffers_checkpoint, buffers_clean, buffers_backend
		 FROM pg_stat_bgwriter`).Scan(
		&s.CheckpointsTimed, &s.CheckpointsReq,
		&s.BuffersCheckpoint, &s.BuffersClean, &s.BuffersBackend)
	return &s, err
}

func (p *PgxDB) StatWal(ctx context.Context) (*StatWal, error) {
	var s StatWal
	err := p.conn.QueryRow(ctx,
		`SELECT wal_records, wal_bytes, wal_fpi, wal_buffers_full, wal_write, wal_sync
		 FROM pg_stat_wal`).Scan(
		&s.WalRecords, &s.WalBytes, &s.WalFPI, &s.WalBuffers, &s.WalWrite, &s.WalSync)
	return &s, err
}
