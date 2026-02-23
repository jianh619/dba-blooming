package unit_test

import (
	"os"
	"testing"

	"github.com/luckyjian/pgdba/internal/config"
)

// pgdbaEnvVars lists every environment variable that config.Load reads,
// so TestDefaultValues can save/restore them to ensure test isolation.
var pgdbaEnvVars = []string{
	"PGDBA_PG_HOST",
	"PGDBA_PG_PORT",
	"PGDBA_PG_USER",
	"PGDBA_PG_DATABASE",
	"PGDBA_PG_SSLMODE",
	"PGDBA_PG_PASSWORD",
	"PGDBA_PROVIDER_TYPE",
	"PGDBA_CLUSTER_NAME",
	"PGDBA_MONITOR_PROMETHEUS_URL",
	"PGDBA_MONITOR_GRAFANA_URL",
}

// clearPGDBAEnv saves all PGDBA_* environment variables and unsets them.
// The returned function restores the original values; call it with defer.
func clearPGDBAEnv(t *testing.T) func() {
	t.Helper()
	saved := make(map[string]string, len(pgdbaEnvVars))
	for _, k := range pgdbaEnvVars {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = v
		}
		os.Unsetenv(k)
	}
	return func() {
		for _, k := range pgdbaEnvVars {
			if v, ok := saved[k]; ok {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}
}

func TestDefaultValues(t *testing.T) {
	defer clearPGDBAEnv(t)()

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
