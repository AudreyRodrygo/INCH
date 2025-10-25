package postgres

import (
	"context"
	"database/sql"
	"fmt"

	// Register pgx as a database/sql driver.
	// goose requires database/sql, so we use pgx's compatibility layer.
	// The underscore import means "import for side effects only" —
	// it registers the driver without us using the package directly.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrate runs all pending database migrations from the given directory.
//
// Migrations are SQL files named with sequential numbers:
//
//	migrations/
//	  001_create_events.sql
//	  002_add_indexes.sql
//
// goose tracks which migrations have been applied in a `goose_db_version` table.
// Only unapplied migrations are executed — it's safe to call on every startup.
//
// The dsn parameter is a PostgreSQL connection string (from Config.DSN()).
func Migrate(ctx context.Context, dsn, migrationsDir string) error {
	// goose works with database/sql, not pgx directly.
	// pgx/stdlib provides a database/sql-compatible driver.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("opening database for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Set the dialect so goose generates PostgreSQL-compatible SQL.
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	// Run all pending migrations.
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
