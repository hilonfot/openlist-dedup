package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// MediaFile represents a row from the media_files table.
type MediaFile struct {
	ID        int64
	Storage   string
	Path      string
	Name      string
	Size      int64
	IsDir     bool
	Modified  string
	CreatedAt string
}

// QueryByPath retrieves a single file by storage + path.
func (db *DB) QueryByPath(ctx context.Context, storage, path string) (*MediaFile, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, storage, path, name, size, is_dir, modified, created_at
		FROM media_files
		WHERE storage = ? AND path = ?
	`, storage, path)

	return scanMediaFile(row)
}

// QueryByName retrieves files matching the given name (LIKE match).
func (db *DB) QueryByName(ctx context.Context, name string) ([]MediaFile, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage, path, name, size, is_dir, modified, created_at
		FROM media_files
		WHERE name LIKE ?
		ORDER BY name
	`, "%"+name+"%")
	if err != nil {
		return nil, fmt.Errorf("query by name: %w", err)
	}
	defer rows.Close()

	return scanMediaFiles(rows)
}

// QueryByStorage retrieves all files for a given storage.
func (db *DB) QueryByStorage(ctx context.Context, storage string) ([]MediaFile, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage, path, name, size, is_dir, modified, created_at
		FROM media_files
		WHERE storage = ?
		ORDER BY path
	`, storage)
	if err != nil {
		return nil, fmt.Errorf("query by storage: %w", err)
	}
	defer rows.Close()

	return scanMediaFiles(rows)
}

// QueryAllFiles returns all non-directory media files.
func (db *DB) QueryAllFiles(ctx context.Context) ([]MediaFile, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage, path, name, size, is_dir, modified, created_at
		FROM media_files
		WHERE is_dir = 0
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query all files: %w", err)
	}
	defer rows.Close()

	return scanMediaFiles(rows)
}

// CountFiles returns the total number of media file entries.
func (db *DB) CountFiles(ctx context.Context) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_files`).Scan(&count)
	return count, err
}

// scanMediaFile scans a single row into MediaFile.
func scanMediaFile(s interface {
	Scan(dest ...interface{}) error
}) (*MediaFile, error) {
	var m MediaFile
	var isDir int
	err := s.Scan(&m.ID, &m.Storage, &m.Path, &m.Name, &m.Size, &isDir, &m.Modified, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan media_file: %w", err)
	}
	m.IsDir = isDir == 1
	return &m, nil
}

// scanMediaFiles scans all rows into a MediaFile slice.
func scanMediaFiles(rows *sql.Rows) ([]MediaFile, error) {
	var results []MediaFile
	for rows.Next() {
		var m MediaFile
		var isDir int
		if err := rows.Scan(&m.ID, &m.Storage, &m.Path, &m.Name, &m.Size, &isDir, &m.Modified, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan media_file row: %w", err)
		}
		m.IsDir = isDir == 1
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return results, nil
}
