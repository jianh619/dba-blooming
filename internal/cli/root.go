package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/luckyjian/pgdba/internal/cluster"
	"github.com/luckyjian/pgdba/internal/config"
	"github.com/luckyjian/pgdba/internal/output"
)

// NewRootCmd builds and returns the root cobra.Command for the pgdba CLI.
// It uses the default cluster registry (~/.pgdba/clusters.json).
func NewRootCmd() *cobra.Command {
	reg, _ := cluster.DefaultRegistry()
	return buildRootCmd(reg)
}

// NewRootCmdWithRegistry returns the root command using a custom registry path.
// This is used in tests to inject a temporary registry file.
func NewRootCmdWithRegistry(registryPath string) *cobra.Command {
	reg := cluster.NewRegistry(registryPath)
	return buildRootCmd(reg)
}

// buildRootCmd constructs the cobra command tree given a cluster registry.
func buildRootCmd(reg *cluster.Registry) *cobra.Command {
	var (
		cfgFile  string
		format   output.Format
		verbose  bool
		provider string
	)

	// cfg is a shared pointer populated in PersistentPreRunE before any
	// subcommand runs, ensuring environment variables and config file are loaded.
	cfg := &config.Config{}

	root := &cobra.Command{
		Use:   "pgdba",
		Short: "PostgreSQL virtual DBA expert system",
		Long: "pgdba provides a full suite of PostgreSQL DBA operations — high-availability " +
			"deployment, failover, backup/restore, monitoring, and tuning — all outputting " +
			"AI-parseable JSON.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			switch format {
			case output.FormatJSON, output.FormatTable, output.FormatYAML:
			default:
				return fmt.Errorf(
					"invalid format %q: must be json, table, or yaml", format,
				)
			}
			loaded, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			*cfg = *loaded
			_ = verbose
			_ = provider
			return nil
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default ~/.pgdba/config.yaml)")
	root.PersistentFlags().StringVar((*string)(&format), "format", "json", "Output format: json|table|yaml")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	root.PersistentFlags().StringVar(&provider, "provider", "docker", "Infrastructure provider: docker|baremetal|kubernetes")

	root.AddCommand(newHealthCmd(cfg, &format))
	root.AddCommand(newClusterCmd(cfg, &format, reg))

	return root
}

// Execute runs the root command and exits with code 1 on error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
