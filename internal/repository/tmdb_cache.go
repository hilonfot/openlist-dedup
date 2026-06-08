package repository

import (
	"context"
	"database/sql"
	"fmt"
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
		ON CONFLICT(normalized_name) DO UPDATE SET
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
