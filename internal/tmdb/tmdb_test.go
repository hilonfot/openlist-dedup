package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openlist/internal/repository"
)

// mockTMDBServer simulates TMDB API responses for testing.
type mockTMDBServer struct {
	server *httptest.Server
}

func newMockTMDB(t *testing.T, handler http.HandlerFunc) *mockTMDBServer {
	t.Helper()
	return &mockTMDBServer{
		server: httptest.NewServer(handler),
	}
}

func (m *mockTMDBServer) URL() string {
	return m.server.URL
}

func (m *mockTMDBServer) Close() {
	m.server.Close()
}

func newTestClient(t *testing.T, apiURL string, cache *repository.DB) *Client {
	t.Helper()
	return &Client{
		apiKey:  "test_key",
		baseURL: apiURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		cache:     cache,
		cacheTTL:  defaultCacheTTL,
		rateLimit: time.NewTicker(time.Millisecond), // no real rate limit in tests
	}
}

func TestSearchMovie_Found(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("api_key") != "test_key" {
			http.Error(w, "unauthorized", 401)
			return
		}
		json.NewEncoder(w).Encode(searchResponse{
			Results: []SearchResult{
				{ID: 550, Title: "Fight Club", ReleaseDate: "1999-10-15", Overview: "An insomniac...", VoteAverage: 8.4},
				{ID: 123, Title: "Fight Club 2", ReleaseDate: "2016-01-01", Overview: "Sequel...", VoteAverage: 6.0},
			},
		})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchMovie(context.Background(), "Fight Club", 1999)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TMDBID != 550 {
		t.Errorf("expected TMDBID 550, got %d", result.TMDBID)
	}
	if result.ReleaseYear != 1999 {
		t.Errorf("expected ReleaseYear 1999, got %d", result.ReleaseYear)
	}
	if result.Title != "Fight Club" {
		t.Errorf("expected title 'Fight Club', got %s", result.Title)
	}
	if result.FromCache {
		t.Error("expected FromCache=false for first request")
	}
}

func TestSearchMovie_NotFound(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(searchResponse{Results: []SearchResult{}})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchMovie(context.Background(), "NonExistentMovieXYZ", 0)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for non-existent movie, got %+v", result)
	}
}

func TestSearchMovie_NoYear(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(searchResponse{
			Results: []SearchResult{
				{ID: 680, Title: "Pulp Fiction", ReleaseDate: "1994-09-10", VoteAverage: 8.5},
			},
		})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchMovie(context.Background(), "Pulp Fiction", 0)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TMDBID != 680 {
		t.Errorf("expected TMDBID 680, got %d", result.TMDBID)
	}
}

func TestSearchTV_Found(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(searchResponse{
			Results: []SearchResult{
				{ID: 1396, Name: "Breaking Bad", FirstAirDate: "2008-01-20", Overview: "A high school teacher...", VoteAverage: 8.9},
				{ID: 1234, Name: "Better Call Saul", FirstAirDate: "2015-02-08", VoteAverage: 8.5},
			},
		})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchTV(context.Background(), "Breaking Bad", 0, 2008)
	if err != nil {
		t.Fatalf("SearchTV failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TMDBID != 1396 {
		t.Errorf("expected TMDBID 1396, got %d", result.TMDBID)
	}
	if result.FirstAirYear != 2008 {
		t.Errorf("expected FirstAirYear 2008, got %d", result.FirstAirYear)
	}
	if result.Name != "Breaking Bad" {
		t.Errorf("expected Name 'Breaking Bad', got %s", result.Name)
	}
}

func TestSearchTV_NoYear(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(searchResponse{
			Results: []SearchResult{
				{ID: 1668, Name: "Friends", FirstAirDate: "1994-09-22", VoteAverage: 8.6},
			},
		})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchTV(context.Background(), "Friends", 0, 0)
	if err != nil {
		t.Fatalf("SearchTV failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TMDBID != 1668 {
		t.Errorf("expected TMDBID 1668, got %d", result.TMDBID)
	}
}

func TestSearchMovie_CacheHit(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("Open DB failed: %v", err)
	}
	defer db.Close()

	// Pre-populate cache
	db.SaveTMDBCache(ctx, "Fight Club", "movie", 550, `{"tmdbid":550,"title":"Fight Club","releaseyear":1999}`)

	// Create mock server that should NOT be called
	callCount := 0
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), db)
	result, err := client.SearchMovie(ctx, "Fight Club", 0)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil cached result")
	}
	if callCount != 0 {
		t.Error("expected 0 API calls (cache hit)")
	}
	if !result.FromCache {
		t.Error("expected FromCache=true")
	}
	if result.TMDBID != 550 {
		t.Errorf("expected TMDBID 550, got %d", result.TMDBID)
	}
}

func TestSearchTV_CacheHit(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("Open DB failed: %v", err)
	}
	defer db.Close()

	db.SaveTMDBCache(ctx, "Breaking Bad (2008)", "tv", 1396, `{"tmdbid":1396,"name":"Breaking Bad","firstairyear":2008}`)

	callCount := 0
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), db)
	result, err := client.SearchTV(ctx, "Breaking Bad", 0, 2008)
	if err != nil {
		t.Fatalf("SearchTV failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil cached result")
	}
	if callCount != 0 {
		t.Error("expected 0 API calls (cache hit)")
	}
	if !result.FromCache {
		t.Error("expected FromCache=true")
	}
}

func TestSearchMovie_CacheMissThenFill(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("Open DB failed: %v", err)
	}
	defer db.Close()

	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		lang := q.Get("language")
		if strings.Contains(lang, "zh") {
			// Return empty for Chinese (simulate no Chinese result)
			json.NewEncoder(w).Encode(searchResponse{Results: []SearchResult{}})
			return
		}
		json.NewEncoder(w).Encode(searchResponse{
			Results: []SearchResult{
				{ID: 155, Title: "The Dark Knight", ReleaseDate: "2008-07-18", VoteAverage: 8.5},
			},
		})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), db)
	result, err := client.SearchMovie(ctx, "The Dark Knight", 0)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TMDBID != 155 {
		t.Errorf("expected TMDBID 155, got %d", result.TMDBID)
	}

	// Second call should use cache
	callCount := 0
	mock2 := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
	})
	defer mock2.Close()

	client2 := newTestClient(t, mock2.URL(), db)
	result2, err := client2.SearchMovie(ctx, "The Dark Knight", 0)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result2 == nil {
		t.Fatal("expected non-nil result on second call")
	}
	if !result2.FromCache {
		t.Error("expected FromCache=true on second call")
	}
	if callCount != 0 {
		t.Error("expected 0 API calls on second call (cached)")
	}
}

func TestSearchMovie_ChineseFallback(t *testing.T) {
	// Simulate Chinese not available, English works
	langCalls := []string{}
	callPaths := []string{}
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		callPaths = append(callPaths, r.URL.Path)
		lang := r.URL.Query().Get("language")
		langCalls = append(langCalls, lang)

		if strings.HasPrefix(r.URL.Path, "/movie/") && !strings.HasPrefix(r.URL.Path, "/search/") {
			// Details endpoint: return Chinese title
			json.NewEncoder(w).Encode(movieDetails{
				ID: 550, Title: "搏击俱乐部", Overview: "An insomniac...",
			})
			return
		}

		if strings.Contains(lang, "zh") {
			json.NewEncoder(w).Encode(searchResponse{Results: []SearchResult{}})
			return
		}
		json.NewEncoder(w).Encode(searchResponse{
			Results: []SearchResult{
				{ID: 550, Title: "Fight Club", ReleaseDate: "1999-10-15", VoteAverage: 8.4},
			},
		})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchMovie(context.Background(), "Fight Club", 0)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(langCalls) != 3 {
		t.Errorf("expected 3 API calls (zh-CN search + en-US search + zh-CN details), got %d: %v", len(langCalls), langCalls)
	}
	// Title should be Chinese from details endpoint, not English
	if result.Title != "搏击俱乐部" {
		t.Errorf("expected Chinese title '搏击俱乐部', got %q", result.Title)
	}
}

func TestSearchMovie_ChineseSuccess(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("language")
		if strings.Contains(lang, "zh") {
			json.NewEncoder(w).Encode(searchResponse{
				Results: []SearchResult{
					{ID: 12345, Title: "让子弹飞", ReleaseDate: "2010-01-01", VoteAverage: 8.0},
				},
			})
			return
		}
		// Should not reach English
		t.Error("English search was called despite Chinese success")
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchMovie(context.Background(), "让子弹飞", 0)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TMDBID != 12345 {
		t.Errorf("expected TMDBID 12345, got %d", result.TMDBID)
	}
}

func TestSearchMovie_APIError(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status_message":"Invalid API key"}`, 401)
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	_, err := client.SearchMovie(context.Background(), "Test", 0)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestExtractYear(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1999-10-15", 1999},
		{"2008-01-20", 2008},
		{"2024", 2024},
		{"", 0},
		{"invalid", 0},
		{"99-01-01", 0},
	}
	for _, tt := range tests {
		got := extractYear(tt.input)
		if got != tt.want {
			t.Errorf("extractYear(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name string
		year int
		want string
	}{
		{"Fight Club", 1999, "Fight Club (1999)"},
		{"Fight Club", 0, "Fight Club"},
	}
	for _, tt := range tests {
		got := cacheKey(tt.name, tt.year)
		if got != tt.want {
			t.Errorf("cacheKey(%q, %d) = %q, want %q", tt.name, tt.year, got, tt.want)
		}
	}
}

func TestNewWithConfig(t *testing.T) {
	db, _ := repository.Open(":memory:")
	defer db.Close()

	cfg := Config{
		APIKey:    "my_key",
		Cache:     db,
		CacheTTL:  1 * time.Hour,
		RateLimit: 10,
	}
	c := New(cfg)
	if c.apiKey != "my_key" {
		t.Errorf("expected apiKey my_key, got %s", c.apiKey)
	}
	if c.cacheTTL != 1*time.Hour {
		t.Errorf("expected cacheTTL 1h, got %s", c.cacheTTL)
	}

	// Default config
	c2 := New(Config{APIKey: "key"})
	if c2.cacheTTL != defaultCacheTTL {
		t.Errorf("expected default cacheTTL, got %s", c2.cacheTTL)
	}
	if c2.cache != nil {
		t.Error("expected nil cache when not provided")
	}
}

func TestSearchTV_NotFound(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(searchResponse{Results: []SearchResult{}})
	})
	defer mock.Close()

	client := newTestClient(t, mock.URL(), nil)
	result, err := client.SearchTV(context.Background(), "NonExistentShow", 0, 0)
	if err != nil {
		t.Fatalf("SearchTV failed: %v", err)
	}
	if result != nil {
		t.Error("expected nil for non-existent show")
	}
}

func TestToLower(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "hello"},
		{"ABC", "abc"},
		{"123", "123"},
		{"", ""},
		{"MIXED123", "mixed123"},
	}
	for _, tt := range tests {
		got := toLower(tt.input)
		if got != tt.want {
			t.Errorf("toLower(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s, sub string
		want   bool
	}{
		{"hello world", "world", true},
		{"hello world", "xyz", false},
		{"test", "", true},
		{"", "", true},
		{"abc", "abcd", false},
	}
	for _, tt := range tests {
		got := contains(tt.s, tt.sub)
		if got != tt.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.sub, got, tt.want)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	mock := newMockTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(searchResponse{
			Results: []SearchResult{
				{ID: 1, Title: fmt.Sprintf("Movie %s", r.URL.Query().Get("query")), ReleaseDate: "2020-01-01"},
			},
		})
	})
	defer mock.Close()

	// Very slow rate limit: 1 request per second
	client := &Client{
		apiKey:  "test_key",
		baseURL: mock.URL(),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		cache:     nil,
		cacheTTL:  defaultCacheTTL,
		rateLimit: time.NewTicker(50 * time.Millisecond),
	}

	// Make 2 requests — should not panic
	_, err1 := client.SearchMovie(context.Background(), "Test1", 0)
	_, err2 := client.SearchMovie(context.Background(), "Test2", 0)

	if err1 != nil {
		t.Errorf("first request failed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("second request failed: %v", err2)
	}
}

func TestTMDBCache_TTL(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("Open DB failed: %v", err)
	}
	defer db.Close()

	// Insert an old cache entry by raw SQL (bypass SaveTMDBCache which sets datetime('now'))
	db.ExecContext(ctx, `INSERT INTO tmdb_cache (normalized_name, tmdb_id, media_type, data, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		"old movie", int64(999), "movie", `{}`, "2020-01-01 00:00:00")

	// Should be expired with 1 hour TTL
	entry, err := db.GetTMDBCache(ctx, "old movie", "movie", 1*time.Hour)
	if err != nil {
		t.Fatalf("GetTMDBCache failed: %v", err)
	}
	if entry != nil {
		t.Error("expected nil for expired cache entry")
	}

	// With 0 TTL (no expiry), should return
	entry, err = db.GetTMDBCache(ctx, "old movie", "movie", 0)
	if err != nil {
		t.Fatalf("GetTMDBCache failed: %v", err)
	}
	if entry == nil {
		t.Error("expected non-nil for 0 TTL")
	}
}

func TestClearExpiredCache(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(":memory:")
	if err != nil {
		t.Fatalf("Open DB failed: %v", err)
	}
	defer db.Close()

	db.SaveTMDBCache(ctx, "fresh", "movie", 1, `{}`)
	db.SaveTMDBCache(ctx, "stale", "movie", 2, `{}`)

	// Manually set "stale" to old date
	db.ExecContext(ctx, `UPDATE tmdb_cache SET updated_at = '2020-01-01 00:00:00' WHERE normalized_name = ?`, "stale")

	err = db.ClearExpiredTMDBCache(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("ClearExpiredTMDBCache failed: %v", err)
	}

	// "fresh" should still exist
	fresh, _ := db.GetTMDBCache(ctx, "fresh", "movie", 0)
	if fresh == nil {
		t.Error("expected 'fresh' to still exist after clear")
	}

	// "stale" should be gone
	stale, _ := db.GetTMDBCache(ctx, "stale", "movie", 0)
	if stale != nil {
		t.Error("expected 'stale' to be deleted")
	}
}
