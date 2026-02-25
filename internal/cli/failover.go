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

// newFailoverCmd returns the "failover" parent command.
func newFailoverCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "failover",
		Short: "Trigger or inspect a cluster failover / switchover",
	}
	cmd.AddCommand(
		newFailoverTriggerCmd(format, reg),
		newFailoverStatusCmd(format, reg),
	)
	return cmd
}

// newFailoverTriggerCmd implements "failover trigger".
//
//   - Default (no --force): controlled switchover — both primary and replica
//     participate; no data loss. Runs pre-checks before calling Patroni.
//   - --force: forced failover — used when the primary is unreachable.
//     Pre-checks are skipped; the candidate must be specified.
func newFailoverTriggerCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name, patroniURL, candidate string
	var force bool

	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Trigger a switchover (controlled) or forced failover",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := resolvePatroniURL(name, patroniURL, reg)
			if err != nil {
				return writeFailure(cmd, *format, "failover trigger", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			client := patroni.NewClient(url)

			if force {
				return runForcedFailover(cmd, ctx, client, format, candidate)
			}
			return runSwitchover(cmd, ctx, client, format, candidate)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name (looks up Patroni URL from registry)")
	cmd.Flags().StringVar(&patroniURL, "patroni-url", "", "Patroni API URL")
	cmd.Flags().StringVar(&candidate, "candidate", "", "Target replica to promote (empty = Patroni chooses best)")
	cmd.Flags().BoolVar(&force, "force", false, "Force failover even if primary is unreachable (skips pre-checks)")
	return cmd
}

// runSwitchover performs a controlled switchover with pre-checks.
func runSwitchover(cmd *cobra.Command, ctx context.Context, client *patroni.Client,
	format *output.Format, candidate string) error {

	cs, err := client.GetClusterStatus(ctx)
	if err != nil {
		return writeFailure(cmd, *format, "failover trigger",
			fmt.Errorf("get cluster status: %w", err))
	}

	if err := failover.CheckSwitchover(cs, candidate, failover.DefaultMaxLagBytes); err != nil {
		return writeFailure(cmd, *format, "failover trigger", err)
	}

	primary, err := failover.FindPrimary(cs)
	if err != nil {
		return writeFailure(cmd, *format, "failover trigger", err)
	}

	// Auto-select best candidate if not specified.
	target := candidate
	if target == "" {
		target, err = failover.FindBestCandidate(cs)
		if err != nil {
			return writeFailure(cmd, *format, "failover trigger", err)
		}
	}

	if err := client.Switchover(ctx, primary, target); err != nil {
		return writeFailure(cmd, *format, "failover trigger",
			fmt.Errorf("switchover failed: %w", err))
	}

	resp := output.Success("failover trigger", map[string]string{
		"type":      "switchover",
		"from":      primary,
		"to":        target,
		"status":    "completed",
	})
	out, err := output.FormatResponse(resp, *format)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), out)
	return nil
}

// runForcedFailover performs a forced failover (POST /failover) without pre-checks.
func runForcedFailover(cmd *cobra.Command, ctx context.Context, client *patroni.Client,
	format *output.Format, candidate string) error {

	if candidate == "" {
		return writeFailure(cmd, *format, "failover trigger",
			fmt.Errorf("--candidate is required when using --force"))
	}

	if err := client.Failover(ctx, candidate); err != nil {
		return writeFailure(cmd, *format, "failover trigger",
			fmt.Errorf("forced failover failed: %w", err))
	}

	resp := output.Success("failover trigger", map[string]string{
		"type":      "failover",
		"to":        candidate,
		"status":    "completed",
	})
	out, err := output.FormatResponse(resp, *format)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), out)
	return nil
}

// newFailoverStatusCmd implements "failover status".
func newFailoverStatusCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name, patroniURL string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current cluster failover / switchover state",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := resolvePatroniURL(name, patroniURL, reg)
			if err != nil {
				return writeFailure(cmd, *format, "failover status", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cs, err := patroni.NewClient(url).GetClusterStatus(ctx)
			if err != nil {
				return writeFailure(cmd, *format, "failover status",
					fmt.Errorf("get cluster status: %w", err))
			}

			primary, _ := failover.FindPrimary(cs)
			replicas := failover.ListReplicas(cs)
			replicaNames := make([]string, 0, len(replicas))
			for _, r := range replicas {
				replicaNames = append(replicaNames, r.Name)
			}

			data := map[string]interface{}{
				"primary":           primary,
				"replicas":          replicaNames,
				"failover_in_progress": cs.Failover != nil,
				"paused":            cs.Pause,
				"member_count":      len(cs.Members),
			}

			resp := output.Success("failover status", data)
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
