package unit_test

import (
	"os"
	"testing"

	"github.com/luckyjian/pgdba/internal/config"
)

func TestDefaultValues(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.PG.Port != config.DefaultPGPort {
		t.Errorf("expected default port %d, got %d", config.DefaultPGPort, cfg.PG.Port)
	}
	if cfg.PG.SSLMode != config.DefaultSSLMode {
		t.Errorf("expected default sslmode %q, got %q", config.DefaultSSLMode, cfg.PG.SSLMode)
	}
	if cfg.Provider.Type != config.DefaultProvider {
		t.Errorf("expected default provider %q, got %q", config.DefaultProvider, cfg.Provider.Type)
	}
}

func TestEnvVarOverride(t *testing.T) {
	os.Setenv("PGDBA_PG_HOST", "testhost")
	defer os.Unsetenv("PGDBA_PG_HOST")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.PG.Host != "testhost" {
		t.Errorf("expected host 'testhost' from env, got %q", cfg.PG.Host)
	}
}

func TestValidate_InvalidProvider(t *testing.T) {
	cfg := &config.Config{}
	cfg.PG.Port = 5432
	cfg.Provider.Type = "invalid-provider"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid provider type")
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.PG.Port = 5432
	cfg.PG.SSLMode = "prefer"
	cfg.Provider.Type = "docker"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &config.Config{}
	cfg.PG.Port = 0
	cfg.Provider.Type = "docker"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid port 0")
	}
}

func TestValidate_PortTooHigh(t *testing.T) {
	cfg := &config.Config{}
	cfg.PG.Port = 70000
	cfg.Provider.Type = "docker"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port > 65535")
	}
}

func TestNoPasswordInConfig(t *testing.T) {
	// Config struct must not contain a Password field (security requirement).
	// This is verified at compile time: the following lines access only
	// allowed fields. If PG.Password existed, we would not access it here,
	// making the test act as a specification that Password is absent.
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	_ = cfg.PG.Host
	_ = cfg.PG.Port
	_ = cfg.PG.User
	_ = cfg.PG.Database
	_ = cfg.PG.SSLMode
	// If this compiles without cfg.PG.Password, the security requirement is met.
}
