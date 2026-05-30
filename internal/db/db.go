package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// migrationFn is a Go function for migrations too complex to express in a
// single SQL string. It runs inside the same transaction as the schema_version
// update, so it is applied atomically.
type migrationFn func(ctx context.Context, tx *sql.Tx) error

// DB wraps a SQLite connection with migration support.
type DB struct {
	sql  *sql.DB
	path string
}

// Open opens the SQLite database at path and runs all pending migrations.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1) // SQLite is single-writer

	db := &DB{sql: conn, path: path}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error { return db.sql.Close() }

// SQL returns the underlying *sql.DB for packages that need direct access.
func (db *DB) SQL() *sql.DB { return db.sql }

// Path returns the filesystem path of the database file.
func (db *DB) Path() string { return db.path }

// Snapshot creates a consistent point-in-time copy of the database at destPath.
// Safe to call while the database is open and in use (uses SQLite VACUUM INTO).
func (db *DB) Snapshot(ctx context.Context, destPath string) error {
	_, err := db.sql.ExecContext(ctx, "VACUUM INTO ?", destPath)
	return err
}

// SchemaVersion returns the current migration version.
func (db *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := db.sql.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&v)
	return v, err
}

// Stats returns row counts for the main tables.
type Stats struct {
	Nodes, Favorites, Users, Configs int
}

func (db *DB) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	for _, r := range []struct {
		table string
		dest  *int
	}{
		{"nodes", &s.Nodes},
		{"favorites", &s.Favorites},
		{"users", &s.Users},
		{"configs", &s.Configs},
	} {
		if err := db.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+r.table).Scan(r.dest); err != nil {
			return s, err
		}
	}
	return s, nil
}

func (db *DB) migrate() error {
	ctx := context.Background()

	_, err := db.sql.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		return err
	}

	var current int
	_ = db.sql.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current)

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("migration %d: begin: %w", m.version, err)
		}
		if m.fn != nil {
			if err := m.fn(ctx, tx); err != nil {
				tx.Rollback()
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
		} else if m.sql != "" {
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				tx.Rollback()
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d: commit: %w", m.version, err)
		}
	}
	return nil
}

type migration struct {
	version int
	sql     string
	fn      migrationFn
}

var migrations = []migration{
	{version: 1, sql: `
		CREATE TABLE IF NOT EXISTS nodes (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			name         TEXT NOT NULL,
			node_number  TEXT NOT NULL,
			ami_host     TEXT NOT NULL DEFAULT 'localhost',
			ami_port     INTEGER NOT NULL DEFAULT 5038,
			ami_user     TEXT NOT NULL DEFAULT 'admin',
			ami_pass     TEXT NOT NULL DEFAULT '',
			enabled      INTEGER NOT NULL DEFAULT 1,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS favorites (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id      INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			node_number  TEXT NOT NULL,
			callsign     TEXT,
			description  TEXT,
			location     TEXT,
			cmd          TEXT,
			sort_order   INTEGER NOT NULL DEFAULT 0,
			group_name   TEXT NOT NULL DEFAULT 'default',
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS users (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			username     TEXT UNIQUE NOT NULL,
			password     TEXT NOT NULL,
			permission   TEXT NOT NULL DEFAULT 'readonly',
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS configs (
			key        TEXT PRIMARY KEY,
			value      TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS tls_certs (
			id           INTEGER PRIMARY KEY,
			cert_pem     TEXT,
			key_pem      TEXT,
			generated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`},
	{version: 2, sql: `ALTER TABLE favorites ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
	     UPDATE favorites SET position = id WHERE position = 0;`},
	{version: 3, sql: `ALTER TABLE nodes ADD COLUMN description TEXT NOT NULL DEFAULT '';
	     ALTER TABLE nodes ADD COLUMN location TEXT NOT NULL DEFAULT '';`},
	{version: 4, sql: `ALTER TABLE users ADD COLUMN full_name TEXT NOT NULL DEFAULT '';
	     ALTER TABLE users ADD COLUMN avatar_url TEXT NOT NULL DEFAULT '';`},
	{version: 5, sql: `ALTER TABLE users ADD COLUMN tailscale_usernames TEXT NOT NULL DEFAULT '';`},
	{version: 6, fn: migration6},
	{version: 7, sql: `CREATE TABLE IF NOT EXISTS stats_cache (
		node_number TEXT PRIMARY KEY,
		stats_json  TEXT NOT NULL,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	)`},
	{version: 8, sql: `CREATE TABLE IF NOT EXISTS qrz_cache (
		callsign    TEXT PRIMARY KEY,
		record_json TEXT NOT NULL,
		fetched_at  DATETIME NOT NULL
	)`},
}
