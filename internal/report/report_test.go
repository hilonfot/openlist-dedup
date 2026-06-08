package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openlist/internal/duplicate"
)

func TestGenerate_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.html")

	data := ReportData{
		GeneratedAt: "2024-01-01 12:00:00",
		MovieGroups: []duplicate.DuplicateGroup{
			{
				NormalizedName: "Avatar",
				Files: []duplicate.FileEntry{
					{ID: 1, Storage: "local", Path: "/local/avatar.mkv", Name: "Avatar.mkv", Size: 2000000000, Decision: duplicate.Keep},
					{ID: 2, Storage: "quark", Path: "/quark/avatar.mkv", Name: "Avatar.mkv", Size: 2000000000, Decision: duplicate.Delete},
				},
			},
		},
		Stats: duplicate.Stats{
			TotalFiles:    2,
			UniqueFiles:   0,
			DuplicateSets: 1,
			DuplicateFiles: 2,
			DuplicateSize: 2000000000,
			KeepFiles:     1,
			DeleteFiles:   1,
		},
	}

	err := Generate(path, data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty report file")
	}

	// Read content and verify key parts
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	html := string(content)

	if !strings.Contains(html, "OpenList 媒体去重报告") {
		t.Error("expected report title in HTML")
	}
	if !strings.Contains(html, "Avatar") {
		t.Error("expected movie name in HTML")
	}
	if !strings.Contains(html, "local") {
		t.Error("expected storage type in HTML")
	}
	if !strings.Contains(html, "GB") {
		t.Error("expected formatted size in HTML")
	}
	if !strings.Contains(html, "2024-01-01") {
		t.Error("expected generation timestamp in HTML")
	}
}

func TestGenerate_EmptyNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.html")

	data := ReportData{
		GeneratedAt: "2024-06-01",
		Stats: duplicate.Stats{
			TotalFiles:  10,
			UniqueFiles: 10,
		},
	}

	err := Generate(path, data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	html := string(content)

	if !strings.Contains(html, "未发现重复资源") {
		t.Error("expected 'no duplicates' message in HTML")
	}
}

func TestGenerate_TVGroups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tv_report.html")

	data := ReportData{
		MovieGroups: []duplicate.DuplicateGroup{
			{
				NormalizedName: "Breaking Bad",
				EpisodeTag:     "S01E01",
				IsEpisode:      true,
				Files: []duplicate.FileEntry{
					{ID: 1, Storage: "local", Path: "/local/bb/s01e01.mkv", Size: 500000000, Decision: duplicate.Keep},
					{ID: 2, Storage: "tianyi", Path: "/tianyi/bb/s01e01.mkv", Size: 500000000, Decision: duplicate.Delete},
				},
			},
		},
		Stats: duplicate.Stats{
			TotalFiles:    2,
			DuplicateSets: 1,
			DuplicateFiles: 2,
			DuplicateSize: 500000000,
			KeepFiles:     1,
			DeleteFiles:   1,
		},
	}

	err := Generate(path, data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	html := string(content)

	if !strings.Contains(html, "重复剧集") {
		t.Error("expected TV section in HTML")
	}
	if !strings.Contains(html, "Breaking Bad") {
		t.Error("expected TV show name in HTML")
	}
	if !strings.Contains(html, "S01E01") {
		t.Error("expected episode tag in HTML")
	}
}

func TestGenerate_StorageStats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "storage.html")

	data := ReportData{
		StorageStats: []StorageStat{
			{Name: "local", FileCount: 10, TotalSize: 10000000000, DupeSize: 2000000000},
			{Name: "quark", FileCount: 20, TotalSize: 30000000000, DupeSize: 5000000000},
			{Name: "tianyi", FileCount: 5, TotalSize: 5000000000, DupeSize: 0},
		},
		Stats: duplicate.Stats{
			TotalFiles:  35,
			UniqueFiles: 25,
			DuplicateFiles: 10,
		},
	}

	err := Generate(path, data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	html := string(content)

	if !strings.Contains(html, "存储统计") {
		t.Error("expected storage stats section")
	}
	if !strings.Contains(html, "quark") {
		t.Error("expected quark storage stats")
	}
}

func TestGenerate_ValidHTMLStructure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.html")

	data := ReportData{
		GeneratedAt: "2024-06-15 10:30:00",
		Stats: duplicate.Stats{
			TotalFiles:  5,
			UniqueFiles: 3,
			DuplicateFiles: 2,
		},
	}

	err := Generate(path, data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	html := string(content)

	// Basic HTML structure validation
	checks := []struct {
		name string
		fn   func(string) bool
	}{
		{"has doctype", func(s string) bool { return strings.HasPrefix(s, "<!DOCTYPE html>") }},
		{"has html tag", func(s string) bool { return strings.Contains(s, "<html") }},
		{"has head", func(s string) bool { return strings.Contains(s, "<head>") }},
		{"has body", func(s string) bool { return strings.Contains(s, "<body>") }},
		{"has closing html", func(s string) bool { return strings.Contains(s, "</html>") }},
		{"has meta charset", func(s string) bool { return strings.Contains(s, `charset="UTF-8"`) || strings.Contains(s, `charset="utf-8"`) }},
		{"has viewport", func(s string) bool { return strings.Contains(s, "viewport") }},
	}

	for _, c := range checks {
		if !c.fn(html) {
			t.Errorf("HTML structure check failed: %s", c.name)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.input)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
		{123456789, "123,456,789"},
	}
	for _, tt := range tests {
		got := formatInt(tt.input)
		if got != tt.want {
			t.Errorf("formatInt(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerate_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.html")

	data := ReportData{
		Stats: duplicate.Stats{TotalFiles: 1},
	}

	if err := Generate(path, data); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// Should be readable by owner
	if info.Mode().Perm()&0400 == 0 {
		t.Error("expected file to be readable by owner")
	}
}

func TestSeparateMovieAndTV(t *testing.T) {
	// Test that Generate correctly separates Movie and TV groups
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.html")

	movieGroup := duplicate.DuplicateGroup{
		NormalizedName: "The Matrix",
		IsEpisode:     false,
		Files: []duplicate.FileEntry{
			{ID: 1, Storage: "local", Path: "/m.mkv", Size: 100, Decision: duplicate.Keep},
		},
	}
	tvGroup := duplicate.DuplicateGroup{
		NormalizedName: "The Last of Us",
		EpisodeTag:     "S01E01",
		IsEpisode:      true,
		Files: []duplicate.FileEntry{
			{ID: 2, Storage: "tianyi", Path: "/t.mkv", Size: 100, Decision: duplicate.Keep},
		},
	}

	data := ReportData{
		MovieGroups: []duplicate.DuplicateGroup{movieGroup, tvGroup},
		Stats:       duplicate.Stats{TotalFiles: 2},
	}

	err := Generate(path, data)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	html := string(content)

	if !strings.Contains(html, "重复电影") {
		t.Error("expected movie section")
	}
	if !strings.Contains(html, "重复剧集") {
		t.Error("expected TV section")
	}
}
