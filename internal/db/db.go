// Package db owns the SQLite connection and schema migrations.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open returns a *sql.DB connected to the given SQLite DSN. Use ":memory:"
// for tests, a file path like "/config/commentarr.db" for production.
func Open(dsn string) (*sql.DB, error) {
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dsn, err)
	}
	if _, err := d.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		d.Close()
		return nil, fmt.Errorf("pragmas: %w", err)
	}
	return d, nil
}
