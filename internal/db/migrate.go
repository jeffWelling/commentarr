package db

import (
	"database/sql"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	sqliteDriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Migrate applies every pending up migration in migrationsDir to d.
// Idempotent: running twice is a no-op.
func Migrate(d *sql.DB, migrationsDir string) error {
	drv, err := sqliteDriver.WithInstance(d, &sqliteDriver.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
