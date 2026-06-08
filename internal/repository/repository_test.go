package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()

	// Use a unique temp file per test to avoid WAL locking issues
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_InMemory(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open :memory: failed: %v", err)
	}
	defer db.Close()

	count, err := db.CountFiles(context.Background())
	if err != nil {
		t.Fatalf("CountFiles failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files, got %d", count)
	}
}

func TestSchema_CreatesTables(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Verify all tables exist
	tables := []string{"media_files", "scan_tasks", "tmdb_cache"}
	for _, name := range tables {
		var count int
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&count)
		if err != nil {
			t.Fatalf("check table %s: %v", name, err)
		}
		if count != 1 {
			t.Errorf("table %s not found", name)
		}
	}
}

func TestSchema_CreatesIndexes(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	indexes := []string{
		"idx_media_files_name",
		"idx_media_files_storage",
		"idx_scan_tasks_status",
		"idx_tmdb_cache_media_type",
	}
	for _, name := range indexes {
		var count int
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name,
		).Scan(&count)
		if err != nil {
			t.Fatalf("check index %s: %v", name, err)
		}
		if count != 1 {
			t.Errorf("index %s not found", name)
		}
	}
}

func TestBatchInserter_InsertAndFlush(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	b := NewBatchInserter(db)
	for i := 0; i < 10; i++ {
		row := MediaRow{
			Storage:  "local",
			Path:     "/movies/file" + itoa(i) + ".mp4",
			Name:     "file" + itoa(i) + ".mp4",
			Size:     int64(1000 + i),
			IsDir:    false,
			Modified: "2024-01-01T00:00:00Z",
		}
		if err := b.Insert(ctx, row); err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	if b.Len() != 10 {
		t.Errorf("expected 10 buffered rows, got %d", b.Len())
	}

	if err := b.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if b.Len() != 0 {
		t.Errorf("expected 0 rows after flush, got %d", b.Len())
	}

	count, err := db.CountFiles(ctx)
	if err != nil {
		t.Fatalf("CountFiles failed: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 files, got %d", count)
	}
}

func TestBatchInserter_AutoFlushAtLimit(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	var flushCount int
	b := NewBatchInserter(db)
	b.OnFlush(func(count int) {
		flushCount++
	})

	// Insert BatchMaxSize rows — should auto-flush at BatchMaxSize
	for i := 0; i < BatchMaxSize; i++ {
		row := MediaRow{
			Storage:  "quark",
			Path:     "/q/file" + itoa(i) + ".mp4",
			Name:     "file" + itoa(i) + ".mp4",
			Size:     500,
			IsDir:    false,
			Modified: "2024-01-01",
		}
		if err := b.Insert(ctx, row); err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}
	}

	// The 1000th insert should trigger auto-flush, so we have 0 buffered
	remaining := b.Len()
	if remaining != 0 {
		t.Errorf("expected 0 rows after auto-flush, got %d", remaining)
	}

	if flushCount < 1 {
		t.Error("expected at least 1 auto-flush")
	}

	count, err := db.CountFiles(ctx)
	if err != nil {
		t.Fatalf("CountFiles failed: %v", err)
	}
	if count != BatchMaxSize {
		t.Errorf("expected %d files, got %d", BatchMaxSize, count)
	}
}

func TestBatchInserter_Dedup(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	b := NewBatchInserter(db)

	// Insert same row twice
	row := MediaRow{
		Storage:  "local",
		Path:     "/movies/unique.mp4",
		Name:     "unique.mp4",
		Size:     1000,
		IsDir:    false,
		Modified: "2024-01-01",
	}
	if err := b.Insert(ctx, row); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if err := b.Insert(ctx, row); err != nil {
		t.Fatalf("Insert duplicate failed: %v", err)
	}
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Only 1 row due to INSERT OR IGNORE
	count, err := db.CountFiles(ctx)
	if err != nil {
		t.Fatalf("CountFiles failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 file after dedup, got %d", count)
	}
}

func TestQueryByPath(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert a file
	b := NewBatchInserter(db)
	b.Insert(ctx, MediaRow{
		Storage:  "local",
		Path:     "/movies/test.mp4",
		Name:     "test.mp4",
		Size:     2048,
		IsDir:    false,
		Modified: "2024-06-01",
	})
	b.Flush(ctx)

	m, err := db.QueryByPath(ctx, "local", "/movies/test.mp4")
	if err != nil {
		t.Fatalf("QueryByPath failed: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil result")
	}
	if m.Name != "test.mp4" {
		t.Errorf("expected name test.mp4, got %s", m.Name)
	}
	if m.Size != 2048 {
		t.Errorf("expected size 2048, got %d", m.Size)
	}
	if m.IsDir != false {
		t.Errorf("expected is_dir false, got %v", m.IsDir)
	}
	if m.Storage != "local" {
		t.Errorf("expected storage local, got %s", m.Storage)
	}
}

func TestQueryByPath_NotFound(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	m, err := db.QueryByPath(ctx, "local", "/nonexistent")
	if err != nil {
		t.Fatalf("QueryByPath failed: %v", err)
	}
	if m != nil {
		t.Error("expected nil for not found")
	}
}

func TestQueryByName(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	b := NewBatchInserter(db)
	for _, name := range []string{"avatar.mp4", "avengers.mp4", "alien.mp4", "brave.mp4"} {
		b.Insert(ctx, MediaRow{
			Storage: "local", Path: "/movies/" + name,
			Name: name, Size: 1000, IsDir: false, Modified: "2024-01-01",
		})
	}
	b.Flush(ctx)

	results, err := db.QueryByName(ctx, "alien")
	if err != nil {
		t.Fatalf("QueryByName failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'alien', got %d", len(results))
	}
	if results[0].Name != "alien.mp4" {
		t.Errorf("expected alien.mp4, got %s", results[0].Name)
	}
}

func TestQueryByStorage(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	b := NewBatchInserter(db)
	rows := []MediaRow{
		{Storage: "local", Path: "/a.mp4", Name: "a.mp4", Size: 100},
		{Storage: "quark", Path: "/b.mp4", Name: "b.mp4", Size: 200},
		{Storage: "local", Path: "/c.mp4", Name: "c.mp4", Size: 300},
	}
	for _, r := range rows {
		b.Insert(ctx, r)
	}
	b.Flush(ctx)

	results, err := db.QueryByStorage(ctx, "local")
	if err != nil {
		t.Fatalf("QueryByStorage failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 local files, got %d", len(results))
	}
}

func TestScanTask_SaveAndUpdate(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if err := db.SaveTask(ctx, "local", "/movies"); err != nil {
		t.Fatalf("SaveTask failed: %v", err)
	}

	pending, err := db.LoadPendingTasks(ctx)
	if err != nil {
		t.Fatalf("LoadPendingTasks failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending task, got %d", len(pending))
	}
	if pending[0].Path != "/movies" {
		t.Errorf("expected path /movies, got %s", pending[0].Path)
	}
	if pending[0].Status != TaskStatusPending {
		t.Errorf("expected status pending, got %s", pending[0].Status)
	}

	if err := db.UpdateTaskStatus(ctx, pending[0].ID, TaskStatusRunning); err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	if err := db.UpdateTaskStatusByPath(ctx, "local", "/movies", TaskStatusCompleted); err != nil {
		t.Fatalf("UpdateTaskStatusByPath failed: %v", err)
	}

	pending, err = db.LoadPendingTasks(ctx)
	if err != nil {
		t.Fatalf("LoadPendingTasks failed: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending tasks, got %d", len(pending))
	}
}

func TestScanTask_Resume(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Simulate a partially completed scan
	tasks := []struct {
		storage string
		path    string
		status  string
	}{
		{"local", "/done", TaskStatusCompleted},
		{"local", "/pending1", TaskStatusPending},
		{"local", "/pending2", TaskStatusRunning},
		{"local", "/failed", TaskStatusFailed},
	}

	for _, tt := range tasks {
		if err := db.SaveTask(ctx, tt.storage, tt.path); err != nil {
			t.Fatalf("SaveTask(%s) failed: %v", tt.path, err)
		}
		// Update to desired status
		if err := db.UpdateTaskStatusByPath(ctx, tt.storage, tt.path, tt.status); err != nil {
			t.Fatalf("UpdateTaskStatus(%s) failed: %v", tt.path, err)
		}
	}

	pending, err := db.LoadPendingTasks(ctx)
	if err != nil {
		t.Fatalf("LoadPendingTasks failed: %v", err)
	}
	if len(pending) != 3 {
		t.Errorf("expected 3 unfinished tasks, got %d", len(pending))
	}

	hasPending, err := db.HasUnfinishedTasks(ctx)
	if err != nil {
		t.Fatalf("HasUnfinishedTasks failed: %v", err)
	}
	if !hasPending {
		t.Error("expected unfinished tasks")
	}
}

func TestScanTask_LoadByStorage(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.SaveTask(ctx, "local", "/a")
	db.SaveTask(ctx, "quark", "/b")
	db.SaveTask(ctx, "local", "/c")

	localTasks, err := db.LoadPendingTasksByStorage(ctx, "local")
	if err != nil {
		t.Fatalf("LoadPendingTasksByStorage failed: %v", err)
	}
	if len(localTasks) != 2 {
		t.Errorf("expected 2 local tasks, got %d", len(localTasks))
	}
}

func TestBatchInserter_FlushLoop(t *testing.T) {
	db := newTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	b := NewBatchInserter(db)
	b.FlushLoop(ctx)

	// Insert a few rows and wait for the ticker to flush
	for i := 0; i < 5; i++ {
		b.Insert(ctx, MediaRow{
			Storage: "local", Path: "/flush/file" + itoa(i) + ".mp4",
			Name: "file" + itoa(i) + ".mp4", Size: 100, IsDir: false,
			Modified: "2024-01-01",
		})
	}

	// Wait for the periodic flush (BatchMaxAge = 5s)
	time.Sleep(6 * time.Second)

	cancel()
	time.Sleep(100 * time.Millisecond) // allow final flush

	count, err := db.CountFiles(context.Background())
	if err != nil {
		t.Fatalf("CountFiles failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 files after flush loop, got %d", count)
	}
}

func TestQueryAllFiles_ExcludesDirectories(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	b := NewBatchInserter(db)
	rows := []MediaRow{
		{Storage: "local", Path: "/movies/f1.mp4", Name: "f1.mp4", Size: 100, IsDir: false},
		{Storage: "local", Path: "/movies/sub", Name: "sub", Size: 0, IsDir: true},
		{Storage: "local", Path: "/movies/f2.mp4", Name: "f2.mp4", Size: 200, IsDir: false},
	}
	for _, r := range rows {
		b.Insert(ctx, r)
	}
	b.Flush(ctx)

	files, err := db.QueryAllFiles(ctx)
	if err != nil {
		t.Fatalf("QueryAllFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files (no dirs), got %d", len(files))
	}
}

func TestBatchInserter_FlushEmpty(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	b := NewBatchInserter(db)
	// Flush with no data should not error
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("Flush empty failed: %v", err)
	}
}

func TestBatchInserter_100kFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 100k insert test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "large.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	b := NewBatchInserter(db)

	const count = 100000
	for i := 0; i < count; i++ {
		row := MediaRow{
			Storage:  "local",
			Path:     "/files/file_" + itoa(i) + ".mp4",
			Name:     "file_" + itoa(i) + ".mp4",
			Size:     int64(i * 100),
			IsDir:    false,
			Modified: "2024-01-01",
		}
		if err := b.Insert(ctx, row); err != nil {
			t.Fatalf("Insert %d failed: %v", i, err)
		}
	}
	// Flush remaining
	if err := b.Flush(ctx); err != nil {
		t.Fatalf("Final flush failed: %v", err)
	}

	total, err := db.CountFiles(ctx)
	if err != nil {
		t.Fatalf("CountFiles failed: %v", err)
	}
	if total != count {
		t.Errorf("expected %d files, got %d", count, total)
	}

	// Verify DB file size
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	t.Logf("100k files DB size: %d bytes (%.1f MB)", info.Size(), float64(info.Size())/(1024*1024))
}

func TestDeleteCompletedTasks(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.SaveTask(ctx, "local", "/a")
	db.SaveTask(ctx, "local", "/b")
	db.UpdateTaskStatusByPath(ctx, "local", "/a", TaskStatusCompleted)

	if err := db.DeleteCompletedTasks(ctx); err != nil {
		t.Fatalf("DeleteCompletedTasks failed: %v", err)
	}

	pending, err := db.LoadPendingTasks(ctx)
	if err != nil {
		t.Fatalf("LoadPendingTasks failed: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending task after cleanup, got %d", len(pending))
	}
	if pending[0].Path != "/b" {
		t.Errorf("expected remaining task /b, got %s", pending[0].Path)
	}
}

func TestStats(t *testing.T) {
	db := newTestDB(t)
	stats := db.Stats()
	if stats.OpenConnections < 1 {
		t.Error("expected at least 1 open connection")
	}
}

// itoa is a simple int to string converter to avoid importing strconv in tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
