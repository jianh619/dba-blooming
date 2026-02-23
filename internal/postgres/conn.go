package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

// Config holds PostgreSQL connection parameters. No Password field — passwords
// are read exclusively from the PGDBA_PG_PASSWORD environment variable to
// prevent accidental secret leakage through config files or logs.
type Config struct {
	Host     string
	Port     int
	User     string
	Database string
	SSLMode  string
}

// DSN returns a libpq-style connection string with the supplied password.
// Callers should obtain the password via Password() rather than hardcoding it.
func (c Config) DSN(password string) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, password, c.Database, c.SSLMode,
	)
}

// Password reads the PostgreSQL password from the environment variable
// PGDBA_PG_PASSWORD. It never returns a hardcoded fallback.
func Password() string {
	return os.Getenv("PGDBA_PG_PASSWORD")
}

// Connect opens a new PostgreSQL connection. The password is read from the
// PGDBA_PG_PASSWORD environment variable.
func Connect(ctx context.Context, cfg Config) (*pgx.Conn, error) {
	dsn := cfg.DSN(Password())
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres %s:%d: %w", cfg.Host, cfg.Port, err)
	}
	return conn, nil
}

// Ping verifies that an existing connection is still alive.
func Ping(ctx context.Context, conn *pgx.Conn) error {
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}
