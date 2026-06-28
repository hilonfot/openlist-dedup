package repository

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// BatchMaxSize is the number of rows that triggers a flush.
	BatchMaxSize = 1000
	// BatchMaxAge is the maximum time before a forced flush.
	BatchMaxAge = 5 * time.Second
)

// MediaRow represents a row in the media_files table.
type MediaRow struct {
	Storage  string
	Path     string
	Name     string
	Size     int64
	IsDir    bool
	Modified string
}

// BatchInserter accumulates rows and flushes them to the database in bulk
// transactions. It is safe for concurrent use.
type BatchInserter struct {
	db       *DB
	onFlush  func(int) // callback with flush count

	mu       sync.Mutex
	buf      []MediaRow
	lastFlux time.Time
}

// NewBatchInserter creates a BatchInserter backed by the given DB.
func NewBatchInserter(db *DB) *BatchInserter {
	return &BatchInserter{
		db:       db,
		buf:      make([]MediaRow, 0, BatchMaxSize),
		lastFlux: time.Now(),
	}
}

// OnFlush sets a callback that is invoked after each successful flush with
// the number of rows written.
func (b *BatchInserter) OnFlush(fn func(count int)) {
	b.onFlush = fn
}

// Insert adds a row to the buffer and flushes automatically if the buffer
// reaches BatchMaxSize.
func (b *BatchInserter) Insert(ctx context.Context, row MediaRow) error {
	b.mu.Lock()
	b.buf = append(b.buf, row)
	shouldFlush := len(b.buf) >= BatchMaxSize
	b.mu.Unlock()

	if shouldFlush {
		return b.Flush(ctx)
	}
	return nil
}

// Flush writes all buffered rows to the database in a single transaction.
func (b *BatchInserter) Flush(ctx context.Context) error {
	b.mu.Lock()
	if len(b.buf) == 0 {
		b.lastFlux = time.Now()
		b.mu.Unlock()
		return nil
	}
	batch := b.buf
	b.buf = make([]MediaRow, 0, BatchMaxSize)
	b.lastFlux = time.Now()
	b.mu.Unlock()

	if err := b.flushBatch(ctx, batch); err != nil {
		// Write failed: return the rows to the buffer so they are retried on the
		// next flush instead of being silently lost.
		b.mu.Lock()
		b.buf = append(batch, b.buf...)
		b.mu.Unlock()
		return err
	}

	if b.onFlush != nil {
		b.onFlush(len(batch))
	}
	return nil
}

// flushBatch executes a batch INSERT OR IGNORE within a transaction.
func (b *BatchInserter) flushBatch(ctx context.Context, batch []MediaRow) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO media_files (storage, path, name, size, is_dir, modified)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, row := range batch {
		isDir := 0
		if row.IsDir {
			isDir = 1
		}
		if _, err := stmt.ExecContext(ctx, row.Storage, row.Path, row.Name, row.Size, isDir, row.Modified); err != nil {
			return fmt.Errorf("insert row: %w", err)
		}
	}

	return tx.Commit()
}

// Len returns the current number of buffered rows.
func (b *BatchInserter) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

// FlushLoop runs a background goroutine that periodically flushes the buffer
// based on BatchMaxAge. It stops when ctx is cancelled. Call this after
// creating the BatchInserter.
func (b *BatchInserter) FlushLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(BatchMaxAge)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Final flush before exiting
				b.Flush(context.Background())
				return
			case <-ticker.C:
				b.mu.Lock()
				elapsed := time.Since(b.lastFlux)
				shouldFlush := len(b.buf) > 0 && elapsed >= BatchMaxAge
				b.mu.Unlock()
				if shouldFlush {
					b.Flush(ctx)
				}
			}
		}
	}()
}
