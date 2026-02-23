package pgbouncer

import (
	"fmt"
	"strings"
)

// Config holds PgBouncer configuration parameters.
type Config struct {
	ListenAddr      string     // default 0.0.0.0
	ListenPort      int        // default 6432
	AuthFile        string     // path to userlist.txt
	AdminUsers      string     // admin user list
	StatsUsers      string     // stats user list
	PoolMode        string     // transaction (OLTP recommended) | session | statement
	MaxClientConn   int        // maximum client connections
	DefaultPoolSize int        // pool size per database/user pair
	Databases       []Database // database entries
}

// Database represents a single pgbouncer database entry.
type Database struct {
	Name   string // alias used in pgbouncer
	Host   string // backend PostgreSQL host
	Port   int    // backend PostgreSQL port
	DBName string // actual database name
}

// RenderConfig generates pgbouncer.ini content from the given Config.
func RenderConfig(cfg Config) string {
	var b strings.Builder

	b.WriteString("[databases]\n")
	for _, db := range cfg.Databases {
		port := db.Port
		if port == 0 {
			port = 5432
		}
		fmt.Fprintf(&b, "%s = host=%s port=%d dbname=%s\n",
			db.Name, db.Host, port, db.DBName)
	}

	b.WriteString("\n[pgbouncer]\n")

	listenAddr := cfg.ListenAddr
	if listenAddr == "" {
		listenAddr = "0.0.0.0"
	}
	fmt.Fprintf(&b, "listen_addr = %s\n", listenAddr)
	fmt.Fprintf(&b, "listen_port = %d\n", cfg.ListenPort)

	poolMode := cfg.PoolMode
	if poolMode == "" {
		poolMode = "transaction"
	}
	fmt.Fprintf(&b, "pool_mode = %s\n", poolMode)

	fmt.Fprintf(&b, "max_client_conn = %d\n", cfg.MaxClientConn)
	fmt.Fprintf(&b, "default_pool_size = %d\n", cfg.DefaultPoolSize)

	if cfg.AuthFile != "" {
		fmt.Fprintf(&b, "auth_file = %s\n", cfg.AuthFile)
	}
	if cfg.AdminUsers != "" {
		fmt.Fprintf(&b, "admin_users = %s\n", cfg.AdminUsers)
	}
	if cfg.StatsUsers != "" {
		fmt.Fprintf(&b, "stats_users = %s\n", cfg.StatsUsers)
	}

	return b.String()
}

// RenderUserlist generates userlist.txt content from a username-to-hash map.
// Each line is formatted as: "username" "password_hash"
func RenderUserlist(users map[string]string) string {
	var b strings.Builder
	for user, hash := range users {
		fmt.Fprintf(&b, "%q %q\n", user, hash)
	}
	return b.String()
}
