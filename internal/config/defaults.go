package config

const (
	DefaultConfigPath = "~/.pgdba/config.yaml"
	DefaultPGPort     = 5432
	DefaultSSLMode    = "prefer"
	DefaultProvider   = "docker"
	DefaultPGUser     = "postgres"
	DefaultPGDatabase = "postgres"
)

var validProviders = map[string]bool{
	"docker":     true,
	"baremetal":  true,
	"kubernetes": true,
}
