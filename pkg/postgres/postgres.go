// Package postgres provides a PostgreSQL connection pool factory and migration runner
// for Sentinel and Herald services.
//
// Each service creates a pool at startup:
//
//	pool, err := postgres.NewPool(ctx, postgres.Config{
//	    Host:     "localhost",
//	    Port:     5432,
//	    Database: "sentinel",
//	    User:     "sentinel",
//	    Password: "sentinel",
//	    MaxConns: 10,
//	})
//	defer pool.Close()
//
// The pool manages a set of reusable connections. Goroutines acquire a connection
// from the pool, execute queries, and release it back. This avoids the overhead
// of establishing a new TCP+TLS connection for every query.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds PostgreSQL connection parameters.
//
// These map to environment variables via the config package:
//
//	COLLECTOR_POSTGRES_HOST=localhost
//	COLLECTOR_POSTGRES_PORT=5432
//	etc.
type Config struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`

	// MaxConns is the maximum number of connections in the pool.
	// Default: 10. Set higher for write-heavy services (processor),
	// lower for read-light services (collector).
	MaxConns int32 `mapstructure:"max_conns"`

	// MinConns keeps this many idle connections ready.
	// Prevents latency spikes when traffic arrives after quiet periods.
	MinConns int32 `mapstructure:"min_conns"`
}

// DSN builds a PostgreSQL connection string from the config.
//
// Format: postgres://user:password@host:port/database
// This is the standard libpq connection URI format.
func (c Config) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		c.User, c.Password, c.Host, c.Port, c.Database,
	)
}

// NewPool creates a pgx connection pool.
//
// The pool is safe for concurrent use by multiple goroutines.
// Close it during graceful shutdown to release all connections.
//
// pgx is a native PostgreSQL driver (not database/sql) — it's faster
// and supports PostgreSQL-specific features like COPY, LISTEN/NOTIFY, and JSONB.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}

	// Verify the connection works before returning.
	// Fail fast: better to crash at startup than serve broken requests.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return pool, nil
}
