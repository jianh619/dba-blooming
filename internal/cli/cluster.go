package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/luckyjian/pgdba/internal/cluster"
	"github.com/luckyjian/pgdba/internal/config"
	"github.com/luckyjian/pgdba/internal/output"
	"github.com/luckyjian/pgdba/internal/patroni"
)

// ClusterStatusResult holds the cluster topology response data.
type ClusterStatusResult struct {
	ClusterName  string            `json:"cluster_name"`
	Members      []clusterMember   `json:"members"`
	Primary      string            `json:"primary"`
	ReplicaCount int               `json:"replica_count"`
	Healthy      bool              `json:"healthy"`
}

type clusterMember struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	State string `json:"state"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Lag   int64  `json:"lag"`
}

// newClusterCmd returns the "cluster" parent command with all sub-commands.
func newClusterCmd(cfg *config.Config, format *output.Format, reg *cluster.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Cluster lifecycle management (status, connect, init, destroy)",
	}
	cmd.AddCommand(
		newClusterStatusCmd(format, reg),
		newClusterConnectCmd(format, reg),
		newClusterInitCmd(cfg, format),
		newClusterDestroyCmd(format, reg),
	)
	return cmd
}

// newClusterStatusCmd returns "cluster status" which shows topology from Patroni API.
func newClusterStatusCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name, patroniURL string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show cluster topology from Patroni API",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := resolvePatroniURL(name, patroniURL, reg)
			if err != nil {
				return writeFailure(cmd, *format, "cluster status", err)
			}

			client := patroni.NewClient(url)
			cs, err := client.GetClusterStatus(context.Background())
			if err != nil {
				return writeFailure(cmd, *format, "cluster status", err)
			}

			result := buildStatusResult(name, cs)
			resp := output.Success("cluster status", result)
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name (looks up Patroni URL from registry)")
	cmd.Flags().StringVar(&patroniURL, "patroni-url", "", "Patroni API URL (e.g. http://10.0.0.1:8008)")
	return cmd
}

// newClusterConnectCmd returns "cluster connect" which imports an existing Patroni cluster.
func newClusterConnectCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name, patroniURL, pgHost, provider string
	var pgPort int

	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Import an existing Patroni cluster into the registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return writeFailure(cmd, *format, "cluster connect",
					fmt.Errorf("--name is required"))
			}
			if patroniURL == "" {
				return writeFailure(cmd, *format, "cluster connect",
					fmt.Errorf("--patroni-url is required"))
			}
			if pgHost == "" {
				return writeFailure(cmd, *format, "cluster connect",
					fmt.Errorf("--pg-host is required"))
			}

			// Pre-flight: verify Patroni is reachable.
			client := patroni.NewClient(patroniURL)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := client.GetClusterStatus(ctx); err != nil {
				return writeFailure(cmd, *format, "cluster connect",
					fmt.Errorf("Patroni unreachable at %s: %w", patroniURL, err))
			}

			entry := cluster.Entry{
				Name:       name,
				PatroniURL: patroniURL,
				PGHost:     pgHost,
				PGPort:     pgPort,
				Provider:   provider,
				Source:     cluster.SourceExternal,
				CreatedAt:  time.Now().UTC(),
			}
			if err := reg.Add(entry); err != nil {
				return writeFailure(cmd, *format, "cluster connect",
					fmt.Errorf("write registry: %w", err))
			}

			resp := output.Success("cluster connect", map[string]string{
				"cluster": name,
				"status":  "registered",
				"source":  string(cluster.SourceExternal),
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name to register")
	cmd.Flags().StringVar(&patroniURL, "patroni-url", "", "Patroni REST API URL")
	cmd.Flags().StringVar(&pgHost, "pg-host", "", "PostgreSQL host")
	cmd.Flags().IntVar(&pgPort, "pg-port", 5432, "PostgreSQL port")
	cmd.Flags().StringVar(&provider, "provider", "baremetal", "Infrastructure provider")
	return cmd
}

// newClusterInitCmd returns "cluster init" which bootstraps a new managed cluster.
func newClusterInitCmd(cfg *config.Config, format *output.Format) *cobra.Command {
	var name, primaryHost string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a new managed Patroni/PostgreSQL cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if primaryHost == "" {
				return writeFailure(cmd, *format, "cluster init",
					fmt.Errorf("--primary-host is required"))
			}
			if name == "" {
				return writeFailure(cmd, *format, "cluster init",
					fmt.Errorf("--name is required"))
			}
			// Provider.CreateNode is not yet implemented.
			return writeFailure(cmd, *format, "cluster init",
				fmt.Errorf("provider CreateNode: not implemented"))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	cmd.Flags().StringVar(&primaryHost, "primary-host", "", "Host for the primary node")
	_ = cfg
	return cmd
}

// newClusterDestroyCmd returns "cluster destroy" which removes a managed cluster.
func newClusterDestroyCmd(format *output.Format, reg *cluster.Registry) *cobra.Command {
	var name string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy a managed cluster (not allowed for externally-connected clusters)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return writeFailure(cmd, *format, "cluster destroy",
					fmt.Errorf("--confirm flag is required to destroy a cluster"))
			}

			entry, err := reg.Get(name)
			if err != nil {
				return writeFailure(cmd, *format, "cluster destroy", err)
			}

			if entry.Source == cluster.SourceExternal {
				return writeFailure(cmd, *format, "cluster destroy",
					fmt.Errorf("cluster %q was connected with 'cluster connect' and is not managed by pgdba; refusing to destroy", name))
			}

			if err := reg.Remove(name); err != nil {
				return writeFailure(cmd, *format, "cluster destroy",
					fmt.Errorf("remove from registry: %w", err))
			}

			resp := output.Success("cluster destroy", map[string]string{
				"cluster": name,
				"status":  "destroyed",
			})
			out, err := output.FormatResponse(resp, *format)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Cluster name")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm destruction of cluster")
	return cmd
}

// resolvePatroniURL returns the Patroni URL from flag or registry lookup.
func resolvePatroniURL(name, patroniURL string, reg *cluster.Registry) (string, error) {
	if patroniURL != "" {
		return patroniURL, nil
	}
	if name == "" {
		return "", fmt.Errorf("either --name or --patroni-url must be provided")
	}
	entry, err := reg.Get(name)
	if err != nil {
		return "", fmt.Errorf("cluster %q not found in registry: %w", name, err)
	}
	return entry.PatroniURL, nil
}

// buildStatusResult converts a ClusterStatus into a ClusterStatusResult.
func buildStatusResult(name string, cs *patroni.ClusterStatus) *ClusterStatusResult {
	result := &ClusterStatusResult{
		ClusterName: name,
		Healthy:     true,
	}
	for _, m := range cs.Members {
		result.Members = append(result.Members, clusterMember{
			Name:  m.Name,
			Role:  m.Role,
			State: string(m.State),
			Host:  m.Host,
			Port:  m.Port,
			Lag:   m.Lag,
		})
		if m.Role == "master" || m.Role == "primary" {
			result.Primary = m.Name
		} else {
			result.ReplicaCount++
		}
		if m.State != patroni.StateRunning {
			result.Healthy = false
		}
	}
	return result
}

// writeFailure emits a JSON failure response and returns the original error.
func writeFailure(cmd *cobra.Command, format output.Format, command string, err error) error {
	resp := output.Failure(command, err)
	out, fmtErr := output.FormatResponse(resp, format)
	if fmtErr != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), out)
	return err
}

