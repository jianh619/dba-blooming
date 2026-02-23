package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/luckyjian/pgdba/internal/config"
	"github.com/luckyjian/pgdba/internal/output"
)

// NewRootCmd builds and returns the root cobra.Command for the pgdba CLI.
func NewRootCmd() *cobra.Command {
	var (
		cfgFile  string
		format   output.Format
		verbose  bool
		provider string
	)

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
			return nil
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default ~/.pgdba/config.yaml)")
	root.PersistentFlags().StringVar((*string)(&format), "format", "json", "Output format: json|table|yaml")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	root.PersistentFlags().StringVar(&provider, "provider", "docker", "Infrastructure provider: docker|baremetal|kubernetes")

	// Attach sub-commands with a lazily-initialized default config.
	root.AddCommand(newHealthCmd(&config.Config{}, &format))

	return root
}

// Execute runs the root command and exits with code 1 on error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
