package repository

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Schema SQL statements.
const (
	schemaMediaFiles = `
CREATE TABLE IF NOT EXISTS media_files (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    storage   TEXT    NOT NULL,
    path      TEXT    NOT NULL,
    name      TEXT    NOT NULL,
    size      INTEGER NOT NULL DEFAULT 0,
    is_dir    INTEGER NOT NULL DEFAULT 0,
    modified  TEXT    NOT NULL DEFAULT '',
    created_at TEXT   NOT NULL DEFAULT (datetime('now')),
    UNIQUE(storage, path)
);
CREATE INDEX IF NOT EXISTS idx_media_files_name    ON media_files(name);
CREATE INDEX IF NOT EXISTS idx_media_files_storage ON media_files(storage);
`

	schemaScanTasks = `
CREATE TABLE IF NOT EXISTS scan_tasks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    storage    TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    status     TEXT    NOT NULL DEFAULT 'pending',
    updated_at TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(storage, path)
);
CREATE INDEX IF NOT EXISTS idx_scan_tasks_status ON scan_tasks(status);
`

	schemaTMDBCache = `
CREATE TABLE IF NOT EXISTS tmdb_cache (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    normalized_name TEXT    NOT NULL,
    tmdb_id         INTEGER NOT NULL DEFAULT 0,
    media_type      TEXT    NOT NULL DEFAULT '',
    data            TEXT    NOT NULL DEFAULT '{}',
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(normalized_name, media_type)
);
CREATE INDEX IF NOT EXISTS idx_tmdb_cache_media_type ON tmdb_cache(media_type);
`
)

// DB wraps a *sql.DB with schema initialization.
type DB struct {
	*sql.DB
}

// Open opens or creates the SQLite database at the given path and applies
// the schema. The path ":memory:" creates an in-memory database.
func Open(path string) (*DB, error) {
	// Apply pragmas via the DSN so they take effect on *every* pooled
	// connection. busy_timeout/synchronous/foreign_keys are per-connection
	// settings; running them once via db.Exec would only configure whichever
	// connection happened to serve that call, leaving later connections with
	// SQLite defaults (busy_timeout=0 → spurious SQLITE_BUSY under concurrency).
	const pragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"

	memory := path == ":memory:"
	dsn := "file:" + path + "?" + pragmas
	if memory {
		dsn = "file::memory:?" + pragmas
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if memory {
		// An in-memory database lives entirely within a single connection;
		// pin the pool to one connection so every query observes the same DB.
		db.SetMaxOpenConns(1)
	}

	w := &DB{DB: db}
	if err := w.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return w, nil
}

// initSchema creates all tables and indexes.
func (db *DB) initSchema(ctx context.Context) error {
	for _, stmt := range []string{schemaMediaFiles, schemaScanTasks, schemaTMDBCache} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec schema: %w", err)
		}
	}
	return nil
}

// Close shuts down the database.
func (db *DB) Close() error {
	return db.DB.Close()
}

// Stats returns database statistics.
func (db *DB) Stats() sql.DBStats {
	return db.DB.Stats()
}
