package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"
)

// DefaultCacheTTL is the default time-to-live for TMDB cache entries (24 hours).
const DefaultCacheTTL = 24 * time.Hour

// TMDBCacheEntry represents a row from the tmdb_cache table.
type TMDBCacheEntry struct {
	ID             int64
	NormalizedName string
	TMDBID         int64
	MediaType      string
	Data           string
	UpdatedAt      string
}

// GetTMDBCache retrieves a cached TMDB result. Returns nil if not found or
// the entry has expired (older than ttl).
func (db *DB) GetTMDBCache(ctx context.Context, normalizedName, mediaType string, ttl time.Duration) (*TMDBCacheEntry, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, normalized_name, tmdb_id, media_type, data, updated_at
		FROM tmdb_cache
		WHERE normalized_name = ? AND media_type = ?
	`, normalizedName, mediaType)

	var entry TMDBCacheEntry
	err := row.Scan(&entry.ID, &entry.NormalizedName, &entry.TMDBID, &entry.MediaType, &entry.Data, &entry.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query tmdb_cache: %w", err)
	}

	// Check TTL
	if ttl > 0 {
		updated, err := time.Parse("2006-01-02 15:04:05", entry.UpdatedAt)
		if err != nil {
			// If we can't parse the timestamp, treat as expired
			return nil, nil
		}
		if time.Since(updated) > ttl {
			return nil, nil
		}
	}

	return &entry, nil
}

// SaveTMDBCache upserts a TMDB cache entry.
func (db *DB) SaveTMDBCache(ctx context.Context, normalizedName, mediaType string, tmdbID int64, data string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO tmdb_cache (normalized_name, tmdb_id, media_type, data)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(normalized_name, media_type) DO UPDATE SET
			tmdb_id = excluded.tmdb_id,
			media_type = excluded.media_type,
			data = excluded.data,
			updated_at = datetime('now')
	`, normalizedName, tmdbID, mediaType, data)
	if err != nil {
		return fmt.Errorf("save tmdb_cache: %w", err)
	}
	return nil
}

// ClearExpiredTMDBCache removes cache entries older than ttl.
func (db *DB) ClearExpiredTMDBCache(ctx context.Context, ttl time.Duration) error {
	cutoff := time.Now().UTC().Add(-ttl).Format("2006-01-02 15:04:05")
	_, err := db.ExecContext(ctx, `
		DELETE FROM tmdb_cache WHERE updated_at < ?
	`, cutoff)
	if err != nil {
		return fmt.Errorf("clear expired tmdb_cache: %w", err)
	}
	return nil
}

// migrateTMDBSchema detects and migrates the tmdb_cache table from the old single-column
// UNIQUE(normalized_name) constraint to the new composite UNIQUE(normalized_name, media_type).
// This handles databases created before the schema change in commit 3940a4d.
func (db *DB) migrateTMDBSchema(ctx context.Context) {
	// Detect old schema: inline UNIQUE on column (NOT NULL UNIQUE) vs composite (UNIQUE(col1, col2))
	var oldSchema string
	err := db.QueryRowContext(ctx, `
		SELECT sql FROM sqlite_master
		WHERE type='table' AND name='tmdb_cache'
		  AND sql LIKE '%NOT NULL UNIQUE%'
	`).Scan(&oldSchema)
	if err == sql.ErrNoRows {
		return // already using new schema or table doesn't exist
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "TMDB schema check: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "TMDB: migrating tmdb_cache schema...\n")

	// Migrate: create new table with composite UNIQUE → copy data → drop old → rename
	_, execErr := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tmdb_cache_new (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			normalized_name TEXT    NOT NULL,
			tmdb_id         INTEGER NOT NULL DEFAULT 0,
			media_type      TEXT    NOT NULL DEFAULT '',
			data            TEXT    NOT NULL DEFAULT '{}',
			updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			UNIQUE(normalized_name, media_type)
		);
		INSERT INTO tmdb_cache_new (id, normalized_name, tmdb_id, media_type, data, updated_at)
			SELECT id, normalized_name, tmdb_id, media_type, data, updated_at FROM tmdb_cache;
		DROP TABLE tmdb_cache;
		ALTER TABLE tmdb_cache_new RENAME TO tmdb_cache;
	`)
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "TMDB schema migration failed: %v\n", execErr)
		return
	}
	fmt.Fprintf(os.Stderr, "TMDB: schema migration complete\n")
}
