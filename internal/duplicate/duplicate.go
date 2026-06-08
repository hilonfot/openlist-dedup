package duplicate

import (
	"fmt"
	"math"
	"sort"

	"openlist/internal/media"
)

// Decision is the action to take for a duplicate file.
type Decision string

const (
	Keep   Decision = "Keep"
	Delete Decision = "Delete"
	Unique Decision = "Unique"
)

// storagePriority defines the priority for keeping files.
// Lower index = higher priority.
var storagePriority = []string{"local", "tianyi", "quark"}

// FileEntry represents a scanned file with its duplicate decision.
type FileEntry struct {
	ID       int64
	Storage  string
	Path     string
	Name     string
	Size     int64
	IsDir    bool
	Decision Decision
}

// DuplicateGroup represents a group of files that are potential duplicates.
type DuplicateGroup struct {
	NormalizedName string
	EpisodeTag     string
	IsEpisode      bool
	Files          []FileEntry
}

// Stats holds duplicate detection statistics.
type Stats struct {
	TotalFiles    int
	UniqueFiles   int
	DuplicateSets int // number of duplicate groups
	DuplicateFiles int
	DuplicateSize int64 // total wasted space if duplicates are removed
	KeepFiles     int
	DeleteFiles   int
}

// Detector finds duplicate media files using multi-layer detection.
type Detector struct{}

// New creates a new Detector.
func New() *Detector {
	return &Detector{}
}

// Detect runs duplicate detection on the given file entries.
// It performs three layers of matching:
//
//	Layer 1: Normalized name grouped by media.Normalize()
//	Layer 2: File size comparison with < 1% tolerance
//	Layer 3: TMDB ID matching (skipped until Phase 7)
func (d *Detector) Detect(entries []FileEntry) ([]DuplicateGroup, Stats) {
	if len(entries) == 0 {
		return nil, Stats{}
	}

	// Layer 1: Group by normalized name + episode tag
	groups := d.groupByNormalizedName(entries)

	// Layer 2: Within each group, verify by file size
	var result []DuplicateGroup
	stats := Stats{TotalFiles: len(entries)}
	seen := make(map[int64]bool) // track unique files

	for _, g := range groups {
		if len(g.Files) <= 1 {
			// Single file — mark as unique
			if len(g.Files) == 1 {
				g.Files[0].Decision = Unique
				stats.UniqueFiles++
				seen[g.Files[0].ID] = true
			}
			result = append(result, g)
			continue
		}

		// Layer 2: Verify by size comparison
		verified := d.verifyBySize(g.Files)

		if len(verified) <= 1 {
			// Size didn't match — treat all as unique
			for i := range g.Files {
				g.Files[i].Decision = Unique
				stats.UniqueFiles++
				seen[g.Files[i].ID] = true
			}
			result = append(result, g)
			continue
		}

		// Some files matched by size — they form a duplicate group
		d.assignDecisions(verified)
		dupGroup := DuplicateGroup{
			NormalizedName: g.NormalizedName,
			EpisodeTag:     g.EpisodeTag,
			IsEpisode:      g.IsEpisode,
			Files:          verified,
		}
		result = append(result, dupGroup)
		stats.DuplicateSets++
		stats.DuplicateFiles += len(verified)

		for _, f := range verified {
			seen[f.ID] = true
			if f.Decision == Keep {
				stats.KeepFiles++
			} else {
				stats.DeleteFiles++
				stats.DuplicateSize += f.Size
			}
		}

		// Non-matched files in the same name group remain as unique
		var leftover []FileEntry
		matchedIDs := make(map[int64]bool, len(verified))
		for _, f := range verified {
			matchedIDs[f.ID] = true
		}
		for _, f := range g.Files {
			if !matchedIDs[f.ID] {
				f.Decision = Unique
				leftover = append(leftover, f)
				stats.UniqueFiles++
				seen[f.ID] = true
			}
		}
		if len(leftover) > 0 {
			result = append(result, DuplicateGroup{
				NormalizedName: g.NormalizedName,
				EpisodeTag:     g.EpisodeTag,
				IsEpisode:      g.IsEpisode,
				Files:          leftover,
			})
		}
	}

	// Count unique files that weren't in any group
	stats.UniqueFiles = stats.TotalFiles - stats.DuplicateFiles - (len(entries) - len(seen))

	return result, stats
}

// groupByNormalizedName groups FileEntry slices by their normalized media name.
// Layer 1: name-based grouping.
func (d *Detector) groupByNormalizedName(entries []FileEntry) []DuplicateGroup {
	groups := make(map[string]*DuplicateGroup)
	// Use a stable key: "normalized_name||episode_tag"
	var keys []string

	for _, entry := range entries {
		info := media.Normalize(entry.Name)
		key := info.Title + "||" + info.EpisodeTag

		if g, ok := groups[key]; ok {
			g.Files = append(g.Files, entry)
		} else {
			keys = append(keys, key)
			groups[key] = &DuplicateGroup{
				NormalizedName: info.Title,
				EpisodeTag:     info.EpisodeTag,
				IsEpisode:      info.IsEpisode,
				Files:          []FileEntry{entry},
			}
		}
	}

	// Convert to slice in insertion order
	result := make([]DuplicateGroup, 0, len(groups))
	for _, k := range keys {
		result = append(result, *groups[k])
	}
	return result
}

// verifyBySize filters files that have similar sizes (within 1% tolerance).
// Returns only files that form at least one size-matching pair.
func (d *Detector) verifyBySize(files []FileEntry) []FileEntry {
	if len(files) < 2 {
		return files
	}

	// Build adjacency: files with similar sizes are grouped together
	matched := make(map[int64]bool)
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if sizeWithinTolerance(files[i].Size, files[j].Size) {
				matched[files[i].ID] = true
				matched[files[j].ID] = true
			}
		}
	}

	var result []FileEntry
	for _, f := range files {
		if matched[f.ID] {
			result = append(result, f)
		}
	}
	return result
}

// sizeWithinTolerance checks if two file sizes are within 1% of each other.
func sizeWithinTolerance(a, b int64) bool {
	if a == 0 && b == 0 {
		return true
	}
	if a == 0 || b == 0 {
		return false
	}
	max := math.Max(float64(a), float64(b))
	diff := math.Abs(float64(a) - float64(b))
	return (diff / max) < 0.01
}

// assignDecisions applies storage priority to a group of confirmed duplicates.
// The file with the highest priority storage is kept; others are marked for deletion.
func (d *Detector) assignDecisions(files []FileEntry) {
	if len(files) == 0 {
		return
	}

	// Find the file with the highest storage priority
	bestIdx := 0
	bestPriority := storageRank(files[0].Storage)

	for i := 1; i < len(files); i++ {
		pri := storageRank(files[i].Storage)
		if pri < bestPriority {
			bestPriority = pri
			bestIdx = i
		}
	}

	// Mark best as Keep, rest as Delete
	for i := range files {
		if i == bestIdx {
			files[i].Decision = Keep
		} else {
			files[i].Decision = Delete
		}
	}
}

// storageRank returns the priority rank of a storage type.
// Lower number = higher priority. Unknown storages get lowest priority.
func storageRank(storage string) int {
	for i, s := range storagePriority {
		if s == storage {
			return i
		}
	}
	return len(storagePriority)
}

// FormatGroupOutput returns a human-readable summary of a duplicate group.
func FormatGroupOutput(g DuplicateGroup) string {
	s := fmt.Sprintf("Group: %s", g.NormalizedName)
	if g.IsEpisode {
		s += " " + g.EpisodeTag
	}
	s += fmt.Sprintf(" (%d files)\n", len(g.Files))

	// Sort by decision (Keep first) for cleaner output
	sorted := make([]FileEntry, len(g.Files))
	copy(sorted, g.Files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Decision < sorted[j].Decision
	})

	for _, f := range sorted {
		mark := " "
		if f.Decision == Keep {
			mark = "✓"
		} else if f.Decision == Delete {
			mark = "✗"
		}
		s += fmt.Sprintf("  %s [%s] %s (%s, %d bytes)\n", mark, f.Storage, f.Path, f.Decision, f.Size)
	}
	return s
}
