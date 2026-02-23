package provider

import (
	"context"
	"fmt"
)

// NodeRole identifies the role of a PostgreSQL cluster node.
type NodeRole string

const (
	RolePrimary NodeRole = "primary"
	RoleStandby NodeRole = "standby"
	RoleEtcd    NodeRole = "etcd"
)

// NodeStatus describes the runtime state of a single cluster node.
type NodeStatus struct {
	ID      string
	Host    string
	Role    NodeRole
	Running bool
	Healthy bool
}

// NodeConfig describes a new node to be provisioned.
type NodeConfig struct {
	Name    string
	Role    NodeRole
	Host    string
	DataDir string
	Port    int
	Labels  map[string]string
}

// Provider is the abstraction for infrastructure backends (Docker, baremetal, Kubernetes).
type Provider interface {
	CreateNode(ctx context.Context, cfg NodeConfig) (NodeStatus, error)
	DestroyNode(ctx context.Context, id string) error
	ExecOnNode(ctx context.Context, id string, cmd []string) (string, error)
	GetNodeStatus(ctx context.Context, id string) (NodeStatus, error)
	ListNodes(ctx context.Context) ([]NodeStatus, error)
	PartitionNode(ctx context.Context, id string, isolate bool) error
	Type() string
}

// New returns a Provider implementation for the given type.
func New(providerType string, cfg map[string]string) (Provider, error) {
	switch providerType {
	case "docker":
		return newDockerProvider(cfg), nil
	case "baremetal":
		return nil, fmt.Errorf("baremetal provider not yet implemented")
	case "kubernetes":
		return nil, fmt.Errorf("kubernetes provider not yet implemented")
	default:
		return nil, fmt.Errorf(
			"unknown provider type %q: must be docker, baremetal, or kubernetes",
			providerType,
		)
	}
}
