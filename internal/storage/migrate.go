package storage

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(dbPath string) error {
	// Create a separate connection for migrations to avoid interfering with the main connection
	migrateDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer migrateDB.Close()

	driver, err := sqlite.WithInstance(migrateDB, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("create sqlite driver: %w", err)
	}

	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", d, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}