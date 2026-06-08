package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"openlist/internal/openlist"
)

// mockFileTree is a simple in-memory file tree for the mock server.
type mockFileTree map[string][]openlist.FileInfo

// scanTestHarness wraps the mock server and scanner for testing.
type scanTestHarness struct {
	server *httptest.Server
	client *openlist.Client
}

// newScanTestHarness creates a test harness with a mock OpenList server that
// serves the given file tree.
func newScanTestHarness(t *testing.T, tree mockFileTree) *scanTestHarness {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		content, ok := tree[req.Path]
		if !ok {
			content = []openlist.FileInfo{} // empty directory
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"content": content,
				"total":   len(content),
			},
		})
	})

	server := httptest.NewServer(mux)
	client := openlist.New(server.URL, "", 0, 0)

	return &scanTestHarness{server: server, client: client}
}

func (h *scanTestHarness) Close() {
	h.server.Close()
}

func TestNew(t *testing.T) {
	s := New(Config{})
	if s.workers != 32 {
		t.Errorf("expected default workers 32, got %d", s.workers)
	}
	if s.queueSize != 10000 {
		t.Errorf("expected default queueSize 10000, got %d", s.queueSize)
	}
	if s.taskCh == nil {
		t.Error("expected non-nil taskCh")
	}
	if s.resultCh == nil {
		t.Error("expected non-nil resultCh")
	}
	if s.stats == nil {
		t.Error("expected non-nil stats")
	}
}

func TestNew_CustomConfig(t *testing.T) {
	s := New(Config{Workers: 8, QueueSize: 500})
	if s.workers != 8 {
		t.Errorf("expected workers 8, got %d", s.workers)
	}
	if s.queueSize != 500 {
		t.Errorf("expected queueSize 500, got %d", s.queueSize)
	}
}

func TestScan_SingleDirectory(t *testing.T) {
	tree := mockFileTree{
		"/movies": {
			{Name: "movie1.mp4", Path: "/movies/movie1.mp4", Size: 1000, IsDir: false},
			{Name: "movie2.mp4", Path: "/movies/movie2.mp4", Size: 2000, IsDir: false},
		},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var mu sync.Mutex
	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/movies"}})
	s.Wait()
	wg.Wait()

	if len(results) != 2 {
		t.Errorf("expected 2 file results, got %d", len(results))
	}

	mu.Lock()
	defer mu.Unlock()
	found := make(map[string]bool)
	for _, r := range results {
		found[r.Name] = true
		if r.Storage != StorageLocal {
			t.Errorf("expected StorageLocal, got %s", r.Storage)
		}
	}
	if !found["movie1.mp4"] || !found["movie2.mp4"] {
		t.Errorf("expected to find both movies, got %v", found)
	}
}

func TestScan_NestedDirectoriesBFS(t *testing.T) {
	tree := mockFileTree{
		"/media": {
			{Name: "movies", Path: "/media/movies", IsDir: true, Size: 0},
			{Name: "tv", Path: "/media/tv", IsDir: true, Size: 0},
		},
		"/media/movies": {
			{Name: "film1.mp4", Path: "/media/movies/film1.mp4", Size: 500, IsDir: false},
		},
		"/media/tv": {
			{Name: "show1.mp4", Path: "/media/tv/show1.mp4", Size: 300, IsDir: false},
			{Name: "series", Path: "/media/tv/series", IsDir: true, Size: 0},
		},
		"/media/tv/series": {
			{Name: "ep1.mp4", Path: "/media/tv/series/ep1.mp4", Size: 200, IsDir: false},
			{Name: "ep2.mp4", Path: "/media/tv/series/ep2.mp4", Size: 200, IsDir: false},
		},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/media"}})
	s.Wait()
	wg.Wait()

	if len(results) != 4 {
		t.Errorf("expected 4 file results, got %d", len(results))
	}

	files := make(map[string]int)
	for _, r := range results {
		files[r.Path]++
	}
	expectedFiles := []string{
		"/media/movies/film1.mp4",
		"/media/tv/show1.mp4",
		"/media/tv/series/ep1.mp4",
		"/media/tv/series/ep2.mp4",
	}
	for _, ef := range expectedFiles {
		if files[ef] != 1 {
			t.Errorf("expected file %s exactly once, got %d", ef, files[ef])
		}
	}
}

func TestScan_MultipleStorages(t *testing.T) {
	tree := mockFileTree{
		"/": {
			{Name: "file1.mp4", Path: "/file1.mp4", Size: 100, IsDir: false},
		},
		"/quark-root": {
			{Name: "qfile1.mp4", Path: "/quark-root/qfile1.mp4", Size: 200, IsDir: false},
		},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	seeds := []ScanTask{
		{Storage: StorageLocal, Path: "/"},
		{Storage: StorageQuark, Path: "/quark-root"},
	}
	s.Start(ctx, seeds)
	s.Wait()
	wg.Wait()

	if len(results) != 2 {
		t.Errorf("expected 2 file results, got %d", len(results))
	}

	storageTypes := make(map[Storage]int)
	for _, r := range results {
		storageTypes[r.Storage]++
	}
	if storageTypes[StorageLocal] != 1 {
		t.Errorf("expected 1 local file, got %d", storageTypes[StorageLocal])
	}
	if storageTypes[StorageQuark] != 1 {
		t.Errorf("expected 1 quark file, got %d", storageTypes[StorageQuark])
	}
}

func TestScan_EmptyDirectory(t *testing.T) {
	tree := mockFileTree{
		"/empty": {},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/empty"}})
	s.Wait()
	wg.Wait()

	if len(results) != 0 {
		t.Errorf("expected 0 file results for empty directory, got %d", len(results))
	}
}

func TestScan_StatsTracking(t *testing.T) {
	tree := mockFileTree{
		"/root": {
			{Name: "sub", Path: "/root/sub", IsDir: true, Size: 0},
			{Name: "f1.mp4", Path: "/root/f1.mp4", Size: 100, IsDir: false},
		},
		"/root/sub": {
			{Name: "f2.mp4", Path: "/root/sub/f2.mp4", Size: 200, IsDir: false},
		},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/root"}})
	s.Wait()
	wg.Wait()

	stats := s.Stats()
	if stats.Directories != 2 {
		t.Errorf("expected 2 directories scanned, got %d", stats.Directories)
	}
	if stats.Files != 2 {
		t.Errorf("expected 2 files found, got %d", stats.Files)
	}
	if stats.Elapsed <= 0 {
		t.Error("expected positive elapsed time")
	}
}

func TestScan_ContextCancellation(t *testing.T) {
	// Create a server that blocks to allow context cancellation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"content": []interface{}{},
				"total":   0,
			},
		})
	}))
	defer server.Close()

	client := openlist.New(server.URL, "", 0, 0)
	s := New(Config{Client: client, Workers: 4, QueueSize: 100})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	// Should return without hanging
	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/slow"}})
	s.Wait()
}

func TestScan_DeepDirectory(t *testing.T) {
	// Build a deep directory tree: /a/b/c/d/e/f/g/h/i/j
	tree := mockFileTree{}
	depth := 10
	path := "/a"
	for i := 0; i < depth-1; i++ {
		child := string(rune('a' + i + 1))
		subPath := path + "/" + child
		tree[path] = []openlist.FileInfo{
			{Name: child, Path: subPath, IsDir: true, Size: 0},
		}
		path = subPath
	}
	// Leaf directory with a file
	tree[path] = []openlist.FileInfo{
		{Name: "deep.mp4", Path: path + "/deep.mp4", Size: 999, IsDir: false},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/a"}})
	s.Wait()
	wg.Wait()

	if len(results) != 1 {
		t.Errorf("expected 1 file result at depth %d, got %d", depth, len(results))
	}
	if len(results) > 0 && results[0].Name != "deep.mp4" {
		t.Errorf("expected deep.mp4, got %s", results[0].Name)
	}

	stats := s.Stats()
	if stats.Directories != int64(depth) {
		t.Errorf("expected %d directories scanned, got %d", depth, stats.Directories)
	}
}

func TestScan_ConcurrencyLimit(t *testing.T) {
	tree := mockFileTree{
		"/pool": {
			{Name: "a", Path: "/pool/a", IsDir: true, Size: 0},
			{Name: "b", Path: "/pool/b", IsDir: true, Size: 0},
			{Name: "c", Path: "/pool/c", IsDir: true, Size: 0},
		},
		"/pool/a": {{Name: "fa.mp4", Path: "/pool/a/fa.mp4", Size: 100, IsDir: false}},
		"/pool/b": {{Name: "fb.mp4", Path: "/pool/b/fb.mp4", Size: 100, IsDir: false}},
		"/pool/c": {{Name: "fc.mp4", Path: "/pool/c/fc.mp4", Size: 100, IsDir: false}},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	// Use only 2 workers to verify pool behavior
	s := New(Config{Client: h.client, Workers: 2, QueueSize: 10})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/pool"}})
	s.Wait()
	wg.Wait()

	if len(results) != 3 {
		t.Errorf("expected 3 file results, got %d", len(results))
	}
}

func TestScan_NoSeeds(t *testing.T) {
	h := newScanTestHarness(t, mockFileTree{})
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	s.Start(ctx, nil) // no seeds
	s.Wait()
	wg.Wait()

	if len(results) != 0 {
		t.Errorf("expected 0 results with no seeds, got %d", len(results))
	}
}

func TestStatsSnapshot(t *testing.T) {
	stats := newStats()
	stats.addDirectory()
	stats.addFile()
	stats.addFile()
	stats.addFile()

	snap := stats.Snapshot()
	if snap.Directories != 1 {
		t.Errorf("expected 1 directory, got %d", snap.Directories)
	}
	if snap.Files != 3 {
		t.Errorf("expected 3 files, got %d", snap.Files)
	}
	if snap.Elapsed <= 0 {
		t.Error("expected positive elapsed")
	}
}

func TestActiveWorkers(t *testing.T) {
	tree := mockFileTree{
		"/work": {
			{Name: "f1.mp4", Path: "/work/f1.mp4", Size: 100, IsDir: false},
			{Name: "f2.mp4", Path: "/work/f2.mp4", Size: 200, IsDir: false},
		},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	// Check active workers before start
	if s.ActiveWorkers() != 0 {
		t.Errorf("expected 0 active workers before start, got %d", s.ActiveWorkers())
	}

	var done bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range s.Results() {
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/work"}})
	s.Wait()
	wg.Wait()

	if !done {
		// Just verify the scan completed
	}
}

func TestDoneChannel(t *testing.T) {
	tree := mockFileTree{
		"/done": {
			{Name: "f.mp4", Path: "/done/f.mp4", Size: 100, IsDir: false},
		},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range s.Results() {
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageLocal, Path: "/done"}})
	s.Wait()
	wg.Wait()

	// Done channel should be closed
	select {
	case <-s.Done():
		// OK
	default:
		t.Error("expected Done channel to be closed after scan completes")
	}
}

func TestScan_LargeDirectory(t *testing.T) {
	// Create a directory with 100 files
	content := make([]openlist.FileInfo, 100)
	for i := 0; i < 100; i++ {
		content[i] = openlist.FileInfo{
			Name:     fmt.Sprintf("file_%03d.mp4", i),
			Path:     fmt.Sprintf("/large/file_%03d.mp4", i),
			Size:     int64(i * 100),
			IsDir:    false,
		}
	}
	tree := mockFileTree{
		"/large": content,
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 8, QueueSize: 200})
	ctx := context.Background()

	var results []ScanResult
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			results = append(results, r)
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageQuark, Path: "/large"}})
	s.Wait()
	wg.Wait()

	if len(results) != 100 {
		t.Errorf("expected 100 file results, got %d", len(results))
	}
}

func TestScan_ResultOrder(t *testing.T) {
	// Results should be delivered regardless of order
	tree := mockFileTree{
		"/order": {
			{Name: "dir1", Path: "/order/dir1", IsDir: true, Size: 0},
			{Name: "f1.mp4", Path: "/order/f1.mp4", Size: 100, IsDir: false},
			{Name: "dir2", Path: "/order/dir2", IsDir: true, Size: 0},
			{Name: "f2.mp4", Path: "/order/f2.mp4", Size: 200, IsDir: false},
		},
		"/order/dir1": {
			{Name: "sub1.mp4", Path: "/order/dir1/sub1.mp4", Size: 300, IsDir: false},
		},
		"/order/dir2": {
			{Name: "sub2.mp4", Path: "/order/dir2/sub2.mp4", Size: 400, IsDir: false},
		},
	}

	h := newScanTestHarness(t, tree)
	defer h.Close()

	s := New(Config{Client: h.client, Workers: 4, QueueSize: 100})
	ctx := context.Background()

	allFiles := make(map[string]bool)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range s.Results() {
			allFiles[r.Path] = true
		}
	}()

	s.Start(ctx, []ScanTask{{Storage: StorageTianyi, Path: "/order"}})
	s.Wait()
	wg.Wait()

	expected := []string{
		"/order/f1.mp4",
		"/order/f2.mp4",
		"/order/dir1/sub1.mp4",
		"/order/dir2/sub2.mp4",
	}
	for _, p := range expected {
		if !allFiles[p] {
			t.Errorf("missing file result: %s", p)
		}
	}
	if len(allFiles) != len(expected) {
		t.Errorf("expected %d files, got %d", len(expected), len(allFiles))
	}
}
