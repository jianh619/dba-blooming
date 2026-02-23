package unit_test

import (
	"strings"
	"testing"

	"github.com/luckyjian/pgdba/internal/patroni"
)

func TestRenderPatroniConfig_ContainsClusterName(t *testing.T) {
	cfg := patroni.PatroniConfig{
		ClusterName:         "test-cluster",
		NodeName:            "pg-primary",
		Host:                "10.0.0.1",
		PGPort:              5432,
		DataDir:             "/var/lib/postgresql/data",
		EtcdHosts:           "10.0.0.1:2379",
		ReplicationPassword: "repl-pass",
		SuperuserPassword:   "super-pass",
		RewindPassword:      "rewind-pass",
	}
	out, err := patroni.RenderPatroniConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "test-cluster") {
		t.Error("rendered config should contain cluster name")
	}
	if !strings.Contains(out, "pg-primary") {
		t.Error("rendered config should contain node name")
	}
	if !strings.Contains(out, "10.0.0.1") {
		t.Error("rendered config should contain host")
	}
}

func TestRenderPatroniConfig_NoHardcodedPassword(t *testing.T) {
	cfg := patroni.PatroniConfig{
		ClusterName:         "prod-cluster",
		NodeName:            "pg-1",
		Host:                "10.0.0.1",
		PGPort:              5432,
		DataDir:             "/data",
		EtcdHosts:           "10.0.0.1:2379",
		ReplicationPassword: "my-repl-secret",
		SuperuserPassword:   "my-super-secret",
		RewindPassword:      "my-rewind-secret",
	}
	out, err := patroni.RenderPatroniConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Passwords must come from config variables, not hardcoded values.
	if !strings.Contains(out, "my-repl-secret") {
		t.Error("rendered config should contain replication password from config")
	}
	if !strings.Contains(out, "my-super-secret") {
		t.Error("rendered config should contain superuser password from config")
	}
	if !strings.Contains(out, "my-rewind-secret") {
		t.Error("rendered config should contain rewind password from config")
	}
}

func TestRenderEtcdConfig_ContainsNodeName(t *testing.T) {
	cfg := patroni.EtcdConfig{
		NodeName:       "etcd-1",
		Host:           "10.0.0.1",
		DataDir:        "/var/lib/etcd",
		ClusterName:    "test-cluster",
		InitialCluster: "etcd-1=http://10.0.0.1:2380",
	}
	out, err := patroni.RenderEtcdConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "etcd-1") {
		t.Error("rendered etcd config should contain node name")
	}
	if !strings.Contains(out, "10.0.0.1") {
		t.Error("rendered etcd config should contain host")
	}
}

func TestRenderPatroniConfig_ValidYAML(t *testing.T) {
	cfg := patroni.PatroniConfig{
		ClusterName: "c", NodeName: "n", Host: "127.0.0.1",
		PGPort: 5432, DataDir: "/d", EtcdHosts: "127.0.0.1:2379",
		ReplicationPassword: "r", SuperuserPassword: "s", RewindPassword: "w",
	}
	out, err := patroni.RenderPatroniConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	if !strings.Contains(out, "scope:") {
		t.Error("expected 'scope:' in rendered patroni config")
	}
	if !strings.Contains(out, "postgresql:") {
		t.Error("expected 'postgresql:' in rendered patroni config")
	}
}

func TestRenderPatroniConfig_ContainsPGPort(t *testing.T) {
	cfg := patroni.PatroniConfig{
		ClusterName:         "port-test",
		NodeName:            "pg-1",
		Host:                "10.0.0.1",
		PGPort:              5433,
		DataDir:             "/data",
		EtcdHosts:           "10.0.0.1:2379",
		ReplicationPassword: "r",
		SuperuserPassword:   "s",
		RewindPassword:      "w",
	}
	out, err := patroni.RenderPatroniConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "5433") {
		t.Error("rendered config should contain the PG port")
	}
}

func TestRenderPatroniConfig_ContainsDataDir(t *testing.T) {
	cfg := patroni.PatroniConfig{
		ClusterName:         "c",
		NodeName:            "n",
		Host:                "127.0.0.1",
		PGPort:              5432,
		DataDir:             "/custom/data/dir",
		EtcdHosts:           "127.0.0.1:2379",
		ReplicationPassword: "r",
		SuperuserPassword:   "s",
		RewindPassword:      "w",
	}
	out, err := patroni.RenderPatroniConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "/custom/data/dir") {
		t.Error("rendered config should contain data dir")
	}
}

func TestRenderEtcdConfig_ContainsInitialCluster(t *testing.T) {
	cfg := patroni.EtcdConfig{
		NodeName:       "etcd-2",
		Host:           "10.0.0.2",
		DataDir:        "/var/lib/etcd",
		ClusterName:    "test-cluster",
		InitialCluster: "etcd-1=http://10.0.0.1:2380,etcd-2=http://10.0.0.2:2380",
	}
	out, err := patroni.RenderEtcdConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "etcd-1=http://10.0.0.1:2380,etcd-2=http://10.0.0.2:2380") {
		t.Error("rendered etcd config should contain initial cluster string")
	}
}

func TestRenderEtcdConfig_ContainsClusterToken(t *testing.T) {
	cfg := patroni.EtcdConfig{
		NodeName:       "etcd-1",
		Host:           "10.0.0.1",
		DataDir:        "/data",
		ClusterName:    "my-pg-cluster",
		InitialCluster: "etcd-1=http://10.0.0.1:2380",
	}
	out, err := patroni.RenderEtcdConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The token should embed the cluster name.
	if !strings.Contains(out, "my-pg-cluster") {
		t.Error("rendered etcd config should reference cluster name in token")
	}
}

func TestRenderPatroniConfig_ContainsEtcdHosts(t *testing.T) {
	cfg := patroni.PatroniConfig{
		ClusterName:         "c",
		NodeName:            "n",
		Host:                "127.0.0.1",
		PGPort:              5432,
		DataDir:             "/d",
		EtcdHosts:           "10.0.0.1:2379,10.0.0.2:2379",
		ReplicationPassword: "r",
		SuperuserPassword:   "s",
		RewindPassword:      "w",
	}
	out, err := patroni.RenderPatroniConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "10.0.0.1:2379,10.0.0.2:2379") {
		t.Error("rendered config should contain etcd hosts")
	}
}
