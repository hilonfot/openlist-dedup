package scanner

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stats tracks scanning progress.
type Stats struct {
	mu             sync.Mutex
	directories    int64
	files          int64
	startTime      time.Time
	lastReportTime time.Time
	lastFileCount  int64
	currentSpeed   float64
}

// newStats creates a new Stats tracker.
func newStats() *Stats {
	now := time.Now()
	return &Stats{
		startTime:      now,
		lastReportTime: now,
	}
}

// addDirectory increments the directory count.
func (s *Stats) addDirectory() {
	atomic.AddInt64(&s.directories, 1)
}

// addFile increments the file count.
func (s *Stats) addFile() {
	atomic.AddInt64(&s.files, 1)
}

// Directories returns the number of scanned directories.
func (s *Stats) Directories() int64 {
	return atomic.LoadInt64(&s.directories)
}

// Files returns the number of discovered files.
func (s *Stats) Files() int64 {
	return atomic.LoadInt64(&s.files)
}

// Snapshot returns a point-in-time copy of the stats.
func (s *Stats) Snapshot() StatsSnapshot {
	elapsed := time.Since(s.startTime)
	files := atomic.LoadInt64(&s.files)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update speed every 2 seconds
	if time.Since(s.lastReportTime) > 2*time.Second {
		delta := files - s.lastFileCount
		elapsedSinceLast := time.Since(s.lastReportTime).Seconds()
		if elapsedSinceLast > 0 {
			s.currentSpeed = float64(delta) / elapsedSinceLast
		}
		s.lastReportTime = time.Now()
		s.lastFileCount = files
	}

	return StatsSnapshot{
		Directories:     atomic.LoadInt64(&s.directories),
		Files:           files,
		Elapsed:         elapsed,
		CurrentSpeed:    s.currentSpeed,
	}
}

// StatsSnapshot is a point-in-time view of scanner progress.
type StatsSnapshot struct {
	Directories     int64
	Files           int64
	Elapsed         time.Duration
	CurrentSpeed    float64
}

// ETA returns the estimated time remaining based on current speed.
// Returns 0 if speed is too low to estimate.
func (s StatsSnapshot) ETA() time.Duration {
	if s.CurrentSpeed <= 0 {
		return 0
	}
	return 0 // unknown total, so ETA is not meaningful for directory listing
}
