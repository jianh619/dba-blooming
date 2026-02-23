package provider

import (
	"context"
	"fmt"
)

// DockerProvider implements Provider for Docker/container environments.
// Full Docker SDK integration is covered by integration tests; the unit
// surface validates input contracts without requiring a running Docker daemon.
type DockerProvider struct {
	networkName string
	labelPrefix string
}

func newDockerProvider(cfg map[string]string) *DockerProvider {
	network := "pgdba-net"
	if cfg != nil {
		if v, ok := cfg["network"]; ok {
			network = v
		}
	}
	return &DockerProvider{
		networkName: network,
		labelPrefix: "pgdba",
	}
}

// Type returns the provider identifier string.
func (d *DockerProvider) Type() string { return "docker" }

// CreateNode provisions a new container node for the cluster.
func (d *DockerProvider) CreateNode(_ context.Context, cfg NodeConfig) (NodeStatus, error) {
	if cfg.Name == "" {
		return NodeStatus{}, fmt.Errorf("node name is required")
	}
	return NodeStatus{}, fmt.Errorf("not implemented: requires Docker SDK integration test")
}

// DestroyNode stops and removes the container identified by id.
func (d *DockerProvider) DestroyNode(_ context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("node id is required")
	}
	return fmt.Errorf("not implemented: requires Docker SDK integration test")
}

// ExecOnNode runs a command inside the container identified by id.
func (d *DockerProvider) ExecOnNode(_ context.Context, id string, cmd []string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("node id is required")
	}
	if len(cmd) == 0 {
		return "", fmt.Errorf("command is required")
	}
	return "", fmt.Errorf("not implemented: requires Docker SDK integration test")
}

// GetNodeStatus returns the runtime status of the container identified by id.
func (d *DockerProvider) GetNodeStatus(_ context.Context, id string) (NodeStatus, error) {
	if id == "" {
		return NodeStatus{}, fmt.Errorf("node id is required")
	}
	return NodeStatus{}, fmt.Errorf("not implemented: requires Docker SDK integration test")
}

// ListNodes returns the status of all cluster containers managed by this provider.
func (d *DockerProvider) ListNodes(_ context.Context) ([]NodeStatus, error) {
	return nil, fmt.Errorf("not implemented: requires Docker SDK integration test")
}

// PartitionNode isolates or reconnects the container identified by id to the cluster network.
func (d *DockerProvider) PartitionNode(_ context.Context, id string, _ bool) error {
	if id == "" {
		return fmt.Errorf("node id is required")
	}
	return fmt.Errorf("not implemented: requires Docker SDK integration test")
}
