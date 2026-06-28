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

// Package-level compiled regexps. These are matched once per file during
// detection, so compiling them lazily inside the helper functions would
// recompile on every call — hoist them here to compile exactly once.
var (
	reEpisodeToken     = regexp.MustCompile(`^s\d{1,2}e\d{1,3}$`)
	reDigits           = regexp.MustCompile(`\d+`)
	reChineseSeason    = regexp.MustCompile(`^第([一二三四五六七八九十百千]+)季$`)
	seasonFolderRegexp = []*regexp.Regexp{
		regexp.MustCompile(`^[Ss]eason\s+\d+$`),
		regexp.MustCompile(`^[Ss]\d+$`),
		regexp.MustCompile(`^第\s*\d+\s*季$`),
	}
)

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
	Year           int
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
	var result []DuplicateGroup
	stats := Stats{TotalFiles: len(entries)}
	handled := make(map[int64]bool)

	for _, g := range d.detectSeriesFolderDuplicates(entries) {
		result = append(result, g)
		stats.DuplicateSets++
		stats.DuplicateFiles += len(g.Files)
		for _, f := range g.Files {
			handled[f.ID] = true
			if f.Decision == Keep {
				stats.KeepFiles++
			} else if f.Decision == Delete {
				stats.DeleteFiles++
				stats.DuplicateSize += f.Size
			}
		}
	}

	pending := make([]FileEntry, 0, len(entries)-len(handled))
	for _, entry := range entries {
		if !handled[entry.ID] {
			pending = append(pending, entry)
		}
	}

	groups := d.groupByNormalizedName(pending)

	for _, g := range groups {
		if len(g.Files) <= 1 {
			if len(g.Files) == 1 {
				g.Files[0].Decision = Unique
				stats.UniqueFiles++
			}
			result = append(result, g)
			continue
		}

		clusters, unique := d.clusterBySize(g.Files)
		if len(clusters) == 0 {
			for i := range unique {
				unique[i].Decision = Unique
			}
			stats.UniqueFiles += len(unique)
			g.Files = unique
			result = append(result, g)
			continue
		}

		for _, cluster := range clusters {
			d.assignDecisions(cluster)
			result = append(result, DuplicateGroup{
				NormalizedName: g.NormalizedName,
				EpisodeTag:     g.EpisodeTag,
				IsEpisode:      g.IsEpisode,
				Year:           g.Year,
				Files:          cluster,
			})
			stats.DuplicateSets++
			stats.DuplicateFiles += len(cluster)
			for _, f := range cluster {
				if f.Decision == Keep {
					stats.KeepFiles++
				} else if f.Decision == Delete {
					stats.DeleteFiles++
					stats.DuplicateSize += f.Size
				}
			}
		}

		for i := range unique {
			unique[i].Decision = Unique
		}
		if len(unique) > 0 {
			result = append(result, DuplicateGroup{
				NormalizedName: g.NormalizedName,
				EpisodeTag:     g.EpisodeTag,
				IsEpisode:      g.IsEpisode,
				Year:           g.Year,
				Files:          unique,
			})
			stats.UniqueFiles += len(unique)
		}
	}

	return result, stats
}

type seriesFile struct {
	entry   FileEntry
	title   string
	year    int
	epTag   string
	rootDir string
}

type seriesFolder struct {
	rootDir  string
	files    []seriesFile
	episodes map[string]bool
	total    int64
}

func (d *Detector) detectSeriesFolderDuplicates(entries []FileEntry) []DuplicateGroup {
	bySeries := make(map[string]map[string]*seriesFolder)
	seriesOrder := make([]string, 0)
	seriesMeta := make(map[string]seriesFile)

	for _, entry := range entries {
		sf, ok := seriesFileInfo(entry)
		if !ok {
			continue
		}
		seriesKey := entry.Storage + "||" + identityKey("tv-series", sf.title, sf.year, "")
		folders, ok := bySeries[seriesKey]
		if !ok {
			bySeries[seriesKey] = make(map[string]*seriesFolder)
			folders = bySeries[seriesKey]
			seriesOrder = append(seriesOrder, seriesKey)
			seriesMeta[seriesKey] = sf
		}
		folder, ok := folders[sf.rootDir]
		if !ok {
			folder = &seriesFolder{
				rootDir:  sf.rootDir,
				episodes: make(map[string]bool),
			}
			folders[sf.rootDir] = folder
		}
		folder.files = append(folder.files, sf)
		folder.episodes[sf.epTag] = true
		folder.total += sf.entry.Size
	}

	var result []DuplicateGroup
	for _, seriesKey := range seriesOrder {
		folders := bySeries[seriesKey]
		if len(folders) <= 1 {
			continue
		}
		ordered := make([]*seriesFolder, 0, len(folders))
		for _, folder := range folders {
			ordered = append(ordered, folder)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if len(ordered[i].episodes) != len(ordered[j].episodes) {
				return len(ordered[i].episodes) > len(ordered[j].episodes)
			}
			if len(ordered[i].files) != len(ordered[j].files) {
				return len(ordered[i].files) > len(ordered[j].files)
			}
			if ordered[i].total != ordered[j].total {
				return ordered[i].total > ordered[j].total
			}
			return ordered[i].rootDir < ordered[j].rootDir
		})

		keepRoot := ordered[0].rootDir
		var files []FileEntry
		for _, folder := range ordered {
			for _, sf := range folder.files {
				f := sf.entry
				if folder.rootDir == keepRoot {
					f.Decision = Keep
				} else {
					f.Decision = Delete
				}
				files = append(files, f)
			}
		}
		sort.Slice(files, func(i, j int) bool {
			if files[i].Decision != files[j].Decision {
				return files[i].Decision < files[j].Decision
			}
			return files[i].Path < files[j].Path
		})

		meta := seriesMeta[seriesKey]
		result = append(result, DuplicateGroup{
			NormalizedName: meta.title,
			EpisodeTag:     folderLevelTag,
			IsEpisode:      true,
			Year:           meta.year,
			Files:          files,
		})
	}
	return result
}

func seriesFileInfo(entry FileEntry) (seriesFile, bool) {
	info := media.Normalize(entry.Name)
	_, title, epTag, isEpisode, year := mediaIdentity(entry, info)
	if !isEpisode || title == "" || epTag == "" {
		return seriesFile{}, false
	}
	return seriesFile{
		entry:   entry,
		title:   title,
		year:    year,
		epTag:   epTag,
		rootDir: seriesRootDir(entry.Path),
	}, true
}

func seriesRootDir(path string) string {
	parent := filepath.Dir(path)
	if isSeasonFolder(filepath.Base(parent)) {
		return filepath.Dir(parent)
	}
	return parent
}

// groupByNormalizedName groups files by one concrete media identity.
// Movies use title + release year. TV files use series title + release year +
// season/episode, so different episodes are never considered duplicates of each
// other. Directory context is used only when an episode filename has no title.
func (d *Detector) groupByNormalizedName(entries []FileEntry) []DuplicateGroup {
	groups := make(map[string]*DuplicateGroup)
	var keys []string

	for _, entry := range entries {
		// Build parent directory chain for context-aware normalization
		parentDir := filepath.Dir(entry.Path)
		grandparentDir := filepath.Dir(parentDir)
		info := media.NormalizeWithContext(entry.Name, parentDir, grandparentDir)

		if info.Title == "" {
			info.Title = strings.TrimSpace(entry.Name)
		}

		key, normName, epTag, isEp, year := mediaIdentity(entry, info)

		if g, ok := groups[key]; ok {
			g.Files = append(g.Files, entry)
		} else {
			keys = append(keys, key)
			groups[key] = &DuplicateGroup{
				NormalizedName: normName,
				EpisodeTag:     epTag,
				IsEpisode:      isEp,
				Year:           year,
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

func mediaIdentity(entry FileEntry, info media.MediaInfo) (key, normName, epTag string, isEp bool, year int) {
	year = info.Year
	if info.IsEpisode {
		normName = info.Title
		epTag = info.EpisodeTag
		if epTag == "" {
			epTag = formatEpisodeTag(info.Season, info.Episode)
		}
		return identityKey("tv", normName, year, epTag), normName, epTag, true, year
	}

	if n, ok := bareEpisodeNumber(entry.Name); ok {
		normName, year = folderTitle(entry, year)
		epTag = formatEpisodeTag(seasonFromPath(entry.Path), n)
		return identityKey("tv", normName, year, epTag), normName, epTag, true, year
	}

	if n, suffix, ok := leadingEpisodeNumber(entry.Name); ok {
		suffixInfo := media.Normalize(suffix)
		if suffixInfo.Title != "" && !looksLikeEpisodeToken(suffixInfo.Title) {
			normName = suffixInfo.Title
			if suffixInfo.Year > 0 {
				year = suffixInfo.Year
			}
		} else {
			normName, year = folderTitle(entry, year)
		}
		epTag = formatEpisodeTag(seasonFromPath(entry.Path), n)
		return identityKey("tv", normName, year, epTag), normName, epTag, true, year
	}

	normName = info.Title
	if normName == "" {
		normName = strings.TrimSpace(entry.Name)
	}
	return identityKey("movie", normName, year, ""), normName, "", false, year
}

func identityKey(mediaType, title string, year int, episodeTag string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	return fmt.Sprintf("%s||%s||%04d||%s", mediaType, title, year, episodeTag)
}

func folderTitle(e FileEntry, fallbackYear int) (string, int) {
	parentDir := filepath.Dir(e.Path)
	parentFolder := filepath.Base(parentDir)
	folderName := parentFolder
	if isSeasonFolder(parentFolder) {
		folderName = filepath.Base(filepath.Dir(parentDir))
	}
	info := media.Normalize(folderName)
	title := info.Title
	if title == "" {
		title = folderName
	}
	year := fallbackYear
	if info.Year > 0 {
		year = info.Year
	}
	return title, year
}

func looksLikeEpisodeToken(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	return t == "" || reEpisodeToken.MatchString(t)
}

// isBareNumberFile checks if a filename is a bare number (e.g., "01.mkv", "02.mp4").
// hasLeadingEpNumber checks if filename starts with an episode number prefix.
// Examples: "14.枭起青壤.mkv" -> true, "02.剧名.mp4" -> true
func hasLeadingEpNumber(name string) bool {
	_, _, ok := leadingEpisodeNumber(name)
	return ok
}

func leadingEpisodeNumber(name string) (int, string, bool) {
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(name) {
		return 0, "", false
	}
	if name[i] != '.' && name[i] != ' ' && name[i] != '-' && name[i] != '_' {
		return 0, "", false
	}
	n := 0
	for _, r := range name[:i] {
		n = n*10 + int(r-'0')
	}
	suffix := strings.Trim(name[i+1:], " .-_")
	return n, suffix, true
}

func isBareNumberFile(name string) bool {
	_, ok := bareEpisodeNumber(name)
	return ok
}

func bareEpisodeNumber(name string) (int, bool) {
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}
	if name == "" {
		return 0, false
	}
	n := 0
	for _, c := range name {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func seasonFromPath(path string) int {
	parent := filepath.Base(filepath.Dir(path))
	if s, ok := parseSeasonFolder(parent); ok {
		return s
	}
	return 1
}

func formatEpisodeTag(season, episode int) string {
	return fmt.Sprintf("S%02dE%02d", season, episode)
}

func (d *Detector) clusterBySize(files []FileEntry) ([][]FileEntry, []FileEntry) {
	if len(files) < 2 {
		return nil, files
	}
	parent := make([]int, len(files))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if sizeWithinTolerance(files[i].Size, files[j].Size) {
				union(i, j)
			}
		}
	}
	byRoot := make(map[int][]FileEntry)
	for i, f := range files {
		byRoot[find(i)] = append(byRoot[find(i)], f)
	}
	var clusters [][]FileEntry
	var unique []FileEntry
	for _, fs := range byRoot {
		if len(fs) > 1 {
			clusters = append(clusters, fs)
		} else {
			unique = append(unique, fs[0])
		}
	}
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i][0].ID < clusters[j][0].ID
	})
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].ID < unique[j].ID
	})
	return clusters, unique
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

	// Calculate quality fingerprints for all files.
	// Quality-first: keep the best quality version; storage is the tiebreaker.
	type scored struct {
		idx   int
		score int
	}
	scoredFiles := make([]scored, len(files))
	for i, f := range files {
		fp := media.ExtraceFingerprint(f.Name)
		scoredFiles[i] = scored{idx: i, score: fp.Score()}
	}

	// Find the best file: highest quality score, storage as tiebreaker
	bestIdx := scoredFiles[0].idx
	bestScore := scoredFiles[0].score
	bestPriority := storageRank(files[scoredFiles[0].idx].Storage)

	for i := 1; i < len(scoredFiles); i++ {
		si := scoredFiles[i].idx
		pri := storageRank(files[si].Storage)

		if scoredFiles[i].score > bestScore {
			bestScore = scoredFiles[i].score
			bestIdx = si
			bestPriority = pri
		} else if scoredFiles[i].score == bestScore && pri < bestPriority {
			bestIdx = si
			bestPriority = pri
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

	// Same-storage TV episodes are different episodes — keep all.
	// Different storages: keep ALL files from the best storage, delete the rest.
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
	_, ok := parseSeasonFolder(name)
	return ok
}

func parseSeasonFolder(name string) (int, bool) {
	for _, re := range seasonFolderRegexp {
		if re.MatchString(name) {
			digits := reDigits.FindString(name)
			if digits == "" {
				return 1, true
			}
			n := 0
			for _, r := range digits {
				n = n*10 + int(r-'0')
			}
			return n, true
		}
	}
	if m := reChineseSeason.FindStringSubmatch(name); m != nil {
		return chineseNumber(m[1]), true
	}
	return 0, false
}

func chineseNumber(s string) int {
	values := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	if len([]rune(s)) == 1 {
		if v := values[[]rune(s)[0]]; v > 0 {
			return v
		}
	}
	if s == "十" {
		return 10
	}
	total := 0
	if strings.ContainsRune(s, '十') {
		parts := strings.Split(s, "十")
		if parts[0] == "" {
			total = 10
		} else {
			total = values[[]rune(parts[0])[0]] * 10
		}
		if len(parts) > 1 && parts[1] != "" {
			total += values[[]rune(parts[1])[0]]
		}
		return total
	}
	return 1
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
