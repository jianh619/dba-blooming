package unit_test

import (
	"context"
	"testing"

	"github.com/luckyjian/pgdba/internal/provider"
)

func TestNodeRoleConstants(t *testing.T) {
	if provider.RolePrimary != "primary" {
		t.Errorf("expected RolePrimary='primary', got %q", provider.RolePrimary)
	}
	if provider.RoleStandby != "standby" {
		t.Errorf("expected RoleStandby='standby', got %q", provider.RoleStandby)
	}
	if provider.RoleEtcd != "etcd" {
		t.Errorf("expected RoleEtcd='etcd', got %q", provider.RoleEtcd)
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	_, err := provider.New("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown provider type")
	}
}

func TestNew_DockerProvider(t *testing.T) {
	p, err := provider.New("docker", nil)
	if err != nil {
		t.Fatalf("expected no error for docker provider, got: %v", err)
	}
	if p.Type() != "docker" {
		t.Errorf("expected type 'docker', got %q", p.Type())
	}
}

func TestDockerProvider_Type(t *testing.T) {
	p, _ := provider.New("docker", nil)
	if p.Type() != "docker" {
		t.Errorf("expected 'docker', got %q", p.Type())
	}
}

func TestDockerProvider_CreateNode_EmptyName(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.CreateNode(context.Background(), provider.NodeConfig{})
	if err == nil {
		t.Error("expected error for empty node name")
	}
}

func TestDockerProvider_ExecOnNode_EmptyID(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.ExecOnNode(context.Background(), "", []string{"ls"})
	if err == nil {
		t.Error("expected error for empty node id")
	}
}

func TestDockerProvider_ExecOnNode_EmptyCmd(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.ExecOnNode(context.Background(), "some-id", []string{})
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestNew_BaremetalNotImplemented(t *testing.T) {
	_, err := provider.New("baremetal", nil)
	if err == nil {
		t.Error("expected error for unimplemented baremetal provider")
	}
}

func TestNew_KubernetesNotImplemented(t *testing.T) {
	_, err := provider.New("kubernetes", nil)
	if err == nil {
		t.Error("expected error for unimplemented kubernetes provider")
	}
}

func TestDockerProvider_DestroyNode_EmptyID(t *testing.T) {
	p, _ := provider.New("docker", nil)
	err := p.DestroyNode(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty node id")
	}
}

func TestDockerProvider_GetNodeStatus_EmptyID(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.GetNodeStatus(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty node id")
	}
}

func TestDockerProvider_PartitionNode_EmptyID(t *testing.T) {
	p, _ := provider.New("docker", nil)
	err := p.PartitionNode(context.Background(), "", true)
	if err == nil {
		t.Error("expected error for empty node id")
	}
}

func TestDockerProvider_WithCustomNetwork(t *testing.T) {
	p, err := provider.New("docker", map[string]string{"network": "custom-net"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if p.Type() != "docker" {
		t.Errorf("expected type 'docker', got %q", p.Type())
	}
}
