package duplicate

import (
	"testing"
)

// --- 辅助函数 ---

func makeFile(id int64, storage, name, path string, size int64) FileEntry {
	return FileEntry{
		ID:      id,
		Storage: storage,
		Name:    name,
		Path:    path,
		Size:    size,
	}
}

func TestDetect_NoFiles(t *testing.T) {
	d := New()
	groups, stats := d.Detect(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
	if stats.TotalFiles != 0 {
		t.Errorf("expected 0 total files, got %d", stats.TotalFiles)
	}

	groups, stats = d.Detect([]FileEntry{})
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for empty slice, got %d", len(groups))
	}
	if stats.TotalFiles != 0 {
		t.Errorf("expected 0 total files, got %d", stats.TotalFiles)
	}
}

func TestDetect_SingleFile(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "local", "Avatar.2022.1080p.mkv", "/movies/avatar.mkv", 1000000),
	}

	groups, stats := d.Detect(entries)
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
	if len(groups) > 0 && len(groups[0].Files) != 1 {
		t.Errorf("expected 1 file in group, got %d", len(groups[0].Files))
	}
	if stats.UniqueFiles != 1 {
		t.Errorf("expected 1 unique file, got %d", stats.UniqueFiles)
	}
	if stats.DuplicateSets != 0 {
		t.Errorf("expected 0 duplicate sets, got %d", stats.DuplicateSets)
	}
	if len(groups) > 0 && groups[0].Files[0].Decision != Unique {
		t.Errorf("expected Unique decision, got %s", groups[0].Files[0].Decision)
	}
}

func TestDetect_DuplicateMovies_CrossStorage(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "local", "Avatar.2022.1080p.BluRay.x265.mkv", "/local/videos/avatar.mkv", 2048576000),
		makeFile(2, "quark", "Avatar.2022.1080p.BluRay.x265.mkv", "/quark/movies/avatar.mkv", 2048576000),
	}

	groups, stats := d.Detect(entries)

	// Should find 1 duplicate group with 2 files
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	files := groups[0].Files
	if len(files) != 2 {
		t.Fatalf("expected 2 files in group, got %d", len(files))
	}

	if stats.DuplicateSets != 1 {
		t.Errorf("expected 1 duplicate set, got %d", stats.DuplicateSets)
	}
	if stats.DuplicateFiles != 2 {
		t.Errorf("expected 2 duplicate files, got %d", stats.DuplicateFiles)
	}
	if stats.KeepFiles != 1 {
		t.Errorf("expected 1 Keep, got %d", stats.KeepFiles)
	}
	if stats.DeleteFiles != 1 {
		t.Errorf("expected 1 Delete, got %d", stats.DeleteFiles)
	}

	// Local should be Keep, Quark should be Delete
	for _, f := range files {
		if f.Storage == "local" && f.Decision != Keep {
			t.Errorf("expected local file to be Keep, got %s", f.Decision)
		}
		if f.Storage == "quark" && f.Decision != Delete {
			t.Errorf("expected quark file to be Delete, got %s", f.Decision)
		}
	}
}

func TestDetect_DuplicateMovies_ThreeStorages(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "quark", "The.Matrix.1999.4K.mkv", "/quark/matrix.mkv", 5000000000),
		makeFile(2, "local", "The.Matrix.1999.4K.mkv", "/local/matrix.mkv", 5000000000),
		makeFile(3, "tianyi", "The.Matrix.1999.4K.mkv", "/tianyi/matrix.mkv", 5000000000),
	}

	groups, stats := d.Detect(entries)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if stats.DuplicateSets != 1 {
		t.Errorf("expected 1 duplicate set, got %d", stats.DuplicateSets)
	}
	if stats.KeepFiles != 1 {
		t.Errorf("expected 1 Keep, got %d", stats.KeepFiles)
	}
	if stats.DeleteFiles != 2 {
		t.Errorf("expected 2 Delete, got %d", stats.DeleteFiles)
	}

	// Local should be Keep (highest priority)
	for _, f := range groups[0].Files {
		if f.Storage == "local" && f.Decision != Keep {
			t.Errorf("expected local to be Keep, got %s", f.Decision)
		}
		if f.Storage != "local" && f.Decision != Delete {
			t.Errorf("expected %s to be Delete, got %s", f.Storage, f.Decision)
		}
	}
}

func TestDetect_DifferentMovies(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "local", "Avatar.2022.1080p.mkv", "/movies/avatar.mkv", 2000000),
		makeFile(2, "local", "The.Matrix.1999.4K.mkv", "/movies/matrix.mkv", 3000000),
	}

	groups, stats := d.Detect(entries)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if stats.UniqueFiles != 2 {
		t.Errorf("expected 2 unique files, got %d", stats.UniqueFiles)
	}
	if stats.DuplicateSets != 0 {
		t.Errorf("expected 0 duplicate sets, got %d", stats.DuplicateSets)
	}
}

func TestDetect_DuplicateEpisode(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "local", "Breaking.Bad.S01E01.1080p.mkv", "/local/bb/s01e01.mkv", 500000000),
		makeFile(2, "quark", "Breaking.Bad.S01E01.1080p.mkv", "/quark/bb/s01e01.mkv", 500000000),
	}

	groups, stats := d.Detect(entries)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if !groups[0].IsEpisode {
		t.Errorf("expected IsEpisode true")
	}
	if groups[0].EpisodeTag != folderLevelTag {
		t.Errorf("expected EpisodeTag S01E01, got %s", groups[0].EpisodeTag)
	}
	if stats.DuplicateSets != 1 {
		t.Errorf("expected 1 duplicate set, got %d", stats.DuplicateSets)
	}
}

func TestDetect_DifferentEpisodes(t *testing.T) {
	d := New()
	// Same show folder on different storages → should be in same group (folder-level dedup)
	// Different show folders → should be in different groups
	entries := []FileEntry{
		makeFile(1, "quark", "狂飙_S01E01.mp4", "/quark/电视剧/狂飙/狂飙_S01E01.mp4", 500000000),
		makeFile(2, "quark", "狂飙_S01E02.mp4", "/quark/电视剧/狂飙/狂飙_S01E02.mp4", 500000000),
		makeFile(3, "tianyi", "狂飙_S01E01.mp4", "/tianyi/电视剧/狂飙/狂飙_S01E01.mp4", 500000000),
		makeFile(4, "quark", "漫长的季节_S01E01.mp4", "/quark/电视剧/漫长的季节/漫长的季节_S01E01.mp4", 800000000),
	}

	groups, stats := d.Detect(entries)

	// 狂飙 from quark and tianyi should be in one folder-level group
	// 漫长的季节 should be another group (different folder)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (狂飙 folder, 漫长的季节 folder), got %d", len(groups))
	}
	if stats.DuplicateSets != 1 {
		t.Errorf("expected 1 duplicate set (狂飙 across storages), got %d", stats.DuplicateSets)
	}
	if stats.UniqueFiles != 1 {
		t.Errorf("expected 1 unique file (漫长的季节), got %d", stats.UniqueFiles)
	}
}

func TestDetect_SizeTolerance_Within1Percent(t *testing.T) {
	d := New()
	// 1000000 vs 1005000 = 0.5% difference → should be duplicate
	entries := []FileEntry{
		makeFile(1, "local", "Movie.2020.1080p.mkv", "/local/movie.mkv", 1000000),
		makeFile(2, "quark", "Movie.2020.1080p.mkv", "/quark/movie.mkv", 1005000),
	}

	groups, _ := d.Detect(entries)
	if len(groups) != 1 || len(groups[0].Files) != 2 {
		t.Errorf("expected duplicate detection within 0.5%% tolerance")
	}
}

func TestDetect_SizeTolerance_Exceeds1Percent(t *testing.T) {
	d := New()
	// 1000000 vs 1100000 = 9.1% difference → should NOT be duplicate
	entries := []FileEntry{
		makeFile(1, "local", "Movie.2020.1080p.mkv", "/local/movie.mkv", 1000000),
		makeFile(2, "quark", "Movie.2020.1080p.mkv", "/quark/movie.mkv", 1100000),
	}

	groups, stats := d.Detect(entries)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	// Files should not be size-matched → remain unique
	if stats.UniqueFiles != 2 {
		t.Errorf("expected 2 unique files (sizes differ by >1%%), got %d unique", stats.UniqueFiles)
	}
}

func TestDetect_UnknownStoragePriority(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "local", "Movie.2020.mkv", "/local/movie.mkv", 1000),
		makeFile(2, "unknown", "Movie.2020.mkv", "/unknown/movie.mkv", 1000),
	}

	groups, stats := d.Detect(entries)
	if stats.DuplicateSets != 1 {
		t.Fatalf("expected 1 duplicate set, got %d", stats.DuplicateSets)
	}
	if stats.KeepFiles != 1 {
		t.Errorf("expected 1 Keep, got %d", stats.KeepFiles)
	}
	if stats.DeleteFiles != 1 {
		t.Errorf("expected 1 Delete, got %d", stats.DeleteFiles)
	}

	// Local should be Keep
	for _, f := range groups[0].Files {
		if f.Storage == "local" && f.Decision != Keep {
			t.Errorf("expected local to be Keep, got %s", f.Decision)
		}
		if f.Storage == "unknown" && f.Decision != Delete {
			t.Errorf("expected unknown to be Delete, got %s", f.Decision)
		}
	}
}

func TestDetect_MultipleDuplicateGroups(t *testing.T) {
	d := New()
	entries := []FileEntry{
		// Group 1: Avatar
		makeFile(1, "local", "Avatar.2022.1080p.mkv", "/local/avatar.mkv", 2000000),
		makeFile(2, "quark", "Avatar.2022.1080p.mkv", "/quark/avatar.mkv", 2000000),
		// Group 2: Matrix
		makeFile(3, "local", "The.Matrix.1999.4K.mkv", "/local/matrix.mkv", 3000000),
		makeFile(4, "tianyi", "The.Matrix.1999.4K.mkv", "/tianyi/matrix.mkv", 3000000),
		// Unique file
		makeFile(5, "local", "Inception.2010.BluRay.mkv", "/local/inception.mkv", 1500000),
	}

	groups, stats := d.Detect(entries)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if stats.DuplicateSets != 2 {
		t.Errorf("expected 2 duplicate sets, got %d", stats.DuplicateSets)
	}
	if stats.DuplicateFiles != 4 {
		t.Errorf("expected 4 duplicate files, got %d", stats.DuplicateFiles)
	}
	if stats.UniqueFiles != 1 {
		t.Errorf("expected 1 unique file, got %d", stats.UniqueFiles)
	}
	if stats.KeepFiles != 2 {
		t.Errorf("expected 2 Keep, got %d", stats.KeepFiles)
	}
	if stats.DeleteFiles != 2 {
		t.Errorf("expected 2 Delete, got %d", stats.DeleteFiles)
	}
}

func TestDetect_SameNameDifferentSizeNoMatch(t *testing.T) {
	d := New()
	// Same normalized name but wildly different sizes → not duplicates
	entries := []FileEntry{
		makeFile(1, "local", "Movie.2020.1080p.mkv", "/local/movie.mkv", 1000000),
		makeFile(2, "local", "Movie.2020.720p.mkv", "/local/movie_720p.mkv", 500000),
	}

	groups, stats := d.Detect(entries)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if stats.DuplicateSets != 0 {
		t.Errorf("expected 0 duplicate sets (sizes differ), got %d", stats.DuplicateSets)
	}
	if stats.UniqueFiles != 2 {
		t.Errorf("expected 2 unique files, got %d", stats.UniqueFiles)
	}
}

func TestDetect_ZeroSizedFiles(t *testing.T) {
	d := New()
	// Both zero-sized should match
	entries := []FileEntry{
		makeFile(1, "local", "Movie.mkv", "/local/movie.mkv", 0),
		makeFile(2, "quark", "Movie.mkv", "/quark/movie.mkv", 0),
	}

	groups, stats := d.Detect(entries)
	if stats.DuplicateSets != 1 {
		t.Errorf("expected 1 duplicate set for zero-sized files, got %d", stats.DuplicateSets)
	}
	if len(groups) != 1 || len(groups[0].Files) != 2 {
		t.Errorf("expected both zero-sized files in group")
	}
}

func TestDetect_DuplicateSize(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "quark", "Big.Movie.2024.mkv", "/quark/big.mkv", 5000000000),
		makeFile(2, "local", "Big.Movie.2024.mkv", "/local/big.mkv", 5000000000),
	}

	_, stats := d.Detect(entries)
	if stats.DuplicateSize != 5000000000 {
		t.Errorf("expected 5GB duplicate size, got %d", stats.DuplicateSize)
	}
}

func TestSizeWithinTolerance(t *testing.T) {
	tests := []struct {
		a, b  int64
		want  bool
	}{
		{1000, 1000, true},
		{1000, 1005, true},  // 0.5%
		{1000, 1009, true},  // 0.9%
		{1000, 1011, false}, // 1.1% > 1%
		{1000, 1100, false}, // 9.1%
		{0, 0, true},
		{0, 100, false},
		{100, 0, false},
		{1000000, 1005000, true},  // 0.5%
		{1000000, 1011000, false}, // 1.1% > 1%
	}
	for _, tt := range tests {
		got := sizeWithinTolerance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("sizeWithinTolerance(%d, %d) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestStorageRank(t *testing.T) {
	tests := []struct {
		storage string
		want    int
	}{
		{"local", 0},
		{"tianyi", 1},
		{"quark", 2},
		{"unknown", 3},
	}
	for _, tt := range tests {
		got := storageRank(tt.storage)
		if got != tt.want {
			t.Errorf("storageRank(%q) = %d, want %d", tt.storage, got, tt.want)
		}
	}
}

func TestFormatGroupOutput(t *testing.T) {
	group := DuplicateGroup{
		NormalizedName: "Avatar",
		IsEpisode:      false,
		Files: []FileEntry{
			{ID: 1, Storage: "local", Path: "/local/avatar.mkv", Name: "Avatar.mkv", Size: 1000, Decision: Keep},
			{ID: 2, Storage: "quark", Path: "/quark/avatar.mkv", Name: "Avatar.mkv", Size: 1000, Decision: Delete},
		},
	}

	output := FormatGroupOutput(group)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestDetect_ThreeWayDuplicate(t *testing.T) {
	d := New()
	entries := []FileEntry{
		makeFile(1, "quark", "Movie.2020.1080p.mkv", "/quark/a.mkv", 1000000),
		makeFile(2, "tianyi", "Movie.2020.1080p.mkv", "/tianyi/a.mkv", 1000000),
		makeFile(3, "quark", "Movie.2020.1080p.mkv", "/quark/b.mkv", 1000000),
		// Different size, same name
		makeFile(4, "local", "Movie.2020.720p.mkv", "/local/a_720p.mkv", 500000),
	}

	groups, stats := d.Detect(entries)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Group with all same sizes should have 3 duplicates
	for _, g := range groups {
		if len(g.Files) == 3 {
			// Three files with same size: 2 quark + 1 tianyi
			// Tianyi should be Keep (priority 1 > quark's 2)
			keeps := 0
			deletes := 0
			for _, f := range g.Files {
				if f.Decision == Keep {
					keeps++
				} else if f.Decision == Delete {
					deletes++
				}
			}
			if keeps != 1 {
				t.Errorf("expected 1 Keep in 3-way group, got %d", keeps)
			}
			if deletes != 2 {
				t.Errorf("expected 2 Delete in 3-way group, got %d", deletes)
			}
		}
	}

	if stats.DuplicateSets != 1 {
		t.Errorf("expected 1 duplicate set, got %d", stats.DuplicateSets)
	}
	if stats.UniqueFiles != 1 {
		t.Errorf("expected 1 unique file (720p different size), got %d", stats.UniqueFiles)
	}
}
