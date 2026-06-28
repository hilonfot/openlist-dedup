package repository

import (
	"context"
	"fmt"
)

// TaskStatus values for scan_tasks.
const (
	TaskStatusPending    = "pending"
	TaskStatusRunning    = "running"
	TaskStatusCompleted  = "completed"
	TaskStatusFailed     = "failed"
)

// ScanTask represents a row from the scan_tasks table.
type ScanTask struct {
	ID        int64
	Storage   string
	Path      string
	Status    string
	UpdatedAt string
}

// SaveTask inserts a new scan task. If a task with the same (storage, path)
// already exists, it resets the status to pending.
func (db *DB) SaveTask(ctx context.Context, storage, path string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO scan_tasks (storage, path, status)
		VALUES (?, ?, ?)
		ON CONFLICT(storage, path) DO UPDATE SET
			status = 'pending',
			updated_at = datetime('now')
	`, storage, path, TaskStatusPending)
	return err
}

// UpdateTaskStatus updates the status and timestamp of a scan task.
func (db *DB) UpdateTaskStatus(ctx context.Context, id int64, status string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE scan_tasks SET status = ?, updated_at = datetime('now') WHERE id = ?
	`, status, id)
	return err
}

// UpdateTaskStatusByPath updates the status for a given storage + path.
func (db *DB) UpdateTaskStatusByPath(ctx context.Context, storage, path, status string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE scan_tasks SET status = ?, updated_at = datetime('now')
		WHERE storage = ? AND path = ?
	`, status, storage, path)
	return err
}

// LoadPendingTasks returns all tasks that are not completed, ordered by
// creation time. This is used to resume interrupted scans.
func (db *DB) LoadPendingTasks(ctx context.Context) ([]ScanTask, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage, path, status, updated_at
		FROM scan_tasks
		WHERE status != ?
		ORDER BY id
	`, TaskStatusCompleted)
	if err != nil {
		return nil, fmt.Errorf("load pending tasks: %w", err)
	}
	defer rows.Close()

	var tasks []ScanTask
	for rows.Next() {
		var t ScanTask
		if err := rows.Scan(&t.ID, &t.Storage, &t.Path, &t.Status, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan scan_task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return tasks, nil
}

// LoadPendingTasksByStorage returns pending tasks for a specific storage.
func (db *DB) LoadPendingTasksByStorage(ctx context.Context, storage string) ([]ScanTask, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, storage, path, status, updated_at
		FROM scan_tasks
		WHERE status != ? AND storage = ?
		ORDER BY id
	`, TaskStatusCompleted, storage)
	if err != nil {
		return nil, fmt.Errorf("load pending tasks: %w", err)
	}
	defer rows.Close()

	var tasks []ScanTask
	for rows.Next() {
		var t ScanTask
		if err := rows.Scan(&t.ID, &t.Storage, &t.Path, &t.Status, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan scan_task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return tasks, nil
}

// HasUnfinishedTasks returns true if there are any non-completed scan tasks.
func (db *DB) HasUnfinishedTasks(ctx context.Context) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scan_tasks WHERE status != ?
	`, TaskStatusCompleted).Scan(&count)
	return count > 0, err
}

// DeleteCompletedTasks removes all completed tasks older than the given
// duration (in seconds).
func (db *DB) DeleteCompletedTasks(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM scan_tasks WHERE status = ?
	`, TaskStatusCompleted)
	return err
}
