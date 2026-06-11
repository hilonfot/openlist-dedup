package duplicate

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"openlist/internal/media"
)

type Decision string

const (
	Keep   Decision = "Keep"
	Delete Decision = "Delete"
	Unique Decision = "Unique"
)

const folderLevelTag = "__FOLDER__"

var storagePriority = []string{"local", "tianyi", "quark"}

type FileEntry struct {
	ID       int64
	Storage  string
	Path     string
	Name     string
	Size     int64
	IsDir    bool
	Decision Decision
}

type DuplicateGroup struct {
	NormalizedName string
	EpisodeTag     string
	IsEpisode      bool
	Files          []FileEntry
}

type Stats struct {
	TotalFiles     int
	UniqueFiles    int
	DuplicateSets  int
	DuplicateFiles int
	DuplicateSize  int64
	KeepFiles      int
	DeleteFiles    int
}

type Detector struct{}

func New() *Detector {
	return &Detector{}
}

func (d *Detector) Detect(entries []FileEntry) ([]DuplicateGroup, Stats) {
	if len(entries) == 0 {
		return nil, Stats{}
	}
	groups := d.groupByNormalizedName(entries)
	var result []DuplicateGroup
	stats := Stats{TotalFiles: len(entries)}
	seen := make(map[int64]bool)

	for _, g := range groups {
		if len(g.Files) <= 1 {
			if len(g.Files) == 1 {
				g.Files[0].Decision = Unique
				stats.UniqueFiles++
				seen[g.Files[0].ID] = true
			}
			result = append(result, g)
			continue
		}

		var verified []FileEntry
		if g.IsEpisode && g.EpisodeTag == folderLevelTag {
			verified = g.Files
		} else {
			verified = d.verifyBySize(g.Files)
		}

		if len(verified) <= 1 {
			for i := range g.Files {
				g.Files[i].Decision = Unique
				stats.UniqueFiles++
				seen[g.Files[i].ID] = true
			}
			result = append(result, g)
			continue
		}

		if g.IsEpisode && g.EpisodeTag == folderLevelTag {
			d.assignFolderDecisions(verified)
		} else {
			d.assignDecisions(verified)
		}
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

	stats.UniqueFiles = stats.TotalFiles - stats.DuplicateFiles - (len(entries) - len(seen))
	return result, stats
}

// groupByNormalizedName groups FileEntry slices by their normalized media name.
// For TV episodes, grouping is done by the parent folder name instead of individual episode name.
// Files named as bare numbers (01.mkv, 02.mkv) are also treated as TV episodes.
func (d *Detector) groupByNormalizedName(entries []FileEntry) []DuplicateGroup {
	groups := make(map[string]*DuplicateGroup)
	var keys []string

	for _, entry := range entries {
		info := media.Normalize(entry.Name)

		var key string
		var normName, epTag string
		var isEp bool

		if info.IsEpisode {
			parentFolder := filepath.Base(filepath.Dir(entry.Path))
			folderName := parentFolder
			if isSeasonFolder(parentFolder) {
				folderName = filepath.Base(filepath.Dir(filepath.Dir(entry.Path)))
			}
			key = folderName + "||" + folderLevelTag
			normName = folderName
			epTag = folderLevelTag
			isEp = true
		} else if isBareNumberFile(entry.Name) || hasLeadingEpNumber(entry.Name) {
			// Bare number files (01.mkv, 02.mkv) → treat as TV episodes
			parentFolder := filepath.Base(filepath.Dir(entry.Path))
			folderName := parentFolder
			if isSeasonFolder(parentFolder) {
				folderName = filepath.Base(filepath.Dir(filepath.Dir(entry.Path)))
			}
			key = folderName + "||" + folderLevelTag
			normName = folderName
			epTag = folderLevelTag
			isEp = true
		} else {
			key = info.Title + "||" + info.EpisodeTag
			normName = info.Title
			epTag = info.EpisodeTag
			isEp = info.IsEpisode
		}

		if g, ok := groups[key]; ok {
			g.Files = append(g.Files, entry)
		} else {
			keys = append(keys, key)
			groups[key] = &DuplicateGroup{
				NormalizedName: normName,
				EpisodeTag:     epTag,
				IsEpisode:      isEp,
				Files:          []FileEntry{entry},
			}
		}
	}

	result := make([]DuplicateGroup, 0, len(groups))
	for _, k := range keys {
		result = append(result, *groups[k])
	}
	return result
}

// isBareNumberFile checks if a filename is a bare number (e.g., "01.mkv", "02.mp4").
// hasLeadingEpNumber checks if filename starts with an episode number prefix.
// Examples: "14.枭起青壤.mkv" -> true, "02.剧名.mp4" -> true
func hasLeadingEpNumber(name string) bool {
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}
	// Check if starts with digits followed by a separator
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(name) {
		return false
	}
	// After digits must have a separator
	return name[i] == '.' || name[i] == ' ' || name[i] == '-' || name[i] == '_'
}

func isBareNumberFile(name string) bool {
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}
	if name == "" {
		return false
	}
	for _, c := range name {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (d *Detector) verifyBySize(files []FileEntry) []FileEntry {
	if len(files) < 2 {
		return files
	}
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

func (d *Detector) assignDecisions(files []FileEntry) {
	if len(files) == 0 {
		return
	}
	bestIdx := 0
	bestPriority := storageRank(files[0].Storage)
	for i := 1; i < len(files); i++ {
		pri := storageRank(files[i].Storage)
		if pri < bestPriority {
			bestPriority = pri
			bestIdx = i
		}
	}
	for i := range files {
		if i == bestIdx {
			files[i].Decision = Keep
		} else {
			files[i].Decision = Delete
		}
	}
}

func (d *Detector) assignFolderDecisions(files []FileEntry) {
	if len(files) == 0 {
		return
	}
	bestStorage := files[0].Storage
	bestPriority := storageRank(files[0].Storage)
	for _, f := range files[1:] {
		pri := storageRank(f.Storage)
		if pri < bestPriority {
			bestPriority = pri
			bestStorage = f.Storage
		}
	}
	for i := range files {
		if files[i].Storage == bestStorage {
			files[i].Decision = Keep
		} else {
			files[i].Decision = Delete
		}
	}
}

func storageRank(storage string) int {
	for i, s := range storagePriority {
		if s == storage {
			return i
		}
	}
	return len(storagePriority)
}

func isSeasonFolder(name string) bool {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^[Ss]eason\s+\d+$`),
		regexp.MustCompile(`^[Ss]\d+$`),
		regexp.MustCompile(`^第[\d一二三四五六七八九十百千]+季$`),
		regexp.MustCompile(`^第\s*\d+\s*季$`),
	}
	for _, re := range patterns {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

func FormatGroupOutput(g DuplicateGroup) string {
	s := fmt.Sprintf("Group: %s", g.NormalizedName)
	if g.IsEpisode {
		s += " " + g.EpisodeTag
	}
	s += fmt.Sprintf(" (%d files)\n", len(g.Files))

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
