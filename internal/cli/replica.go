package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/luckyjian/pgdba/internal/cluster"
	"github.com/luckyjian/pgdba/internal/failover"
	"github.com/luckyjian/pgdba/internal/output"
	"github.com/luckyjian/pgdba/internal/patroni"
)

// newReplicaCmd returns the "replica" parent command.
func newReplicaCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replica",
		Short: "Manage cluster replicas (list, promote)",
	}
	cmd.AddCommand(
		newReplicaListCmd(format, reg),
		newReplicaPromoteCmd(format, reg),
	)
	return cmd
}

// newReplicaListCmd implements "replica list".
func newReplicaListCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name, patroniURL string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all replica nodes and their replication lag",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := resolvePatroniURL(name, patroniURL, reg)
			if err != nil {
				return writeFailure(cmd, *format, "replica list", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cs, err := patroni.NewClient(url).GetClusterStatus(ctx)
			if err != nil {
				return writeFailure(cmd, *format, "replica list",
					fmt.Errorf("get cluster status: %w", err))
			}

			replicas := failover.ListReplicas(cs)
			type replicaRow struct {
				Name  string    `json:"name"`
				State string    `json:"state"`
				Lag   int64     `json:"lag_bytes"`
				Host  string    `json:"host"`
				Port  int       `json:"port"`
			}
			rows := make([]replicaRow, 0, len(replicas))
			for _, r := range replicas {
				rows = append(rows, replicaRow{
					Name:  r.Name,
					State: string(r.State),
					Lag:   r.Lag,
					Host:  r.Host,
					Port:  r.Port,
				})
			}

			resp := output.Success("replica list", map[string]interface{}{
				"replicas": rows,
				"count":    len(rows),
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name (looks up Patroni URL from registry)")
	cmd.Flags().StringVar(&patroniURL, "patroni-url", "", "Patroni API URL")
	return cmd
}

// newReplicaPromoteCmd implements "replica promote".
// It runs a controlled switchover targeting the specified candidate.
func newReplicaPromoteCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name, patroniURL, candidate string

	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote a replica to primary via controlled switchover",
		RunE: func(cmd *cobra.Command, args []string) error {
			if candidate == "" {
				return writeFailure(cmd, *format, "replica promote",
					fmt.Errorf("--candidate is required"))
			}

			url, err := resolvePatroniURL(name, patroniURL, reg)
			if err != nil {
				return writeFailure(cmd, *format, "replica promote", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			client := patroni.NewClient(url)
			cs, err := client.GetClusterStatus(ctx)
			if err != nil {
				return writeFailure(cmd, *format, "replica promote",
					fmt.Errorf("get cluster status: %w", err))
			}

			// Validate the candidate before calling Patroni.
			if err := failover.CheckSwitchover(cs, candidate, failover.DefaultMaxLagBytes); err != nil {
				return writeFailure(cmd, *format, "replica promote", err)
			}

			primary, err := failover.FindPrimary(cs)
			if err != nil {
				return writeFailure(cmd, *format, "replica promote", err)
			}

			if err := client.Switchover(ctx, primary, candidate); err != nil {
				return writeFailure(cmd, *format, "replica promote",
					fmt.Errorf("switchover failed: %w", err))
			}

			resp := output.Success("replica promote", map[string]string{
				"candidate": candidate,
				"from":      primary,
				"status":    "promoted",
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name (looks up Patroni URL from registry)")
	cmd.Flags().StringVar(&patroniURL, "patroni-url", "", "Patroni API URL")
	cmd.Flags().StringVar(&candidate, "candidate", "", "Replica node name to promote")
	return cmd
}
