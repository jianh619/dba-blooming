package unit_test

import (
	"strings"
	"testing"

	"github.com/luckyjian/pgdba/internal/postgres"
)

func TestPGConfig_DSN_NoPassword(t *testing.T) {
	cfg := postgres.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Database: "postgres",
		SSLMode:  "prefer",
	}
	dsn := cfg.DSN("REDACTED")
	if dsn == "" {
		t.Error("expected non-empty DSN")
	}
	if !strings.Contains(dsn, "localhost") {
		t.Error("DSN should contain host")
	}
	if !strings.Contains(dsn, "5432") {
		t.Error("DSN should contain port")
	}
}

func TestPGConfig_DSN_ContainsUser(t *testing.T) {
	cfg := postgres.Config{
		Host:     "db.example.com",
		Port:     5433,
		User:     "admin",
		Database: "mydb",
		SSLMode:  "require",
	}
	dsn := cfg.DSN("secret")
	if !strings.Contains(dsn, "admin") {
		t.Errorf("DSN should contain user 'admin', got: %s", dsn)
	}
	if !strings.Contains(dsn, "mydb") {
		t.Errorf("DSN should contain database 'mydb', got: %s", dsn)
	}
	if !strings.Contains(dsn, "require") {
		t.Errorf("DSN should contain sslmode 'require', got: %s", dsn)
	}
}

func TestPGConfig_DSN_PasswordInjected(t *testing.T) {
	cfg := postgres.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Database: "postgres",
		SSLMode:  "prefer",
	}
	// Caller controls the password passed in; it should appear in DSN.
	dsn := cfg.DSN("mysecret")
	if !strings.Contains(dsn, "mysecret") {
		t.Errorf("DSN should contain the provided password, got: %s", dsn)
	}
}

func TestPGConfig_DSN_EmptyPassword(t *testing.T) {
	cfg := postgres.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Database: "postgres",
		SSLMode:  "prefer",
	}
	// An empty password is valid (trust auth); DSN must still be non-empty.
	dsn := cfg.DSN("")
	if dsn == "" {
		t.Error("DSN with empty password must still be non-empty")
	}
}
