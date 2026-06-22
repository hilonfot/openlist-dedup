package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"openlist/internal/openlist"
)

// Scanner performs BFS directory scanning over OpenList entries using a
// worker pool pattern.
type Scanner struct {
	client    *openlist.Client
	workers   int
	queueSize int

	taskCh   chan ScanTask
	resultCh chan ScanResult
	doneCh   chan struct{}
	stats    *Stats

	wg            sync.WaitGroup
	pending       sync.WaitGroup
	activeWorkers atomic.Int32
	started       atomic.Bool
}

// Config holds scanner configuration.
type Config struct {
	Client    *openlist.Client
	Workers   int
	QueueSize int
}

// New creates a new Scanner with the given config.
func New(cfg Config) *Scanner {
	if cfg.Workers <= 0 {
		cfg.Workers = 32
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 10000
	}

	return &Scanner{
		client:    cfg.Client,
		workers:   cfg.Workers,
		queueSize: cfg.QueueSize,
		taskCh:    make(chan ScanTask, cfg.QueueSize),
		resultCh:  make(chan ScanResult, cfg.QueueSize),
		doneCh:    make(chan struct{}),
		stats:     newStats(),
	}
}

// Start begins scanning from the given seed tasks. It launches the worker
// pool and returns immediately. Results are delivered via the Results channel.
// Call Wait to block until scanning completes.
func (s *Scanner) Start(ctx context.Context, seeds []ScanTask) {
	if s.started.Load() {
		return
	}
	s.started.Store(true)

	// Count seed tasks as pending work
	s.pending.Add(len(seeds))

	// Launch workers
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}

	// Enqueue seed tasks asynchronously to avoid blocking when
	// len(seeds) > queueSize. Workers are already running, so the
	// channel will drain as soon as they start consuming.
	go func() {
		for _, task := range seeds {
			s.taskCh <- task
		}
	}()

	// Monitor: when all pending tasks are done, signal workers to stop
	go func() {
		s.pending.Wait()
		close(s.taskCh)
	}()
}

// Results returns a read-only channel of scan results.
// The channel is closed when all work is done.
func (s *Scanner) Results() <-chan ScanResult {
	return s.resultCh
}

// Stats returns a snapshot of the current scan progress.
func (s *Scanner) Stats() StatsSnapshot {
	return s.stats.Snapshot()
}

// ActiveWorkers returns the number of currently active workers.
func (s *Scanner) ActiveWorkers() int {
	return int(s.activeWorkers.Load())
}

// Wait blocks until all workers have finished, then closes the result and
// done channels. It must NOT be called from within a worker goroutine.
func (s *Scanner) Wait() {
	s.wg.Wait()
	close(s.resultCh)
	close(s.doneCh)
}

// Done returns a channel that is closed when scanning is complete.
func (s *Scanner) Done() <-chan struct{} {
	return s.doneCh
}

// enqueue sends a task to the task channel, respecting context cancellation.
func (s *Scanner) enqueue(ctx context.Context, task ScanTask) bool {
	select {
	case <-ctx.Done():
		s.pending.Done() // don't leak the pending count
		return false
	case s.taskCh <- task:
		return true
	}
}

// drainPending non-blockingly dequeues all tasks from taskCh and decrements
// pending for each one. This is called by workers when context is cancelled to
// ensure the pending WaitGroup can reach zero and the monitor can shut down.
func (s *Scanner) drainPending() {
	for {
		select {
		case <-s.taskCh:
			s.pending.Done()
		default:
			return
		}
	}
}

// worker processes tasks from the task queue.
func (s *Scanner) worker(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			// Drain remaining tasks from taskCh so pending can reach zero
			// and the monitor goroutine can close the channel cleanly.
			s.drainPending()
			return
		case task, ok := <-s.taskCh:
			if !ok {
				return
			}
			s.activeWorkers.Add(1)
			s.processTask(ctx, task)
			s.pending.Done()
			s.activeWorkers.Add(-1)
		}
	}
}

// buildPath constructs the full path for an item. If the API returns an empty
// path, we join the parent path with the item name.
func buildPath(parentPath, itemPath, itemName string) string {
	if itemPath != "" {
		return itemPath
	}
	if parentPath == "" || parentPath == "/" {
		return "/" + itemName
	}
	return parentPath + "/" + itemName
}

// mediaExtensions lists file extensions considered as media files.
var mediaExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	".ts": true, ".mts": true, ".m2ts": true, ".iso": true,
	".mpeg": true, ".mpg": true, ".3gp": true, ".vob": true,
	".ogm": true, ".ogv": true, ".asf": true, ".rm": true, ".rmvb": true,
}

// isMediaFile checks if a file name has a recognized media extension.
func isMediaFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return mediaExtensions[ext]
}

// processTask handles a single scan task by listing the directory and
// dispatching sub-tasks for directories and results for files.
func (s *Scanner) processTask(ctx context.Context, task ScanTask) {
	s.stats.addDirectory()

	result, err := s.client.List(ctx, task.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: scanner: list %s: %v\n", task.Path, err)
		return
	}

	// Log directory scan progress
	var dirCount, fileCount int
	for _, item := range result.Content {
		if item.IsDir {
			dirCount++
		} else {
			fileCount++
		}
	}
	fmt.Fprintf(os.Stderr, "SCAN: %s: %d dirs, %d files (storage: %s)\n", task.Path, dirCount, fileCount, task.Storage)

	for _, item := range result.Content {
		// Build path: use API path if available, otherwise construct from parent+name
		itemPath := buildPath(task.Path, item.Path, item.Name)

		res := ScanResult{
			Storage:  task.Storage,
			Path:     itemPath,
			Name:     item.Name,
			Size:     item.Size,
			IsDir:    item.IsDir,
			Modified: item.Modified,
		}

		if item.IsDir {
			// BFS: enqueue sub-directory
			s.pending.Add(1)
			if !s.enqueue(ctx, ScanTask{Storage: task.Storage, Path: itemPath}) {
				return
			}
		} else if isMediaFile(item.Name) {
			s.stats.addFile()
			// Send result, respecting context cancellation
			select {
			case <-ctx.Done():
				return
			case s.resultCh <- res:
			}
		} else {
			// Skip non-media files (subtitles, images, etc.)
			s.stats.addFile()
		}
	}
}
