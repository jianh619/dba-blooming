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

// newBaselineCmd returns the "baseline" parent command.
func newBaselineCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Generate a comprehensive baseline report or compare two snapshots",
	}
	cmd.AddCommand(
		newBaselineCollectCmd(cfg, format, reg),
		newBaselineDiffCmd(format),
	)
	return cmd
}

// newBaselineCollectCmd implements "baseline collect" (default subcommand).
func newBaselineCollectCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	var (
		name     string
		delta    bool
		interval time.Duration
		sections string
		savePath string
		workload string
		ramGB    int
		cpuCores int
		storage  string
	)

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect a baseline snapshot with optional tuning recommendations",
		RunE: func(cmd *cobra.Command, args []string) error {
			pgCfg, err := resolvePGConfig(name, cfg, reg)
			if err != nil {
				return writeFailure(cmd, *format, "baseline collect", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				return writeFailure(cmd, *format, "baseline collect", fmt.Errorf("connect: %w", err))
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
				return writeFailure(cmd, *format, "baseline collect",
					fmt.Errorf("collect snapshot: %w", err))
			}

			// Generate tuning recommendations if settings available.
			var recs []inspect.Recommendation
			if s, ok := snap.Sections["pg_settings"]; ok && s.Available {
				if settings, ok := s.Data.([]inspect.PGSetting); ok {
					sysInfo := tuning.SystemInfo{
						TotalRAMBytes: int64(ramGB) * 1024 * 1024 * 1024,
						CPUCores:      cpuCores,
						StorageType:   tuning.StorageType(storage),
					}
					recs = tuning.GenerateRecommendations(settings, sysInfo,
						tuning.Workload(workload), tuning.ProfileDefault)
				}
			}

			report := map[string]interface{}{
				"identity":        snap.Identity,
				"collected_at":    snap.CollectedAt,
				"sections":        snap.Sections,
				"recommendations": recs,
			}

			// Save to file if requested.
			if savePath != "" {
				data, _ := json.MarshalIndent(report, "", "  ")
				if err := os.WriteFile(savePath, data, 0o600); err != nil {
					return writeFailure(cmd, *format, "baseline collect",
						fmt.Errorf("save baseline: %w", err))
				}
				report["saved_to"] = savePath
			}

			resp := output.Success("baseline collect", report)
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "baseline collect", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	cmd.Flags().BoolVar(&delta, "delta", false, "Enable delta sampling")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "Delta sampling interval")
	cmd.Flags().StringVar(&sections, "sections", "", "Comma-separated sections to include (default: all)")
	cmd.Flags().StringVar(&savePath, "save", "", "Save baseline to file path")
	cmd.Flags().StringVar(&workload, "workload", "oltp", "Workload type for recommendations")
	cmd.Flags().IntVar(&ramGB, "ram-gb", 8, "Total RAM in GB")
	cmd.Flags().IntVar(&cpuCores, "cpu-cores", 4, "CPU cores")
	cmd.Flags().StringVar(&storage, "storage", "ssd", "Storage type: ssd|hdd")
	return cmd
}

// newBaselineDiffCmd implements "baseline diff".
func newBaselineDiffCmd(format *output.Format) *cobra.Command {
	var (
		beforePath string
		afterPath  string
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare two baseline snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if beforePath == "" || afterPath == "" {
				return writeFailure(cmd, *format, "baseline diff",
					fmt.Errorf("both --before and --after flags are required"))
			}

			beforeData, err := os.ReadFile(beforePath)
			if err != nil {
				return writeFailure(cmd, *format, "baseline diff",
					fmt.Errorf("read before file: %w", err))
			}
			afterData, err := os.ReadFile(afterPath)
			if err != nil {
				return writeFailure(cmd, *format, "baseline diff",
					fmt.Errorf("read after file: %w", err))
			}

			var before, after map[string]interface{}
			if err := json.Unmarshal(beforeData, &before); err != nil {
				return writeFailure(cmd, *format, "baseline diff",
					fmt.Errorf("parse before file: %w", err))
			}
			if err := json.Unmarshal(afterData, &after); err != nil {
				return writeFailure(cmd, *format, "baseline diff",
					fmt.Errorf("parse after file: %w", err))
			}

			diff := map[string]interface{}{
				"before_file": beforePath,
				"after_file":  afterPath,
				"before":      before,
				"after":       after,
			}

			resp := output.Success("baseline diff", diff)
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return writeFailure(cmd, *format, "baseline diff", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&beforePath, "before", "", "Path to before baseline JSON")
	cmd.Flags().StringVar(&afterPath, "after", "", "Path to after baseline JSON")
	return cmd
}
