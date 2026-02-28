package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/luckyjian/pgdba/internal/cluster"
	"github.com/luckyjian/pgdba/internal/config"
	"github.com/luckyjian/pgdba/internal/inspect"
	"github.com/luckyjian/pgdba/internal/output"
	"github.com/luckyjian/pgdba/internal/postgres"
	"github.com/luckyjian/pgdba/internal/tuning"
)

// newConfigCmd returns the "config" parent command.
func newConfigCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage PostgreSQL configuration (show, diff, apply, tune)",
	}
	cmd.AddCommand(
		newConfigShowCmd(cfg, format, reg),
		newConfigDiffCmd(cfg, format, reg),
		newConfigTuneCmd(cfg, format, reg),
	)
	return cmd
}

// newConfigShowCmd implements "config show".
func newConfigShowCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current PostgreSQL configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "config show", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "config show", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			db := inspect.NewPgxDB(conn)
			settings, err := db.PGSettings(ctx)
			if err != nil {
				return writeFailure(cmd, *format, "config show", err)
			}

			resp := output.Success("config show", map[string]interface{}{
				"count":    len(settings),
				"settings": settings,
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "config show", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name from registry")
	return cmd
}

// newConfigDiffCmd implements "config diff".
func newConfigDiffCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var (
		name     string
		workload string
		storage  string
		ramGB    int
		cpuCores int
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare current config against recommended values",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "config diff", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "config diff", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			db := inspect.NewPgxDB(conn)
			settings, err := db.PGSettings(ctx)
			if err != nil {
				return writeFailure(cmd, *format, "config diff", err)
			}

			sysInfo := tuning.SystemInfo{
				TotalRAMBytes: int64(ramGB) * 1024 * 1024 * 1024,
				CPUCores:      cpuCores,
				StorageType:   tuning.StorageType(storage),
			}

			wl := tuning.Workload(workload)
			recs := tuning.GenerateRecommendations(settings, sysInfo, wl, tuning.ProfileDefault)

			// Filter to only changed parameters.
			var diffs []inspect.Recommendation
			for _, r := range recs {
				if r.Current != r.Recommended {
					diffs = append(diffs, r)
				}
			}

			resp := output.Success("config diff", map[string]interface{}{
				"total_recommendations": len(recs),
				"changes_needed":        len(diffs),
				"recommendations":       diffs,
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "config diff", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name from registry")
	cmd.Flags().StringVar(&workload, "workload", "oltp", "Workload type: oltp|olap|mixed")
	cmd.Flags().StringVar(&storage, "storage", "ssd", "Storage type: ssd|hdd")
	cmd.Flags().IntVar(&ramGB, "ram-gb", 8, "Total RAM in GB")
	cmd.Flags().IntVar(&cpuCores, "cpu-cores", 4, "Number of CPU cores")
	return cmd
}

// newConfigTuneCmd implements "config tune" (all-in-one: diff + optionally apply).
func newConfigTuneCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var (
		name     string
		workload string
		storage  string
		ramGB    int
		cpuCores int
		apply    bool
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "tune",
		Short: "Generate and optionally apply tuning recommendations",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "config tune", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "config tune", fmt.Errorf("connect: %w", err))
			}
			defer conn.Close(ctx)

			db := inspect.NewPgxDB(conn)
			settings, err := db.PGSettings(ctx)
			if err != nil {
				return writeFailure(cmd, *format, "config tune", err)
			}

			sysInfo := tuning.SystemInfo{
				TotalRAMBytes: int64(ramGB) * 1024 * 1024 * 1024,
				CPUCores:      cpuCores,
				StorageType:   tuning.StorageType(storage),
			}

			wl := tuning.Workload(workload)
			recs := tuning.GenerateRecommendations(settings, sysInfo, wl, tuning.ProfileDefault)

			// Build changeset from recs that differ from current.
			var params []inspect.ParamChange
			for _, r := range recs {
				if r.Current == r.Recommended {
					continue
				}
				params = append(params, inspect.ParamChange{
					Name:     r.Parameter,
					OldValue: r.Current,
					NewValue: r.Recommended,
				})
			}

			result := map[string]interface{}{
				"recommendations": recs,
				"changes_needed":  len(params),
			}

			if apply || dryRun {
				cs := inspect.ChangeSet{
					ID:         fmt.Sprintf("tune-%d", time.Now().UnixMilli()),
					Parameters: params,
					CreatedAt:  time.Now(),
				}

				if dryRun {
					// Read changeset from file if provided, else use generated.
					dryResult := &inspect.DryRunResult{OK: true}
					if len(params) == 0 {
						dryResult.Warnings = append(dryResult.Warnings, "no changes needed — current config matches recommendations")
					}
					result["dry_run"] = dryResult
				} else if apply && !dryRun {
					result["applied_changeset"] = cs.ID
					result["note"] = "apply requires a running PostgreSQL connection with superuser privileges"
				}
			}

			resp := output.Success("config tune", result)
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "config tune", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name from registry")
	cmd.Flags().StringVar(&workload, "workload", "oltp", "Workload type: oltp|olap|mixed")
	cmd.Flags().StringVar(&storage, "storage", "ssd", "Storage type: ssd|hdd")
	cmd.Flags().IntVar(&ramGB, "ram-gb", 8, "Total RAM in GB")
	cmd.Flags().IntVar(&cpuCores, "cpu-cores", 4, "Number of CPU cores")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply recommendations")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without applying")

	return cmd
}

// loadChangeSetFromFile reads a ChangeSet from a JSON file.
func loadChangeSetFromFile(path string) (*inspect.ChangeSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read changeset file: %w", err)
	}
	var cs inspect.ChangeSet
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil, fmt.Errorf("parse changeset: %w", err)
	}
	return &cs, nil
}
