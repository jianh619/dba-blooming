package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all tool-wide configuration. Password is intentionally absent;
// it is read exclusively from the PGDBA_PG_PASSWORD environment variable at
// connection time.
type Config struct {
	Cluster  ClusterConfig  `yaml:"cluster"  mapstructure:"cluster"`
	Provider ProviderConfig `yaml:"provider" mapstructure:"provider"`
	PG       PGConfig       `yaml:"pg"       mapstructure:"pg"`
	Monitor  MonitorConfig  `yaml:"monitor"  mapstructure:"monitor"`
}

// ClusterConfig holds cluster-level metadata.
type ClusterConfig struct {
	Name string `yaml:"name" mapstructure:"name"`
}

// ProviderConfig specifies the infrastructure provider.
type ProviderConfig struct {
	Type string `yaml:"type" mapstructure:"type"`
}

// PGConfig holds PostgreSQL connection parameters. No Password field — passwords
// are read only from os.Getenv("PGDBA_PG_PASSWORD") to prevent secret leakage.
type PGConfig struct {
	Host     string `yaml:"host"     mapstructure:"host"`
	Port     int    `yaml:"port"     mapstructure:"port"`
	User     string `yaml:"user"     mapstructure:"user"`
	Database string `yaml:"database" mapstructure:"database"`
	SSLMode  string `yaml:"sslmode"  mapstructure:"sslmode"`
}

// MonitorConfig holds optional monitoring integration endpoints.
type MonitorConfig struct {
	PrometheusURL string `yaml:"prometheus_url" mapstructure:"prometheus_url"`
	GrafanaURL    string `yaml:"grafana_url"    mapstructure:"grafana_url"`
}

// Load reads configuration from an optional file and environment variables.
// When cfgFile is empty, only defaults and environment variables are used.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set default values.
	v.SetDefault("pg.port", DefaultPGPort)
	v.SetDefault("pg.sslmode", DefaultSSLMode)
	v.SetDefault("pg.user", DefaultPGUser)
	v.SetDefault("pg.database", DefaultPGDatabase)
	v.SetDefault("provider.type", DefaultProvider)

	// Support environment variables with PGDBA_ prefix (e.g. PGDBA_PG_HOST → pg.host).
	// AutomaticEnv maps keys with "_" separator; we also bind each key explicitly so
	// that the PGDBA_PG_HOST environment variable correctly overrides pg.host.
	v.SetEnvPrefix("PGDBA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Explicit bindings ensure nested keys map correctly to environment variables.
	envBindings := map[string]string{
		"pg.host":               "PGDBA_PG_HOST",
		"pg.port":               "PGDBA_PG_PORT",
		"pg.user":               "PGDBA_PG_USER",
		"pg.database":           "PGDBA_PG_DATABASE",
		"pg.sslmode":            "PGDBA_PG_SSLMODE",
		"provider.type":         "PGDBA_PROVIDER_TYPE",
		"cluster.name":          "PGDBA_CLUSTER_NAME",
		"monitor.prometheus_url": "PGDBA_MONITOR_PROMETHEUS_URL",
		"monitor.grafana_url":   "PGDBA_MONITOR_GRAFANA_URL",
	}
	for key, envVar := range envBindings {
		if err := v.BindEnv(key, envVar); err != nil {
			return nil, fmt.Errorf("bind env %s: %w", envVar, err)
		}
	}

	// Optional config file.
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// Validate checks that the configuration is semantically correct.
func (c *Config) Validate() error {
	if !validProviders[c.Provider.Type] {
		return fmt.Errorf(
			"invalid provider type %q: must be one of docker, baremetal, kubernetes",
			c.Provider.Type,
		)
	}
	if c.PG.Port <= 0 || c.PG.Port > 65535 {
		return fmt.Errorf("invalid pg.port %d: must be between 1 and 65535", c.PG.Port)
	}
	return nil
}
