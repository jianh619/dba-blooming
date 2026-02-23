package unit_test

import (
	"strings"
	"testing"

	"github.com/luckyjian/pgdba/internal/pgbouncer"
)

func TestRenderConfig_ContainsListenPort(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenAddr:      "0.0.0.0",
		ListenPort:      6432,
		PoolMode:        "transaction",
		MaxClientConn:   100,
		DefaultPoolSize: 20,
		AuthFile:        "/etc/pgbouncer/userlist.txt",
		AdminUsers:      "postgres",
		StatsUsers:      "postgres",
	}
	out := pgbouncer.RenderConfig(cfg)
	if !strings.Contains(out, "6432") {
		t.Error("rendered config should contain listen port 6432")
	}
}

func TestRenderConfig_TransactionPoolMode(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenPort:      6432,
		PoolMode:        "transaction",
		MaxClientConn:   500,
		DefaultPoolSize: 10,
	}
	out := pgbouncer.RenderConfig(cfg)
	if !strings.Contains(out, "transaction") {
		t.Error("rendered config should contain pool_mode = transaction")
	}
}

func TestRenderConfig_DatabaseEntry(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenPort:      6432,
		PoolMode:        "transaction",
		MaxClientConn:   100,
		DefaultPoolSize: 10,
		Databases: []pgbouncer.Database{
			{Name: "myapp", Host: "10.0.0.1", Port: 5432, DBName: "myapp_prod"},
		},
	}
	out := pgbouncer.RenderConfig(cfg)
	if !strings.Contains(out, "myapp") {
		t.Error("rendered config should contain database alias 'myapp'")
	}
	if !strings.Contains(out, "10.0.0.1") {
		t.Error("rendered config should contain database host")
	}
	if !strings.Contains(out, "myapp_prod") {
		t.Error("rendered config should contain actual database name")
	}
}

func TestRenderUserlist_Format(t *testing.T) {
	users := map[string]string{
		"postgres": "md5abc123",
		"app_user": "md5def456",
	}
	out := pgbouncer.RenderUserlist(users)
	if !strings.Contains(out, "postgres") {
		t.Error("userlist should contain 'postgres'")
	}
	if !strings.Contains(out, "md5abc123") {
		t.Error("userlist should contain password hash for postgres")
	}
	if !strings.Contains(out, "app_user") {
		t.Error("userlist should contain 'app_user'")
	}
}

func TestRenderConfig_DefaultValues(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenPort:      6432,
		MaxClientConn:   1000,
		DefaultPoolSize: 20,
	}
	out := pgbouncer.RenderConfig(cfg)
	// Should contain pgbouncer section header.
	if !strings.Contains(out, "[pgbouncer]") {
		t.Error("rendered config should contain '[pgbouncer]' section")
	}
	if !strings.Contains(out, "[databases]") {
		t.Error("rendered config should contain '[databases]' section")
	}
}

func TestRenderConfig_MaxClientConn(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenPort:      6432,
		PoolMode:        "session",
		MaxClientConn:   2000,
		DefaultPoolSize: 50,
	}
	out := pgbouncer.RenderConfig(cfg)
	if !strings.Contains(out, "2000") {
		t.Error("rendered config should contain max_client_conn = 2000")
	}
}

func TestRenderConfig_AuthFile(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenPort:      6432,
		PoolMode:        "transaction",
		MaxClientConn:   100,
		DefaultPoolSize: 10,
		AuthFile:        "/etc/pgbouncer/userlist.txt",
	}
	out := pgbouncer.RenderConfig(cfg)
	if !strings.Contains(out, "/etc/pgbouncer/userlist.txt") {
		t.Error("rendered config should contain auth_file path")
	}
}

func TestRenderUserlist_EmptyUsers(t *testing.T) {
	out := pgbouncer.RenderUserlist(map[string]string{})
	// Should not panic; return an empty or header-only string.
	if out == "" {
		// Accept empty output for empty users.
		return
	}
}

func TestRenderConfig_MultipleDatabases(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenPort:      6432,
		MaxClientConn:   100,
		DefaultPoolSize: 10,
		Databases: []pgbouncer.Database{
			{Name: "db1", Host: "10.0.0.1", Port: 5432, DBName: "database1"},
			{Name: "db2", Host: "10.0.0.2", Port: 5432, DBName: "database2"},
		},
	}
	out := pgbouncer.RenderConfig(cfg)
	if !strings.Contains(out, "db1") {
		t.Error("rendered config should contain 'db1'")
	}
	if !strings.Contains(out, "db2") {
		t.Error("rendered config should contain 'db2'")
	}
	if !strings.Contains(out, "database1") {
		t.Error("rendered config should contain 'database1'")
	}
	if !strings.Contains(out, "database2") {
		t.Error("rendered config should contain 'database2'")
	}
}

func TestRenderConfig_ListenAddr(t *testing.T) {
	cfg := pgbouncer.Config{
		ListenAddr:      "127.0.0.1",
		ListenPort:      6432,
		MaxClientConn:   100,
		DefaultPoolSize: 10,
	}
	out := pgbouncer.RenderConfig(cfg)
	if !strings.Contains(out, "127.0.0.1") {
		t.Error("rendered config should contain listen address 127.0.0.1")
	}
}
