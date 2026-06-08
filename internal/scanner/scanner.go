package scanner

import (
	"context"
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

	// Enqueue seed tasks synchronously — fast with a buffered channel
	for _, task := range seeds {
		s.taskCh <- task
	}

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

// worker processes tasks from the task queue.
func (s *Scanner) worker(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			// Drain pending so the monitor can proceed
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

// processTask handles a single scan task by listing the directory and
// dispatching sub-tasks for directories and results for files.
func (s *Scanner) processTask(ctx context.Context, task ScanTask) {
	s.stats.addDirectory()

	result, err := s.client.List(ctx, task.Path)
	if err != nil {
		// Log error but continue — one failed directory shouldn't stop the scan
		return
	}

	for _, item := range result.Content {
		res := ScanResult{
			Storage:  task.Storage,
			Path:     item.Path,
			Name:     item.Name,
			Size:     item.Size,
			IsDir:    item.IsDir,
			Modified: item.Modified,
		}

		if item.IsDir {
			// BFS: enqueue sub-directory
			s.pending.Add(1)
			if !s.enqueue(ctx, ScanTask{Storage: task.Storage, Path: item.Path}) {
				return
			}
		} else {
			s.stats.addFile()
			// Send result, respecting context cancellation
			select {
			case <-ctx.Done():
				return
			case s.resultCh <- res:
			}
		}
	}
}
