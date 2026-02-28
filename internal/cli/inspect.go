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
)

// newInspectCmd returns the "inspect" command.
func newInspectCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var (
		name     string
		delta    bool
		interval time.Duration
	)

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Collect a diagnostic snapshot of the PostgreSQL instance",
		Long: "Inspect gathers pg_settings, pg_stat_activity, pg_stat_statements, " +
			"and other diagnostic data into a single JSON snapshot. " +
			"Supports both instant and delta sampling modes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve PG connection info from registry or config.
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "inspect", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "inspect",
					fmt.Errorf("connect to postgres: %w", err))
			}
			defer conn.Close(ctx)

			samplingCfg := inspect.SamplingConfig{Mode: inspect.SamplingInstant}
			if delta {
				samplingCfg.Mode = inspect.SamplingDelta
				samplingCfg.Interval = interval
			}

			db := inspect.NewPgxDB(conn)
			snap, err := inspect.Collect(ctx, db, samplingCfg, pgCfg.Host, pgCfg.Port)
			if err != nil {
				return writeFailure(cmd, *format, "inspect",
					fmt.Errorf("collect snapshot: %w", err))
			}

			resp := output.Success("inspect", snap)
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "inspect", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Cluster name from registry")
	cmd.Flags().BoolVar(&delta, "delta", false, "Enable delta sampling mode")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "Sampling interval for delta mode")

	return cmd
}

// resolvePGConfig resolves PostgreSQL connection config from the cluster
// registry (if --name is provided) or from the global config/env vars.
func resolvePGConfig(name string, cfg *config.Config, reg *cluster.Registry) (postgres.Config, error) {
	pgCfg := postgres.Config{
		Host:     cfg.PG.Host,
		Port:     cfg.PG.Port,
		User:     cfg.PG.User,
		Database: cfg.PG.Database,
		SSLMode:  cfg.PG.SSLMode,
	}

	if name != "" {
		entry, err := reg.Get(name)
		if err != nil {
			return pgCfg, fmt.Errorf("cluster %q not found in registry: %w", name, err)
		}
		if entry.PGHost != "" {
			pgCfg.Host = entry.PGHost
		}
		if entry.PGPort > 0 {
			pgCfg.Port = entry.PGPort
		}
	}

	if pgCfg.Host == "" {
		return pgCfg, fmt.Errorf("pg host not configured: use --name, PGDBA_PG_HOST, or config file")
	}
	if pgCfg.Port <= 0 {
		pgCfg.Port = 5432
	}
	if pgCfg.User == "" {
		pgCfg.User = "postgres"
	}
	if pgCfg.Database == "" {
		pgCfg.Database = "postgres"
	}
	if pgCfg.SSLMode == "" {
		pgCfg.SSLMode = "prefer"
	}

	return pgCfg, nil
}
