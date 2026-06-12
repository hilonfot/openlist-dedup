package report

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"openlist/internal/duplicate"
	"openlist/internal/media"
)

type ReportData struct {
	GeneratedAt     string
	MovieGroups     []duplicate.DuplicateGroup
	TVGroups        []duplicate.DuplicateGroup
	Stats           duplicate.Stats
	StorageStats    []StorageStat
	TMDBData        map[string]TMDBItem
	StorageTrees    []StorageTree
	Shows           []ShowEntry  // flat list of TV shows (for display)
	Movies          []MovieEntry // flat list of movies (for display)
	OpenListBaseURL string
}

type ShowEntry struct {
	Name        string
	Path        string
	FileCount   int
	Size        int64
	Storage     string
	DupBadge    string
	PosterURL   string
	TMDBURL     string
	Rating      string
	OpenListURL string
	TMDBTitle   string
}

type MovieEntry struct {
	Name        string
	Path        string
	Size        int64
	Storage     string
	DupBadge    string
	PosterURL   string
	TMDBURL     string
	Rating      string
	OpenListURL string
	TMDBTitle   string
}

type TMDBItem struct {
	PosterURL string
	Overview  string
	Rating    float64
	TMDBURL   string
	Title     string // Chinese title from TMDB
}

type StorageStat struct {
	Name       string
	FileCount  int
	TotalSize  int64
	DupeSize   int64
	Percentage float64
}

type StorageTree struct {
	Name  string
	Nodes []FlatNode
}

type FlatNode struct {
	Name        string
	Path        string
	Indent      int
	IsDir       bool
	IsMovie     bool
	IsTVShow    bool
	FileCount   int
	SeasonCount int
	Size        int64
	PosterURL   string
	TMDBURL     string
	DupBadge    string
	Rating      string
	Overview    string
	OpenListURL string
	TMDBTitle   string
}

type dirNode struct {
	children map[string]*dirNode
	files    []duplicate.FileEntry
}

type storageSummary struct {
	Storage   string
	Decision  string
	FileCount int
	TotalSize int64
	IsKeep    bool
	IsDelete  bool
}

type groupSummary struct {
	Name        string
	EpisodeTag  string
	IsEpisode   bool
	Storages    []storageSummary
	TotalSize   int64
	SavableSize int64
	Files       []dupFileEntry
	Folders     []dupFolderEntry
}

type dupFileEntry struct {
	Name        string
	Path        string
	Storage     string
	Size        int64
	Decision    string
	OpenListURL string
}

type dupFolderEntry struct {
	Name        string
	Path        string
	Storage     string
	FileCount   int
	Size        int64
	Decision    string
	OpenListURL string
}

func BuildFileTree(entries []duplicate.FileEntry, tmdbData map[string]TMDBItem, groups []duplicate.DuplicateGroup, openListURL string) []StorageTree {
	dupDecisions := make(map[int64]string)
	for _, g := range groups {
		for _, f := range g.Files {
			if f.Decision == duplicate.Keep {
				dupDecisions[f.ID] = "keep"
			} else if f.Decision == duplicate.Delete {
				dupDecisions[f.ID] = "delete"
			}
		}
	}
	byStorage := make(map[string][]duplicate.FileEntry)
	for _, e := range entries {
		byStorage[e.Storage] = append(byStorage[e.Storage], e)
	}
	storageOrder := []string{"local", "quark", "tianyi"}
	var result []StorageTree
	for _, name := range storageOrder {
		files, ok := byStorage[name]
		if !ok || len(files) == 0 {
			continue
		}
		nodes := buildEntityTree(files, tmdbData, dupDecisions, openListURL)
		result = append(result, StorageTree{Name: name, Nodes: nodes})
	}
	return result
}

func buildEntityTree(files []duplicate.FileEntry, tmdbData map[string]TMDBItem, decisions map[int64]string, baseURL string) []FlatNode {
	root := &dirNode{children: make(map[string]*dirNode)}
	for _, f := range files {
		parts := splitPath(f.Path)
		node := root
		for _, part := range parts[:len(parts)-1] {
			if child, ok := node.children[part]; ok {
				node = child
			} else {
				child = &dirNode{children: make(map[string]*dirNode)}
				node.children[part] = child
				node = child
			}
		}
		node.files = append(node.files, f)
	}
	var result []FlatNode
	flattenEntities(root, 0, "", tmdbData, decisions, baseURL, &result)
	return result
}

func flattenEntities(dir *dirNode, depth int, prefix string, tmdbData map[string]TMDBItem, decisions map[int64]string, baseURL string, result *[]FlatNode) {
	var dirNames []string
	for name := range dir.children {
		dirNames = append(dirNames, name)
	}
	sort.Strings(dirNames)
	indent := depth * 24

	sort.Slice(dir.files, func(i, j int) bool {
		return dir.files[i].Name < dir.files[j].Name
	})

	// Separate subdirectories into season folders and normal folders
	var seasonDirs, normalDirs []string
	for _, name := range dirNames {
		if isSeasonFolderName(name) {
			seasonDirs = append(seasonDirs, name)
		} else {
			normalDirs = append(normalDirs, name)
		}
	}

	// CASE 1: Has season subdirectories → emit one combined TV show entity
	if len(seasonDirs) > 0 {
		showName := filepath.Base(prefix)
		var totalFiles int
		var totalSize int64
		for _, name := range seasonDirs {
			fc, sz := countFilesRecursive(dir.children[name])
			totalFiles += fc
			totalSize += sz
		}
		tmdb, _ := tmdbData[showName]
		decision := findDecisionInNode(dir, decisions)
		rating := ""
		if tmdb.Rating > 0 {
			rating = fmt.Sprintf("★ %.1f", tmdb.Rating)
		}
		*result = append(*result, FlatNode{
			Name: showName, Path: prefix, Indent: indent, IsTVShow: true,
			FileCount: totalFiles, Size: totalSize,
			PosterURL: tmdb.PosterURL, TMDBURL: tmdb.TMDBURL,
			DupBadge: decision, Rating: rating, Overview: tmdb.Overview,
			OpenListURL: buildOpenListPath(prefix, baseURL),
			TMDBTitle:   tmdbDisplayTitle(tmdb)})
		// Recurse only into non-season subdirectories
		for _, name := range normalDirs {
			child := dir.children[name]
			childPath := prefix + "/" + name
			if prefix == "" {
				childPath = name
			}
			flattenEntities(child, depth+1, childPath, tmdbData, decisions, baseURL, result)
		}
		return
	}

	// CASE 2: No season folders — handle files directly in this directory
	hasEpisodes := false
	hasMovies := false
	for _, f := range dir.files {
		if media.Normalize(f.Name).IsEpisode {
			hasEpisodes = true
		} else {
			hasMovies = true
		}
	}

	// Fallback: check if files share a common prefix (e.g. "剧名 01.mp4")
	if !hasEpisodes && hasMovies && len(dir.files) >= 2 {
		if isNumberedSeries(dir.files) {
			hasEpisodes = true
			hasMovies = false
		}
	}
	if hasEpisodes {
		showName := filepath.Base(prefix)
		if showName == "" && len(dir.files) > 0 {
			showName = filepath.Base(filepath.Dir(dir.files[0].Path))
		}
		var totalSize int64
		for _, f := range dir.files {
			totalSize += f.Size
		}
		tmdb, _ := tmdbData[showName]
		decision := findDecision(dir.files, decisions)
		rating := ""
		if tmdb.Rating > 0 {
			rating = fmt.Sprintf("★ %.1f", tmdb.Rating)
		}
		*result = append(*result, FlatNode{
			Name: showName, Path: prefix, Indent: indent, IsTVShow: true,
			FileCount: len(dir.files), Size: totalSize,
			PosterURL: tmdb.PosterURL, TMDBURL: tmdb.TMDBURL,
			DupBadge: decision, Rating: rating, Overview: tmdb.Overview,
			OpenListURL: buildOpenListPath(prefix, baseURL),
			TMDBTitle:   tmdbDisplayTitle(tmdb)})
	}

	if hasMovies {
		for _, f := range dir.files {
			info := media.Normalize(f.Name)
			if info.IsEpisode {
				continue
			}
			parentName := filepath.Base(filepath.Dir(f.Path))
			tmdb, hasTMDB := tmdbData[parentName]
			if !hasTMDB {
				tmdb, hasTMDB = tmdbData[info.Title]
			}
			decision := decisions[f.ID]
			rating := ""
			if hasTMDB && tmdb.Rating > 0 {
				rating = fmt.Sprintf("★ %.1f", tmdb.Rating)
			}
			*result = append(*result, FlatNode{
				Name: f.Name, Path: f.Path, Indent: indent, IsMovie: true, Size: f.Size,
				PosterURL: tmdb.PosterURL, TMDBURL: tmdb.TMDBURL,
				DupBadge: decision, Rating: rating, Overview: tmdb.Overview,
				OpenListURL: buildOpenListPath(f.Path, baseURL),
				TMDBTitle:   tmdbDisplayTitle(tmdb)})
		}
	}

	// Recurse into normal subdirectories
	for _, name := range normalDirs {
		child := dir.children[name]
		childPath := prefix + "/" + name
		if prefix == "" {
			childPath = name
		}
		flattenEntities(child, depth+1, childPath, tmdbData, decisions, baseURL, result)
	}
}

func countFilesRecursive(node *dirNode) (int, int64) {
	count := len(node.files)
	var size int64
	for _, f := range node.files {
		size += f.Size
	}
	for _, child := range node.children {
		c, s := countFilesRecursive(child)
		count += c
		size += s
	}
	return count, size
}

func findDecision(files []duplicate.FileEntry, decisions map[int64]string) string {
	for _, f := range files {
		if d, ok := decisions[f.ID]; ok {
			return d
		}
	}
	return ""
}

func findDecisionInNode(node *dirNode, decisions map[int64]string) string {
	for _, f := range node.files {
		if d, ok := decisions[f.ID]; ok {
			return d
		}
	}
	for _, child := range node.children {
		if d := findDecisionInNode(child, decisions); d != "" {
			return d
		}
	}
	return ""
}

// isNumberedSeries checks if files in a directory share a common naming pattern.
// Supports: "剧名 01.mp4", "剧名 05.mp4" (same base) AND "01.mkv", "02.mkv" (bare numbers).
func isNumberedSeries(files []duplicate.FileEntry) bool {
	if len(files) < 2 {
		return false
	}
	stripNum := func(name string) string {
		if idx := strings.LastIndex(name, "."); idx > 0 {
			e := strings.ToLower(name[idx+1:])
			switch e {
			case "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v",
				"ts", "mts", "m2ts", "mpeg", "mpg", "3gp", "vob":
				name = name[:idx]
			}
		}
		for len(name) > 0 {
			last := name[len(name)-1]
			if last >= '0' && last <= '9' {
				name = name[:len(name)-1]
			} else if last == ' ' || last == '-' || last == '_' || last == '.' {
				name = name[:len(name)-1]
			} else {
				break
			}
		}
		return strings.TrimSpace(name)
	}
	// Check if ALL files are bare numbers (01.mkv, 02.mkv, etc.)
	allBareNumbers := true
	for _, f := range files {
		if !isBareNumber(f.Name) {
			allBareNumbers = false
			break
		}
	}
	if allBareNumbers {
		return true // All files are numbered → same TV series
	}

	// Check for shared suffix pattern (episode prefix like "14.枭起青壤...")
	// Files: "14.枭起青壤.mkv" and "02.枭起青壤.mkv" -> same suffix "枭起青壤.mkv"
	suffixBase := stripLeadingNum(files[0].Name)
	if suffixBase != "" {
		allMatch := true
		for _, f := range files[1:] {
			if stripLeadingNum(f.Name) != suffixBase {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}

	// Otherwise check for shared prefix (剧名 01.mp4 pattern)
	base := stripNum(files[0].Name)
	if base == "" {
		return false
	}
	for _, f := range files[1:] {
		if stripNum(f.Name) != base {
			return false
		}
	}
	return true
}

// stripLeadingNum removes leading episode numbers and separators.
func stripLeadingNum(name string) string {
	for len(name) > 0 {
		first := name[0]
		if first >= '0' && first <= '9' {
			name = name[1:]
		} else if first == ' ' || first == '.' || first == '-' || first == '_' {
			name = name[1:]
		} else {
			break
		}
	}
	if idx := strings.LastIndex(name, "."); idx > 0 {
		e := strings.ToLower(name[idx+1:])
		switch e {
		case "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v",
			"ts", "mts", "m2ts", "mpeg", "mpg", "3gp", "vob":
			name = name[:idx]
		}
	}
	return name
}

// isBareNumber checks if a filename is a bare number (e.g., "01.mkv", "02.mp4").
func isBareNumber(name string) bool {
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
func isSeasonFolderName(name string) bool {
	if strings.HasPrefix(name, "Season ") {
		return true
	}
	if strings.HasPrefix(name, "S") && len(name) <= 3 {
		return true
	}
	if strings.HasPrefix(name, "第") && strings.HasSuffix(name, "季") {
		return true
	}
	return false
}

func cleanName(name string) string {
	info := media.Normalize(name)
	if info.Title != "" {
		return info.Title
	}
	return name
}
func buildOpenListPath(path, baseURL string) string {
	if baseURL == "" {
		return ""
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return baseURL + "/" + strings.TrimLeft(path, "/")
}

func splitPath(path string) []string {
	cleaned := filepath.Clean(path)
	parts := strings.Split(cleaned, "/")
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
func summarizeGroup(g duplicate.DuplicateGroup, baseURL string) groupSummary {
	byStorage := make(map[string]*storageSummary)
	order := make([]string, 0)
	var files []dupFileEntry
	for _, f := range g.Files {
		s, ok := byStorage[f.Storage]
		if !ok {
			order = append(order, f.Storage)
			byStorage[f.Storage] = &storageSummary{Storage: f.Storage, FileCount: 0, TotalSize: 0}
			s = byStorage[f.Storage]
		}
		s.FileCount++
		s.TotalSize += f.Size
		dec := "Unique"
		if f.Decision == duplicate.Keep {
			s.IsKeep = true
			dec = "Keep"
			s.Decision = "Keep"
		} else if f.Decision == duplicate.Delete {
			s.IsDelete = true
			dec = "Delete"
			s.Decision = "Delete"
		} else {
			s.Decision = "Unique"
		}
		files = append(files, dupFileEntry{Name: f.Name, Path: f.Path, Storage: f.Storage, Size: f.Size, Decision: dec, OpenListURL: buildOpenListPath(f.Path, baseURL)})
	}
	sum := groupSummary{Name: g.NormalizedName, EpisodeTag: g.EpisodeTag, IsEpisode: g.IsEpisode}
	var totalSize, savableSize int64
	for _, name := range order {
		s := byStorage[name]
		sum.Storages = append(sum.Storages, *s)
		totalSize += s.TotalSize
		if s.IsDelete {
			savableSize += s.TotalSize
		}
	}
	sum.TotalSize = totalSize
	sum.SavableSize = savableSize
	sum.Files = files
	if g.IsEpisode {
		sum.Folders = summarizeTVFolders(g.Files, baseURL)
	}
	sort.Slice(sum.Storages, func(i, j int) bool {
		o := map[string]int{"Keep": 0, "Delete": 1, "Unique": 2}
		oi, oj := o[sum.Storages[i].Decision], o[sum.Storages[j].Decision]
		if oi != oj {
			return oi < oj
		}
		return sum.Storages[i].Storage < sum.Storages[j].Storage
	})
	return sum
}

func summarizeTVFolders(files []duplicate.FileEntry, baseURL string) []dupFolderEntry {
	byKey := make(map[string]*dupFolderEntry)
	var order []string
	for _, f := range files {
		dir := tvFolderPath(f.Path)
		key := f.Storage + "||" + dir
		entry, ok := byKey[key]
		if !ok {
			order = append(order, key)
			byKey[key] = &dupFolderEntry{
				Name:        filepath.Base(dir),
				Path:        dir,
				Storage:     f.Storage,
				Decision:    "Unique",
				OpenListURL: buildOpenListPath(dir, baseURL),
			}
			entry = byKey[key]
		}
		entry.FileCount++
		entry.Size += f.Size
		if f.Decision == duplicate.Delete {
			entry.Decision = "Delete"
		} else if f.Decision == duplicate.Keep && entry.Decision != "Delete" {
			entry.Decision = "Keep"
		}
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := byKey[order[i]], byKey[order[j]]
		rank := map[string]int{"Keep": 0, "Delete": 1, "Unique": 2}
		if rank[a.Decision] != rank[b.Decision] {
			return rank[a.Decision] < rank[b.Decision]
		}
		if a.Storage != b.Storage {
			return a.Storage < b.Storage
		}
		return a.Path < b.Path
	})
	result := make([]dupFolderEntry, 0, len(order))
	for _, key := range order {
		result = append(result, *byKey[key])
	}
	return result
}

func tvFolderPath(path string) string {
	dir := filepath.Dir(path)
	if isSeasonFolderName(filepath.Base(dir)) {
		return filepath.Dir(dir)
	}
	return dir
}

func filterDuplicates(groups []duplicate.DuplicateGroup) []duplicate.DuplicateGroup {
	var result []duplicate.DuplicateGroup
	for _, g := range groups {
		hasDelete := false
		for _, f := range g.Files {
			if f.Decision == duplicate.Delete {
				hasDelete = true
				break
			}
		}
		if hasDelete {
			result = append(result, g)
		}
	}
	return result
}

func Generate(path string, data ReportData) error {
	if data.GeneratedAt == "" {
		data.GeneratedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	for _, g := range data.MovieGroups {
		if g.IsEpisode {
			data.TVGroups = append(data.TVGroups, g)
		}
	}
	var movieOnly []duplicate.DuplicateGroup
	for _, g := range data.MovieGroups {
		if !g.IsEpisode {
			movieOnly = append(movieOnly, g)
		}
	}
	data.MovieGroups = movieOnly
	data.MovieGroups = filterDuplicates(data.MovieGroups)
	data.TVGroups = filterDuplicates(data.TVGroups)

	// Build flat show and movie lists from storage trees
	for _, st := range data.StorageTrees {
		for _, n := range st.Nodes {
			if n.IsTVShow {
				data.Shows = append(data.Shows, ShowEntry{
					Name: cleanName(n.Name), Path: n.Path,
					FileCount: n.FileCount, Size: n.Size, Storage: st.Name,
					DupBadge: n.DupBadge, PosterURL: n.PosterURL, TMDBURL: n.TMDBURL,
					Rating: n.Rating, OpenListURL: n.OpenListURL,
					TMDBTitle: n.TMDBTitle})
			} else if n.IsMovie {
				data.Movies = append(data.Movies, MovieEntry{
					Name: cleanName(n.Name), Path: n.Path,
					Size: n.Size, Storage: st.Name,
					DupBadge: n.DupBadge, PosterURL: n.PosterURL, TMDBURL: n.TMDBURL,
					Rating: n.Rating, OpenListURL: n.OpenListURL,
					TMDBTitle: n.TMDBTitle})
			}
		}
	}

	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"formatSize": formatSize, "formatInt": formatInt, "summarize": func(g duplicate.DuplicateGroup) groupSummary { return summarizeGroup(g, data.OpenListBaseURL) },
		"decisionLabel": func(d string) string {
			if d == "Keep" || d == "keep" {
				return "keep"
			}
			if d == "Delete" || d == "delete" {
				return "delete"
			}
			return "unique"
		},
		"add": func(a, b int) int { return a + b },
	}).Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatInt(n int) string {
	if n < 1000 {
		return itoa(n)
	}
	s := ""
	for n > 0 {
		if len(s)%4 == 3 {
			s = "," + s
		}
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

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

// hasChinese reports whether s contains any Chinese characters (CJK Unified Ideographs).
func hasChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// tmdbDisplayTitle returns the TMDB title if it contains Chinese characters,
// so it can be shown preferentially over the file/directory name.
// Returns empty string if the title is not Chinese (let the original name be used).
func tmdbDisplayTitle(t TMDBItem) string {
	if t.Title != "" && hasChinese(t.Title) {
		return t.Title
	}
	return ""
}

const reportTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>OpenList · 媒体库报告</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@500;600;700&family=Inter:wght@400;500;600&display=swap');
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Inter','PingFang SC','Microsoft YaHei',sans-serif;background:#0B0B0E;color:#E8E4DF;min-height:100vh;padding:0}
.container{max-width:960px;margin:0 auto;padding:40px 24px}

/* Header */
.header{padding:0 0 36px}
.header h1{font-size:26px;font-weight:700;letter-spacing:-.4px;color:#F5F0EB;margin-bottom:6px}
.header .sub{font-size:13px;color:#6B6B7A;letter-spacing:.3px}

/* Section */
.section{margin-bottom:32px}
.section-header{display:flex;align-items:center;gap:12px;margin-bottom:16px}
.section-header-line{flex:1;height:1px;background:linear-gradient(90deg,#2C2C36,transparent)}
.section-title{font-size:11px;font-weight:600;color:#7C7C8C;letter-spacing:1.2px;white-space:nowrap}

/* Card */
.card{background:linear-gradient(180deg,#1A1A22 0%,#14141B 100%);border:1px solid #2C2C36;border-radius:14px;overflow:hidden}

/* Stats grid */
.stats-grid{display:grid;grid-template-columns:repeat(5,1fr)}
.stat-cell{text-align:center;padding:28px 10px 24px;position:relative;border-right:1px solid #2C2C36}
.stat-cell:last-child{border-right:none}
.stat-value{font-family:'Space Grotesk',monospace;font-size:32px;font-weight:700;letter-spacing:-1px;color:#F5B342}
.stat-cell.warn .stat-value{color:#FB7185}
.stat-cell.ok .stat-value{color:#34D399}
.stat-label{font-size:10px;color:#6B6B7A;margin-top:6px;letter-spacing:1.2px;font-weight:600}

/* Storage table */
.storage-table{width:100%;border-collapse:collapse;font-size:13px}
.storage-table th{text-align:left;padding:14px 18px;font-weight:600;color:#6B6B7A;font-size:10px;letter-spacing:1px;background:#0F0F14;border-bottom:1px solid #2C2C36}
.storage-table td{padding:12px 18px;border-bottom:1px solid #1F1F28;color:#8C8C9A;font-size:13px}
.storage-table tr:last-child td{border-bottom:none}
.storage-table td:first-child{font-weight:600;color:#F5F0EB}
.bar-container{height:6px;background:#2C2C36;border-radius:3px;overflow:hidden;min-width:60px}
.bar-fill{height:100%;border-radius:3px;transition:width .8s cubic-bezier(.4,0,.2,1)}
.bar-fill.quark{background:linear-gradient(90deg,#14B8A6,#2DD4BF)}
.bar-fill.tianyi{background:linear-gradient(90deg,#0EA5E9,#38BDF8)}
.bar-fill.local{background:linear-gradient(90deg,#A78BFA,#C4B5FD)}

/* Duplicate group cards */
.dup-group{border-bottom:1px solid #1F1F28;padding:18px 20px 14px}
.dup-group:last-child{border-bottom:none}
.dup-header{display:flex;align-items:center;gap:10px;margin-bottom:10px}
.dup-icon{font-size:16px;width:22px;text-align:center;flex-shrink:0}
.dup-name{font-size:15px;font-weight:600;color:#F5F0EB;flex:1;min-width:0}
.dup-count{font-size:12px;color:#6B6B7A;white-space:nowrap;flex-shrink:0}
.dup-storages{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:12px}
.dup-storage-badge{display:inline-flex;align-items:center;gap:6px;font-size:10px;padding:4px 12px;border-radius:20px;font-weight:600;letter-spacing:.3px}
.dup-storage-badge.keep{background:rgba(16,185,129,.1);color:#34D399;border:1px solid rgba(16,185,129,.2)}
.dup-storage-badge.delete{background:rgba(239,68,68,.1);color:#FB7185;border:1px solid rgba(239,68,68,.2)}
.dup-storage-badge .storage-tag{font-size:9px;padding:1px 6px}
.dup-files{margin:0 -20px;padding:0}
.dup-file{display:flex;align-items:center;gap:10px;padding:6px 20px;text-decoration:none;color:inherit;transition:background .15s ease;font-size:13px}
.dup-file:hover{background:rgba(245,179,66,.04)}
.dup-file-name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#8C8C9A}
.dup-file-size{color:#6B6B7A;white-space:nowrap;flex-shrink:0}
.dup-file-decision{font-size:9px;padding:2px 12px;border-radius:12px;font-weight:600;flex-shrink:0;letter-spacing:.3px}
.dup-file-decision.keep{background:rgba(16,185,129,.1);color:#34D399;border:1px solid rgba(16,185,129,.2)}
.dup-file-decision.delete{background:rgba(239,68,68,.1);color:#FB7185;border:1px solid rgba(239,68,68,.2)}
.dup-file-decision.unique{background:rgba(108,108,122,.1);color:#6B6B7A;border:1px solid rgba(108,108,122,.15)}

/* Storage tags */
.storage-tag{display:inline-flex;align-items:center;gap:5px;padding:3px 10px;border-radius:6px;font-size:11px;font-weight:600;letter-spacing:.2px;text-transform:capitalize}
.storage-tag::before{content:'';width:5px;height:5px;border-radius:50%;flex-shrink:0}
.local-tag{background:rgba(167,139,250,.08);color:#C4B5FD;border:1px solid rgba(167,139,250,.15)}
.local-tag::before{background:#A78BFA}
.quark-tag{background:rgba(20,184,166,.08);color:#5EEAD4;border:1px solid rgba(20,184,166,.15)}
.quark-tag::before{background:#14B8A6}
.tianyi-tag{background:rgba(14,165,233,.08);color:#7DD3FC;border:1px solid rgba(14,165,233,.15)}
.tianyi-tag::before{background:#0EA5E9}

/* Poster wall grid */
.poster-wall{display:grid;grid-template-columns:repeat(auto-fill,minmax(155px,1fr));gap:16px;padding:20px}
.poster-card{display:flex;flex-direction:column;text-decoration:none;color:inherit;border-radius:10px;overflow:hidden;transition:transform .25s cubic-bezier(.4,0,.2,1),box-shadow .25s cubic-bezier(.4,0,.2,1);background:#1A1A22;border:1px solid #2C2C36;position:relative}
.poster-card:hover{transform:translateY(-4px) scale(1.02);box-shadow:0 12px 40px rgba(0,0,0,.5),0 0 0 1px rgba(245,179,66,.12);z-index:2}
.poster-img-wrap{position:relative;width:100%;aspect-ratio:2/3;overflow:hidden;background:#2C2C36}
.poster-img{width:100%;height:100%;object-fit:cover;display:block}
.poster-placeholder{width:100%;height:100%;display:flex;align-items:center;justify-content:center;font-size:28px;font-weight:700;color:#6B6B7A;background:linear-gradient(135deg,#2C2C36,#1F1F28)}
.poster-badge{position:absolute;top:8px;right:8px;font-size:9px;padding:2px 10px;border-radius:12px;font-weight:700;letter-spacing:.4px;z-index:1;backdrop-filter:blur(4px)}
.poster-badge-keep{background:rgba(16,185,129,.85);color:#fff;border:1px solid rgba(16,185,129,.3)}
.poster-badge-delete{background:rgba(239,68,68,.85);color:#fff;border:1px solid rgba(239,68,68,.3)}
.poster-badge-unique{display:none}
.poster-rating{position:absolute;top:8px;left:8px;font-size:10px;color:#F5B342;font-weight:700;background:rgba(0,0,0,.6);padding:2px 8px;border-radius:8px;backdrop-filter:blur(4px)}
.poster-info{padding:12px 14px 16px;flex:1;display:flex;flex-direction:column;gap:4px}
.poster-title{font-size:14px;font-weight:600;color:#F5F0EB;line-height:1.3;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.poster-meta{font-size:12px;color:#6B6B7A;line-height:1.4}
.poster-meta .storage-tag{font-size:10px;padding:2px 8px;margin-top:3px}

/* Mobile */
@media(max-width:640px){
.container{padding:24px 16px}
.header h1{font-size:22px}
.stats-grid{grid-template-columns:repeat(3,1fr)}
.stat-cell:nth-child(4),.stat-cell:nth-child(5){border-top:1px solid #2C2C36}
.stat-value{font-size:26px}
.stat-cell{padding:20px 6px 18px}
.poster-wall{grid-template-columns:repeat(auto-fill,minmax(120px,1fr));gap:12px;padding:14px}
.poster-info{padding:10px 12px 14px}
.poster-title{font-size:13px}
.dup-group{padding:14px 14px 10px}
.dup-file{padding:5px 14px;font-size:12px}
}
</style>
</head>
<body>
<div class="container">

<div class="header"><h1>OpenList · 媒体库报告</h1>
<div class="sub">{{.GeneratedAt}} · {{formatInt .Stats.TotalFiles}} 个文件 · 发现 {{formatInt .Stats.DuplicateSets}} 组重复内容</div></div>

<div class="section"><div class="card"><div class="stats-grid">
<div class="stat-cell"><div class="stat-value">{{formatInt .Stats.TotalFiles}}</div><div class="stat-label">文件</div></div>
<div class="stat-cell ok"><div class="stat-value">{{formatInt .Stats.UniqueFiles}}</div><div class="stat-label">唯一</div></div>
<div class="stat-cell warn"><div class="stat-value">{{formatInt .Stats.DuplicateFiles}}</div><div class="stat-label">重复</div></div>
<div class="stat-cell warn"><div class="stat-value">{{formatInt .Stats.DuplicateSets}}</div><div class="stat-label">组</div></div>
<div class="stat-cell ok"><div class="stat-value">{{formatSize .Stats.DuplicateSize}}</div><div class="stat-label">可节省</div></div>
</div></div></div>

{{if .StorageStats}}<div class="section"><div class="section-header"><div class="section-title">存储分布</div><div class="section-header-line"></div></div>
<div class="card"><table class="storage-table"><thead><tr><th>存储</th><th>文件</th><th>总大小</th><th>重复</th><th>占比</th></tr></thead><tbody>
{{range $s := .StorageStats}}<tr><td><span class="storage-tag {{$s.Name}}-tag">{{$s.Name}}</span></td><td>{{formatInt $s.FileCount}}</td><td>{{formatSize $s.TotalSize}}</td><td>{{formatSize $s.DupeSize}}</td><td><div class="bar-container"><div class="bar-fill {{$s.Name}}" style="width:{{printf "%.1f" $s.Percentage}}%"></div></div></td></tr>{{end}}
</tbody></table></div></div>{{end}}

{{if or .MovieGroups .TVGroups}}<div class="section"><div class="section-header"><div class="section-title">重复摘要</div><div class="section-header-line"></div></div>
<div class="card">
{{range $g := .MovieGroups}}{{$s := summarize $g}}<div class="dup-group"><div class="dup-header"><span class="dup-icon">🎬</span><span class="dup-name">{{$s.Name}}</span><span class="dup-count">{{len $s.Files}} 个文件 · {{formatSize $s.TotalSize}}</span></div>
<div class="dup-storages">{{range $st := $s.Storages}}<span class="dup-storage-badge {{decisionLabel $st.Decision}}"><span class="storage-tag {{$st.Storage}}-tag">{{$st.Storage}}</span> {{$st.Decision}}</span>{{end}}</div>
<div class="dup-files">{{range $f := $s.Files}}<a href="{{$f.OpenListURL}}" target="_blank" class="dup-file"><span class="dup-file-name">{{$f.Name}}</span><span class="dup-file-size">{{formatSize $f.Size}} · <span class="storage-tag {{$f.Storage}}-tag">{{$f.Storage}}</span></span><span class="dup-file-decision {{if eq $f.Decision "Keep"}}keep{{else if eq $f.Decision "Delete"}}delete{{else}}unique{{end}}">{{$f.Decision}}</span></a>{{end}}</div></div>
{{end}}
{{range $g := .TVGroups}}{{$s := summarize $g}}<div class="dup-group"><div class="dup-header"><span class="dup-icon">📺</span><span class="dup-name">{{$s.Name}}</span><span class="dup-count">{{len $s.Folders}} 个目录 · {{len $s.Files}} 个文件 · {{formatSize $s.TotalSize}}</span></div>
<div class="dup-storages">{{range $st := $s.Storages}}<span class="dup-storage-badge {{decisionLabel $st.Decision}}"><span class="storage-tag {{$st.Storage}}-tag">{{$st.Storage}}</span> {{$st.Decision}}</span>{{end}}</div>
<div class="dup-files">{{range $f := $s.Folders}}<a href="{{$f.OpenListURL}}" target="_blank" class="dup-file"><span class="dup-file-name">{{$f.Name}}</span><span class="dup-file-size">{{formatInt $f.FileCount}} 集 · {{formatSize $f.Size}} · <span class="storage-tag {{$f.Storage}}-tag">{{$f.Storage}}</span></span><span class="dup-file-decision {{if eq $f.Decision "Keep"}}keep{{else if eq $f.Decision "Delete"}}delete{{else}}unique{{end}}">{{$f.Decision}}</span></a>{{end}}</div></div>
{{end}}
</div></div>{{end}}

{{if .Movies}}<div class="section"><div class="section-header"><div class="section-title">电影 · {{len .Movies}}</div><div class="section-header-line"></div></div>
<div class="card"><div class="poster-wall">
{{range .Movies}}<a href="{{.OpenListURL}}" target="_blank" class="poster-card">
<div class="poster-img-wrap">
{{if .PosterURL}}<img class="poster-img" src="{{.PosterURL}}" loading="lazy">{{else}}<div class="poster-placeholder">{{slice .Name 0 1}}</div>{{end}}
{{if .DupBadge}}<span class="poster-badge poster-badge-{{.DupBadge}}">{{.DupBadge}}</span>{{end}}
{{if .Rating}}<span class="poster-rating">{{.Rating}}</span>{{end}}
</div>
<div class="poster-info"><div class="poster-title">{{if .TMDBTitle}}{{.TMDBTitle}}{{else}}{{.Name}}{{end}}</div>
<div class="poster-meta">{{formatSize .Size}}<br><span class="storage-tag {{.Storage}}-tag">{{.Storage}}</span></div></div>
</a>{{end}}
</div></div></div>{{end}}

{{if .Shows}}<div class="section"><div class="section-header"><div class="section-title">电视剧 · {{len .Shows}}</div><div class="section-header-line"></div></div>
<div class="card"><div class="poster-wall">
{{range .Shows}}<a href="{{.OpenListURL}}" target="_blank" class="poster-card">
<div class="poster-img-wrap">
{{if .PosterURL}}<img class="poster-img" src="{{.PosterURL}}" loading="lazy">{{else}}<div class="poster-placeholder">{{slice .Name 0 1}}</div>{{end}}
{{if .DupBadge}}<span class="poster-badge poster-badge-{{.DupBadge}}">{{.DupBadge}}</span>{{end}}
{{if .Rating}}<span class="poster-rating">{{.Rating}}</span>{{end}}
</div>
<div class="poster-info"><div class="poster-title">{{if .TMDBTitle}}{{.TMDBTitle}}{{else}}{{.Name}}{{end}}</div>
<div class="poster-meta">{{.FileCount}} 集 · {{formatSize .Size}}<br><span class="storage-tag {{.Storage}}-tag">{{.Storage}}</span></div></div>
</a>{{end}}
</div></div></div>{{end}}

</div></body></html>`
