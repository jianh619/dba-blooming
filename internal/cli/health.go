package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/luckyjian/pgdba/internal/config"
	"github.com/luckyjian/pgdba/internal/output"
	"github.com/luckyjian/pgdba/internal/postgres"
)

// HealthCheckResult holds the aggregated health information for a PostgreSQL instance.
type HealthCheckResult struct {
	PGVersion     string           `json:"pg_version"`
	UptimeSeconds float64          `json:"uptime_seconds"`
	Connections   ConnectionStats  `json:"connections"`
	Replication   ReplicationStats `json:"replication"`
	Healthy       bool             `json:"healthy"`
}

// ConnectionStats holds current and maximum connection counts.
type ConnectionStats struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

// ReplicationStats holds high-availability replication metadata.
type ReplicationStats struct {
	StandbyCount int `json:"standby_count"`
}

func newHealthCmd(cfg *config.Config, format *output.Format) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Health-check related commands",
	}
	cmd.AddCommand(newHealthCheckCmd(cfg, format))
	return cmd
}

func newHealthCheckCmd(cfg *config.Config, format *output.Format) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run a comprehensive health check against the configured PostgreSQL instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			pgCfg := postgres.Config{
				Host:     cfg.PG.Host,
				Port:     cfg.PG.Port,
				User:     cfg.PG.User,
				Database: cfg.PG.Database,
				SSLMode:  cfg.PG.SSLMode,
			}

			conn, err := postgres.Connect(ctx, pgCfg)
			if err != nil {
				resp := output.Failure("health check", err)
				out, _ := output.FormatResponse(resp, *format)
				fmt.Fprintln(os.Stderr, out)
				return err
			}
			defer conn.Close(ctx)

			result, err := runHealthCheck(ctx, conn)
			if err != nil {
				resp := output.Failure("health check", err)
				out, _ := output.FormatResponse(resp, *format)
				fmt.Fprintln(os.Stderr, out)
				return err
			}

			resp := output.Success("health check", result)
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
}

func runHealthCheck(ctx context.Context, conn *pgx.Conn) (*HealthCheckResult, error) {
	result := &HealthCheckResult{Healthy: true}

	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		return nil, fmt.Errorf("query version: %w", err)
	}
	result.PGVersion = version

	var uptime float64
	if err := conn.QueryRow(ctx,
		"SELECT EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time()))").Scan(&uptime); err != nil {
		return nil, fmt.Errorf("query uptime: %w", err)
	}
	result.UptimeSeconds = uptime

	var current, max int
	if err := conn.QueryRow(ctx,
		"SELECT count(*), (SELECT setting::int FROM pg_settings WHERE name='max_connections') "+
			"FROM pg_stat_activity").Scan(&current, &max); err != nil {
		return nil, fmt.Errorf("query connections: %w", err)
	}
	result.Connections = ConnectionStats{Current: current, Max: max}

	var standbyCount int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_stat_replication").Scan(&standbyCount); err != nil {
		standbyCount = 0 // No replication or insufficient privileges — not fatal.
	}
	result.Replication = ReplicationStats{StandbyCount: standbyCount}

	return result, nil
}
