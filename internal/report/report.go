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

	if hasMovies && !hasEpisodes {
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
@import url('https://fonts.googleapis.com/css2?family=Orbitron:wght@500;600;700;800;900&family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500;700&display=swap');

/* ===== RESET & BASE ===== */
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Inter','PingFang SC','Microsoft YaHei',sans-serif;background:#060B14;color:#90A4AE;min-height:100vh;padding:0}
body::before{content:'';position:fixed;inset:0;pointer-events:none;z-index:0;
  background-image:radial-gradient(circle,rgba(0,229,255,.03) 1px,transparent 1px);
  background-size:32px 32px;
  mask-image:radial-gradient(ellipse 60% 50% at 50% 30%,#000 30%,transparent 70%);
  -webkit-mask-image:radial-gradient(ellipse 60% 50% at 50% 30%,#000 30%,transparent 70%);
}
.container{max-width:960px;margin:0 auto;padding:48px 24px;position:relative;z-index:1;counter-reset:sec 0}

/* ===== HERO HEADER ===== */
.hero{display:grid;grid-template-columns:1fr auto;gap:32px;margin-bottom:48px;align-items:end;padding-bottom:32px;border-bottom:1px solid rgba(0,229,255,.08);position:relative}
.hero::after{content:'';position:absolute;bottom:-1px;left:0;width:120px;height:2px;background:linear-gradient(90deg,#00E5FF,rgba(0,229,255,0))}

/* left side: title block */
.hero-l .eyebrow{font-family:'PingFang SC','Microsoft YaHei','Heiti SC',sans-serif;font-size:12px;font-weight:600;letter-spacing:1.5px;color:#00E5FF;margin-bottom:10px}
.hero-l h1{font-family:'PingFang SC','Microsoft YaHei','Heiti SC',sans-serif;font-size:30px;font-weight:700;letter-spacing:6px;color:#E0F7FA;line-height:1.2;margin-bottom:8px}
.hero-l .tagline{font-family:'PingFang SC','Microsoft YaHei','Heiti SC',sans-serif;font-size:12px;color:#607D8B;letter-spacing:.4px}
.hero-l .tagline em{font-style:normal;color:#00E5FF;font-weight:500}

/* right side: status block */
.hero-r{display:flex;flex-direction:column;align-items:flex-end;gap:14px}
.status-row{display:flex;gap:24px;flex-wrap:wrap;justify-content:flex-end}
.status-item{text-align:right}
.status-item .sv{font-family:'Orbitron',monospace;font-size:24px;font-weight:700;color:#00E5FF;letter-spacing:-.5px;text-shadow:0 0 12px rgba(0,229,255,.2)}
.status-item .sl{font-family:'PingFang SC','Microsoft YaHei','Heiti SC',sans-serif;font-size:11px;color:#78909C;letter-spacing:.3px;margin-top:3px}
.status-item.warn .sv{color:#FF5252;text-shadow:0 0 12px rgba(255,82,82,.2)}
.status-item.ok .sv{color:#69F0AE;text-shadow:0 0 12px rgba(105,240,174,.15)}

/* ===== SECTION ===== */
.section{margin-bottom:44px;counter-increment:sec}
.sec-label{display:flex;align-items:center;gap:10px;margin-bottom:14px}
.sec-label .idx{font-family:'Orbitron',sans-serif;font-size:10px;color:#7C4DFF;font-weight:600;letter-spacing:1px;min-width:28px}.sec-label .idx::before{content:counter(sec,decimal-leading-zero)}
.sec-label .name{font-family:'Orbitron',sans-serif;font-size:9px;font-weight:600;color:#546E7A;letter-spacing:1.4px}
.sec-label .line{flex:1;height:1px;background:linear-gradient(90deg,rgba(0,229,255,.12),transparent)}

/* ===== CARD ===== */
.card{background:rgba(10,18,32,.75);border:1px solid rgba(0,229,255,.06);overflow:hidden;backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);position:relative}
.card::before{content:'';position:absolute;inset:0;pointer-events:none;background:linear-gradient(180deg,rgba(0,229,255,.02) 0%,transparent 50%);z-index:1}
.card>*{position:relative;z-index:2}

/* ===== STATS MINI-GRID ===== */
.stats-grid{display:grid;grid-template-columns:repeat(5,1fr)}
.stat-cell{text-align:center;padding:24px 8px 20px;position:relative;border-right:1px solid rgba(0,229,255,.05)}
.stat-cell:last-child{border-right:none}
.stat-value{font-family:'Orbitron',monospace;font-size:28px;font-weight:700;letter-spacing:-.5px;color:#00E5FF}
.stat-cell.warn .stat-value{color:#FF5252}
.stat-cell.ok .stat-value{color:#69F0AE}
.stat-label{font-family:'JetBrains Mono',monospace;font-size:9px;color:#546E7A;margin-top:6px;letter-spacing:.6px}

/* ===== STORAGE TABLE ===== */
.storage-table{width:100%;border-collapse:collapse}
.storage-table th{text-align:left;padding:12px 16px;font-family:'JetBrains Mono',monospace;font-size:9px;font-weight:500;color:#546E7A;letter-spacing:.6px;background:rgba(0,229,255,.02);border-bottom:1px solid rgba(0,229,255,.06)}
.storage-table td{padding:11px 16px;border-bottom:1px solid rgba(0,229,255,.03);font-size:13px;color:#78909C}
.storage-table tr:last-child td{border-bottom:none}
.storage-table tr:hover td{background:rgba(0,229,255,.015)}
.storage-table td:first-child{font-weight:600;color:#B0BEC5}
.bar-container{height:4px;background:rgba(0,229,255,.05);overflow:hidden;min-width:50px}
.bar-fill{height:100%;transition:width 1.2s cubic-bezier(.4,0,.2,1);position:relative}
.bar-fill::after{content:'';position:absolute;inset:0;background:linear-gradient(90deg,transparent 50%,rgba(255,255,255,.2))}
.bar-fill.quark{background:#00E5FF}
.bar-fill.tianyi{background:#7C4DFF}
.bar-fill.local{background:#00E676}

/* ===== TAGS ===== */
.storage-tag{display:inline-flex;align-items:center;gap:4px;padding:2px 8px;font-family:'JetBrains Mono',monospace;font-size:9px;font-weight:500;letter-spacing:.2px;text-transform:uppercase;border:1px solid}
.storage-tag::before{content:'';width:6px;height:6px;flex-shrink:0}
.local-tag{background:rgba(0,230,118,.06);color:#69F0AE;border-color:rgba(0,230,118,.12)}.local-tag::before{background:#00E676}
.quark-tag{background:rgba(0,229,255,.06);color:#00E5FF;border-color:rgba(0,229,255,.12)}.quark-tag::before{background:#00E5FF}
.tianyi-tag{background:rgba(124,77,255,.06);color:#B388FF;border-color:rgba(124,77,255,.12)}.tianyi-tag::before{background:#7C4DFF}

/* ===== DUPLICATE LOG ===== */
.dup-group{border-bottom:1px solid rgba(0,229,255,.04);padding:16px 18px 12px;transition:background .15s}
.dup-group:last-child{border-bottom:none}
.dup-group:hover{background:rgba(0,229,255,.01)}
.dup-header{display:flex;align-items:center;gap:8px;margin-bottom:8px}
.dup-icon{font-size:15px;flex-shrink:0;opacity:.7}
.dup-name{font-size:14px;font-weight:600;color:#B0BEC5;flex:1;min-width:0}
.dup-count{font-family:'JetBrains Mono',monospace;font-size:10px;color:#546E7A;white-space:nowrap}
.dup-storages{display:flex;gap:6px;flex-wrap:wrap;margin-bottom:10px}
.dup-storage-badge{display:inline-flex;align-items:center;gap:5px;font-family:'JetBrains Mono',monospace;font-size:9px;padding:2px 10px;border:1px solid;font-weight:500;letter-spacing:.2px}
.dup-storage-badge.keep{background:rgba(0,230,118,.06);color:#69F0AE;border-color:rgba(0,230,118,.15)}
.dup-storage-badge.delete{background:rgba(255,82,82,.06);color:#FF5252;border-color:rgba(255,82,82,.15)}
.dup-storage-badge .storage-tag{font-size:8px;padding:1px 5px}
.dup-files{margin:0 -18px}
.dup-file{display:flex;align-items:center;gap:8px;padding:5px 18px;text-decoration:none;color:inherit;transition:background .12s;font-size:12px}
.dup-file:hover{background:rgba(0,229,255,.03)}
.dup-file-name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#607D8B}
.dup-file-size{font-family:'JetBrains Mono',monospace;font-size:10px;color:#546E7A;white-space:nowrap}
.dup-file-decision{font-family:'JetBrains Mono',monospace;font-size:8px;padding:1px 8px;font-weight:500;border:1px solid}
.dup-file-decision.keep{background:rgba(0,230,118,.06);color:#69F0AE;border-color:rgba(0,230,118,.15)}
.dup-file-decision.delete{background:rgba(255,82,82,.06);color:#FF5252;border-color:rgba(255,82,82,.15)}
.dup-file-decision.unique{background:transparent;color:#546E7A;border-color:rgba(84,110,122,.12)}

/* ===== POSTER WALL ===== */
.poster-wall{display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:16px;padding:18px}
.poster-card{display:flex;flex-direction:column;text-decoration:none;color:inherit;overflow:hidden;transition:transform .25s cubic-bezier(.4,0,.2,1),box-shadow .25s;background:rgba(10,18,32,.5);border:1px solid rgba(0,229,255,.05);position:relative}
.poster-card:hover{transform:translateY(-4px);box-shadow:0 12px 36px rgba(0,0,0,.5),0 0 0 1px rgba(0,229,255,.15);z-index:2}
.poster-img-wrap{position:relative;width:100%;aspect-ratio:2/3;overflow:hidden;background:rgba(0,229,255,.03)}
.poster-img{width:100%;height:100%;object-fit:cover;display:block;transition:transform .4s cubic-bezier(.4,0,.2,1)}
.poster-card:hover .poster-img{transform:scale(1.05)}
.poster-placeholder{width:100%;height:100%;display:flex;align-items:center;justify-content:center;font-family:'Orbitron',sans-serif;font-size:30px;font-weight:700;color:rgba(0,229,255,.08);background:linear-gradient(135deg,rgba(0,229,255,.03),rgba(124,77,255,.03))}
.poster-badge{position:absolute;top:6px;right:6px;font-family:'JetBrains Mono',monospace;font-size:8px;padding:2px 8px;font-weight:600;letter-spacing:.3px;z-index:1;backdrop-filter:blur(6px)}
.poster-badge-keep{background:rgba(0,230,118,.8);color:#fff}
.poster-badge-delete{background:rgba(255,82,82,.8);color:#fff}
.poster-badge-unique{display:none}
.poster-rating{position:absolute;top:6px;left:6px;font-family:'Orbitron',sans-serif;font-size:9px;color:#00E5FF;font-weight:600;background:rgba(6,11,20,.8);padding:2px 7px;border:1px solid rgba(0,229,255,.12);backdrop-filter:blur(6px)}
.poster-info{padding:12px 12px 14px;flex:1;display:flex;flex-direction:column;gap:4px}
.poster-title{font-size:13px;font-weight:600;color:#B0BEC5;line-height:1.3;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.poster-meta{font-family:'JetBrains Mono',monospace;font-size:10px;color:#546E7A;line-height:1.5}
.poster-meta .storage-tag{font-size:9px;margin-top:2px}

/* ===== MOBILE ===== */
@media(max-width:640px){
.container{padding:28px 12px}
.hero{grid-template-columns:1fr;gap:20px;margin-bottom:32px;padding-bottom:24px}
.hero-r{flex-direction:row;flex-wrap:wrap;gap:18px}
.status-row{justify-content:flex-start}
.status-item{text-align:left}
.status-item .sv{font-size:18px}
.hero-l h1{font-size:22px;letter-spacing:3px}
.stats-grid{grid-template-columns:repeat(3,1fr)}
.stat-cell:nth-child(4),.stat-cell:nth-child(5){border-top:1px solid rgba(0,229,255,.05)}
.stat-value{font-size:22px}
.stat-cell{padding:18px 4px 16px}
.poster-wall{grid-template-columns:repeat(auto-fill,minmax(115px,1fr));gap:10px;padding:12px}
.poster-info{padding:8px 10px 12px}
.poster-title{font-size:12px}
.dup-group{padding:12px 12px 8px}
.dup-file{padding:4px 12px;font-size:11px}
.storage-table th,.storage-table td{padding:9px 10px;font-size:11px}
}
</style>
</head>
<body>
<div class="container">

<!-- HERO HEADER -->
<div class="hero">
<div class="hero-l">
<div class="eyebrow">openlist · 系统仪表盘</div>
<h1>媒体库扫描报告</h1>
<div class="tagline">扫描时间：<em>{{.GeneratedAt}}</em></div>
</div>
<div class="hero-r">
<div class="status-row">
<div class="status-item"><div class="sv">{{formatInt .Stats.TotalFiles}}</div><div class="sl">文件总数</div></div>
<div class="status-item"><div class="sv">{{formatInt .Stats.UniqueFiles}}</div><div class="sl">唯一文件</div></div>
<div class="status-item warn"><div class="sv">{{formatInt .Stats.DuplicateFiles}}</div><div class="sl">重复文件</div></div>
</div>
<div class="status-row">
<div class="status-item warn"><div class="sv">{{formatInt .Stats.DuplicateSets}}</div><div class="sl">重复组数</div></div>
<div class="status-item ok"><div class="sv">{{formatSize .Stats.DuplicateSize}}</div><div class="sl">可节省空间</div></div>
</div>
</div>
</div>

<!-- Duplicate Log -->
{{if or .MovieGroups .TVGroups}}<div class="section">
<div class="sec-label"><span class="idx"></span><span class="name">DUPLICATE_LOG</span><div class="line"></div></div>
<div class="card">
{{range $g := .MovieGroups}}{{$s := summarize $g}}<div class="dup-group"><div class="dup-header"><span class="dup-icon">🎬</span><span class="dup-name">{{$s.Name}}</span><span class="dup-count">{{len $s.Files}} files · {{formatSize $s.TotalSize}}</span></div>
<div class="dup-storages">{{range $st := $s.Storages}}<span class="dup-storage-badge {{decisionLabel $st.Decision}}"><span class="storage-tag {{$st.Storage}}-tag">{{$st.Storage}}</span> {{$st.Decision}}</span>{{end}}</div>
<div class="dup-files">{{range $f := $s.Files}}<a href="{{$f.OpenListURL}}" target="_blank" class="dup-file"><span class="dup-file-name">{{$f.Name}}</span><span class="dup-file-size">{{formatSize $f.Size}} · <span class="storage-tag {{$f.Storage}}-tag">{{$f.Storage}}</span></span><span class="dup-file-decision {{if eq $f.Decision "Keep"}}keep{{else if eq $f.Decision "Delete"}}delete{{else}}unique{{end}}">{{$f.Decision}}</span></a>{{end}}</div></div>
{{end}}
{{range $g := .TVGroups}}{{$s := summarize $g}}<div class="dup-group"><div class="dup-header"><span class="dup-icon">📺</span><span class="dup-name">{{$s.Name}}</span><span class="dup-count">{{len $s.Folders}} dirs · {{len $s.Files}} files · {{formatSize $s.TotalSize}}</span></div>
<div class="dup-storages">{{range $st := $s.Storages}}<span class="dup-storage-badge {{decisionLabel $st.Decision}}"><span class="storage-tag {{$st.Storage}}-tag">{{$st.Storage}}</span> {{$st.Decision}}</span>{{end}}</div>
<div class="dup-files">{{range $f := $s.Folders}}<a href="{{$f.OpenListURL}}" target="_blank" class="dup-file"><span class="dup-file-name">{{$f.Name}}</span><span class="dup-file-size">{{formatInt $f.FileCount}} eps · {{formatSize $f.Size}} · <span class="storage-tag {{$f.Storage}}-tag">{{$f.Storage}}</span></span><span class="dup-file-decision {{if eq $f.Decision "Keep"}}keep{{else if eq $f.Decision "Delete"}}delete{{else}}unique{{end}}">{{$f.Decision}}</span></a>{{end}}</div></div>
{{end}}
</div></div>{{end}}

<!-- Storage Distribution -->
{{if .StorageStats}}<div class="section">
<div class="sec-label"><span class="idx"></span><span class="name">STORAGE_DISTRIBUTION</span><div class="line"></div></div>
<div class="card"><table class="storage-table"><thead><tr><th>STORAGE</th><th>FILES</th><th>TOTAL</th><th>DUPLICATES</th><th>%</th></tr></thead><tbody>
{{range $s := .StorageStats}}<tr><td><span class="storage-tag {{$s.Name}}-tag">{{$s.Name}}</span></td><td>{{formatInt $s.FileCount}}</td><td>{{formatSize $s.TotalSize}}</td><td>{{formatSize $s.DupeSize}}</td><td><div class="bar-container"><div class="bar-fill {{$s.Name}}" style="width:{{printf "%.1f" $s.Percentage}}%"></div></div></td></tr>{{end}}
</tbody></table></div></div>{{end}}

<!-- Movies -->
{{if .Movies}}<div class="section">
<div class="sec-label"><span class="idx"></span><span class="name">MOVIES · {{len .Movies}} ENTRIES</span><div class="line"></div></div>
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

<!-- TV Shows -->
{{if .Shows}}<div class="section">
<div class="sec-label"><span class="idx"></span><span class="name">TV_SERIES · {{len .Shows}} ENTRIES</span><div class="line"></div></div>
<div class="card"><div class="poster-wall">
{{range .Shows}}<a href="{{.OpenListURL}}" target="_blank" class="poster-card">
<div class="poster-img-wrap">
{{if .PosterURL}}<img class="poster-img" src="{{.PosterURL}}" loading="lazy">{{else}}<div class="poster-placeholder">{{slice .Name 0 1}}</div>{{end}}
{{if .DupBadge}}<span class="poster-badge poster-badge-{{.DupBadge}}">{{.DupBadge}}</span>{{end}}
{{if .Rating}}<span class="poster-rating">{{.Rating}}</span>{{end}}
</div>
<div class="poster-info"><div class="poster-title">{{if .TMDBTitle}}{{.TMDBTitle}}{{else}}{{.Name}}{{end}}</div>
<div class="poster-meta">{{.FileCount}} eps · {{formatSize .Size}}<br><span class="storage-tag {{.Storage}}-tag">{{.Storage}}</span></div></div>
</a>{{end}}
</div></div></div>{{end}}

</div></body></html>`
