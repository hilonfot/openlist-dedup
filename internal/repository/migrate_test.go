package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateTMDBSchema_FromOldToNew(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Step 1: Create a database with the OLD schema (inline UNIQUE on normalized_name)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tmdb_cache (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			normalized_name TEXT    NOT NULL UNIQUE,
			tmdb_id         INTEGER NOT NULL DEFAULT 0,
			media_type      TEXT    NOT NULL DEFAULT '',
			data            TEXT    NOT NULL DEFAULT '{}',
			updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_tmdb_cache_media_type ON tmdb_cache(media_type);
		INSERT INTO tmdb_cache (normalized_name, tmdb_id, media_type, data) VALUES ('Fight Club (1999)', 550, 'movie', '{"title":"Fight Club"}');
		INSERT INTO tmdb_cache (normalized_name, tmdb_id, media_type, data) VALUES ('Breaking Bad (2008)', 1396, 'tv', '{"name":"Breaking Bad"}');
	`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Verify old schema
	rawDB, _ := sql.Open("sqlite", dbPath)
	var oldSQL string
	rawDB.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='tmdb_cache'").Scan(&oldSQL)
	if !strings.Contains(oldSQL, "NOT NULL UNIQUE") {
		t.Fatal("test setup failed: expected old schema with single-column UNIQUE")
	}
	rawDB.Close()

	// Step 2: Open with our repository code (should trigger migration)
	r, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Verify new schema
	var newSQL string
	r.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='tmdb_cache'").Scan(&newSQL)
	if !strings.Contains(newSQL, "UNIQUE(normalized_name, media_type)") {
		t.Errorf("expected new composite UNIQUE, got:\n%s", newSQL)
	}

	// Verify data preserved
	var count int
	r.QueryRow("SELECT COUNT(*) FROM tmdb_cache").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}

	// Verify upsert works with new schema
	ctx := context.Background()
	err = r.SaveTMDBCache(ctx, "Fight Club (1999)", "movie", 550, `{"title":"Fight Club","updated":true}`)
	if err != nil {
		t.Fatalf("UPSERT after migration failed: %v", err)
	}

	// Verify the data was actually updated
	var data string
	r.QueryRow("SELECT data FROM tmdb_cache WHERE normalized_name='Fight Club (1999)' AND media_type='movie'").Scan(&data)
	if !strings.Contains(data, "updated") {
		t.Errorf("expected updated data, got: %s", data)
	}
}

func TestMigrateTMDBSchema_NewSchemaUnchanged(t *testing.T) {
	// Opening a fresh database should NOT trigger migration
	r := newTestDB(t)

	// Check schema is already correct
	var newSQL string
	r.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='tmdb_cache'").Scan(&newSQL)
	if !strings.Contains(newSQL, "UNIQUE(normalized_name, media_type)") {
		t.Errorf("expected composite UNIQUE from the start, got:\n%s", newSQL)
	}

	// Save should work immediately
	ctx := context.Background()
	err := r.SaveTMDBCache(ctx, "Test", "movie", 1, `{}`)
	if err != nil {
		t.Fatalf("UPSERT on fresh DB failed: %v", err)
	}
}

func TestMigrateTMDBSchema_MigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create old schema
	db, _ := sql.Open("sqlite", dbPath)
	db.Exec(`CREATE TABLE tmdb_cache (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		normalized_name TEXT NOT NULL UNIQUE,
		tmdb_id INTEGER NOT NULL DEFAULT 0,
		media_type TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL DEFAULT '{}',
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	db.Exec(`INSERT INTO tmdb_cache (normalized_name, tmdb_id, media_type, data) VALUES ('Test', 1, 'movie', '{}')`)
	db.Close()

	// First open (migrate)
	r1, _ := Open(dbPath)
	r1.Close()

	// Second open (should not error)
	r2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer r2.Close()

	var count int
	r2.QueryRow("SELECT COUNT(*) FROM tmdb_cache").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 entry after migration, got %d", count)
	}
}
