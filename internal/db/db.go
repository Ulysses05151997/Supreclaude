// Package db opens the SQLite database, applies pragmas suited to a small
// multi-user LAN app, and runs embedded migrations on startup.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open opens (creating if needed) the SQLite database at path, sets pragmas,
// and runs any pending migrations. The returned *sql.DB serializes writes
// (MaxOpenConns=1) while WAL keeps reads concurrent — the simplest robust
// pattern for a handful of office users.
func Open(path string) (*sql.DB, error) {
	// WAL + busy_timeout via the DSN so they apply to every connection.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// One connection => writes serialize, no "database is locked" surprises.
	conn.SetMaxOpenConns(1)

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return conn, nil
}

// migrate applies every migration file whose version is greater than the
// highest already recorded in schema_migrations, in ascending order.
func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	var current int
	if err := conn.QueryRow(
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&current); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}

	type migration struct {
		version int
		name    string
	}
	var pending []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		// File names look like "0001_init.sql"; the leading number is the version.
		numPart, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			return fmt.Errorf("malformed migration filename %q (want NNNN_name.sql)", e.Name())
		}
		v, err := strconv.Atoi(numPart)
		if err != nil {
			return fmt.Errorf("migration %q has non-numeric version: %w", e.Name(), err)
		}
		if v > current {
			pending = append(pending, migration{version: v, name: e.Name()})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].version < pending[j].version })

	for _, m := range pending {
		sqlBytes, err := migrationFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying %s: %w", m.name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version) VALUES (?)`, m.version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording %s: %w", m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing %s: %w", m.name, err)
		}
	}
	return nil
}

// BackupTo writes a consistent snapshot of the live database to dest using
// SQLite's VACUUM INTO. Safe to run while the app is serving — avoids the
// WAL/-shm corruption risk of copying the file directly.
func BackupTo(conn *sql.DB, dest string) error {
	// VACUUM INTO requires the destination not already exist.
	if _, err := conn.Exec(`VACUUM INTO ?`, dest); err != nil {
		return fmt.Errorf("VACUUM INTO %q: %w", dest, err)
	}
	return nil
}
