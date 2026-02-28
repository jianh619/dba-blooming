package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/luckyjian/pgdba/internal/cluster"
	"github.com/luckyjian/pgdba/internal/config"
	"github.com/luckyjian/pgdba/internal/inspect"
	"github.com/luckyjian/pgdba/internal/output"
	"github.com/luckyjian/pgdba/internal/postgres"
	"github.com/luckyjian/pgdba/internal/query"
)

// newQueryCmd returns the "query" parent command.
func newQueryCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query analysis (top, slow-log, analyze, index-suggest, locks, bloat, vacuum-health)",
	}
	cmd.AddCommand(
		newQueryTopCmd(cfg, format, reg),
		newQueryAnalyzeCmd(cfg, format, reg),
		newQueryIndexSuggestCmd(cfg, format, reg),
		newQueryLocksCmd(cfg, format, reg),
		newQueryBloatCmd(cfg, format, reg),
		newQueryVacuumHealthCmd(cfg, format, reg),
	)
	return cmd
}

// newQueryTopCmd implements "query top" (also serves as slow-log).
func newQueryTopCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var (
		name   string
		limit  int
		sortBy string
	)

	cmd := &cobra.Command{
		Use:   "top",
		Short: "Show top queries by resource consumption (pg_stat_statements)",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "query top", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "query top", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			db := inspect.NewPgxDB(conn)
			// Check if pg_stat_statements is loaded.
			loaded, err := db.ExtensionLoaded(ctx, "pg_stat_statements")
			if err != nil || !loaded {
				return writeFailure(cmd, *format, "query top",
					fmt.Errorf("pg_stat_statements extension not loaded — run: CREATE EXTENSION pg_stat_statements"))
			}

			rows, err := db.PGStatStatements(ctx, limit)
			if err != nil {
				return writeFailure(cmd, *format, "query top", err)
			}

			resp := output.Success("query top", map[string]interface{}{
				"count":   len(rows),
				"sort_by": sortBy,
				"queries": rows,
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "query top", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	cmd.Flags().IntVar(&limit, "limit", 20, "Number of queries to show")
	cmd.Flags().StringVar(&sortBy, "sort", "total_time", "Sort by: total_time|calls|mean_time")
	return cmd
}

// newQueryAnalyzeCmd implements "query analyze".
func newQueryAnalyzeCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var (
		name string
		sql  string
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Run EXPLAIN ANALYZE on a SQL query",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sql == "" {
				return writeFailure(cmd, *format, "query analyze",
					fmt.Errorf("--sql flag is required"))
			}

			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "query analyze", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "query analyze", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			explainSQL := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", sql)
			var planJSON string
			if err := conn.QueryRow(ctx, explainSQL).Scan(&planJSON); err != nil {
				return writeFailure(cmd, *format, "query analyze",
					fmt.Errorf("EXPLAIN failed: %w", err))
			}

			resp := output.Success("query analyze", map[string]interface{}{
				"sql":  sql,
				"plan": planJSON,
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "query analyze", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	cmd.Flags().StringVar(&sql, "sql", "", "SQL query to analyze")
	return cmd
}

// newQueryIndexSuggestCmd implements "query index-suggest".
func newQueryIndexSuggestCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var (
		name    string
		table   string
		minRows int64
	)

	cmd := &cobra.Command{
		Use:   "index-suggest",
		Short: "Suggest missing indexes based on table statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "query index-suggest", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "query index-suggest", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			// Query table stats from pg_stat_user_tables.
			querySQL := `SELECT schemaname, relname,
				COALESCE(seq_scan,0), COALESCE(seq_tup_read,0),
				COALESCE(idx_scan,0), COALESCE(n_live_tup,0)
				FROM pg_stat_user_tables`
			if table != "" {
				querySQL += fmt.Sprintf(" WHERE relname = '%s'", table) // safe: single table name
			}
			querySQL += " ORDER BY seq_scan DESC"

			rows, err := conn.Query(ctx, querySQL)
			if err != nil {
				return writeFailure(cmd, *format, "query index-suggest", err)
			}
			defer rows.Close()

			var stats []query.TableStat
			for rows.Next() {
				var s query.TableStat
				if err := rows.Scan(&s.Schema, &s.Table, &s.SeqScan, &s.SeqTupRead, &s.IdxScan, &s.NLiveTup); err != nil {
					return writeFailure(cmd, *format, "query index-suggest", err)
				}
				stats = append(stats, s)
			}
			if err := rows.Err(); err != nil {
				return writeFailure(cmd, *format, "query index-suggest", err)
			}

			suggestions := query.SuggestIndexes(stats, minRows)

			resp := output.Success("query index-suggest", map[string]interface{}{
				"tables_analyzed": len(stats),
				"suggestions":    suggestions,
				"count":          len(suggestions),
				"min_rows":       minRows,
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "query index-suggest", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	cmd.Flags().StringVar(&table, "table", "", "Specific table to analyze (optional)")
	cmd.Flags().Int64Var(&minRows, "min-rows", 10000, "Minimum rows to consider for index suggestions")
	return cmd
}

// newQueryLocksCmd implements "query locks".
func newQueryLocksCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "locks",
		Short: "Show active locks and wait chains",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "query locks", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "query locks", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			lockSQL := `SELECT l.pid, l.mode, l.granted,
				COALESCE(c.relname,'') AS relation,
				COALESCE(array_agg(bl.pid) FILTER (WHERE bl.pid IS NOT NULL), '{}') AS waiting_pids
				FROM pg_locks l
				LEFT JOIN pg_class c ON l.relation = c.oid
				LEFT JOIN pg_locks bl ON bl.locktype = l.locktype
					AND bl.database IS NOT DISTINCT FROM l.database
					AND bl.relation IS NOT DISTINCT FROM l.relation
					AND bl.page IS NOT DISTINCT FROM l.page
					AND bl.tuple IS NOT DISTINCT FROM l.tuple
					AND bl.granted = false AND l.granted = true AND bl.pid != l.pid
				WHERE l.pid != pg_backend_pid()
				GROUP BY l.pid, l.mode, l.granted, c.relname
				ORDER BY l.granted DESC, l.pid`

			rows, err := conn.Query(ctx, lockSQL)
			if err != nil {
				return writeFailure(cmd, *format, "query locks", err)
			}
			defer rows.Close()

			var locks []query.LockInfo
			for rows.Next() {
				var l query.LockInfo
				if err := rows.Scan(&l.PID, &l.Mode, &l.Granted, &l.Relation, &l.WaitingPIDs); err != nil {
					return writeFailure(cmd, *format, "query locks", err)
				}
				locks = append(locks, l)
			}

			chains := query.BuildLockChains(locks)

			resp := output.Success("query locks", map[string]interface{}{
				"total_locks":  len(locks),
				"lock_chains":  chains,
				"chain_count":  len(chains),
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "query locks", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	return cmd
}

// newQueryBloatCmd implements "query bloat".
func newQueryBloatCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "bloat",
		Short: "Estimate table and index bloat from catalog stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "query bloat", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "query bloat", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			// Bloat estimation from pg catalog (no extension needed — #11).
			bloatSQL := `SELECT schemaname, tablename,
				pg_total_relation_size(schemaname || '.' || tablename) AS table_bytes,
				GREATEST(
					pg_total_relation_size(schemaname || '.' || tablename)
					- pg_relation_size(schemaname || '.' || tablename), 0
				) AS bloat_bytes,
				CASE WHEN pg_total_relation_size(schemaname || '.' || tablename) > 0
					THEN ROUND(
						(pg_total_relation_size(schemaname || '.' || tablename)
						 - pg_relation_size(schemaname || '.' || tablename))::numeric
						/ pg_total_relation_size(schemaname || '.' || tablename), 4)
					ELSE 0 END AS bloat_ratio
				FROM pg_tables
				WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
				ORDER BY bloat_bytes DESC
				LIMIT 50`

			rows, err := conn.Query(ctx, bloatSQL)
			if err != nil {
				return writeFailure(cmd, *format, "query bloat", err)
			}
			defer rows.Close()

			var bloats []query.TableBloat
			for rows.Next() {
				var b query.TableBloat
				if err := rows.Scan(&b.Schema, &b.Table, &b.TableBytes, &b.BloatBytes, &b.BloatRatio); err != nil {
					return writeFailure(cmd, *format, "query bloat", err)
				}
				bloats = append(bloats, b)
			}

			resp := output.Success("query bloat", map[string]interface{}{
				"count":  len(bloats),
				"tables": bloats,
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "query bloat", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	return cmd
}

// newQueryVacuumHealthCmd implements "query vacuum-health".
func newQueryVacuumHealthCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "vacuum-health",
		Short: "Show vacuum status, dead tuples, and autovacuum activity",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "query vacuum-health", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "query vacuum-health", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			vacSQL := `SELECT schemaname, relname,
				COALESCE(n_dead_tup, 0), COALESCE(n_live_tup, 0),
				COALESCE(last_vacuum::text, ''), COALESCE(last_autovacuum::text, ''),
				COALESCE(autovacuum_count, 0)
				FROM pg_stat_user_tables
				ORDER BY n_dead_tup DESC
				LIMIT 50`

			rows, err := conn.Query(ctx, vacSQL)
			if err != nil {
				return writeFailure(cmd, *format, "query vacuum-health", err)
			}
			defer rows.Close()

			var stats []query.VacuumHealth
			for rows.Next() {
				var v query.VacuumHealth
				if err := rows.Scan(&v.Schema, &v.Table, &v.DeadTuples, &v.LiveTuples,
					&v.LastVacuum, &v.LastAutoVacuum, &v.AutoVacuumCount); err != nil {
					return writeFailure(cmd, *format, "query vacuum-health", err)
				}
				stats = append(stats, v)
			}

			resp := output.Success("query vacuum-health", map[string]interface{}{
				"count":  len(stats),
				"tables": stats,
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "query vacuum-health", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	return cmd
}
