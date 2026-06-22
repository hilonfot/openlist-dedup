package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"openlist/internal/config"
	"openlist/internal/repository"
)

var (
	defaultRateLimit = 40 // requests per 10 seconds
	defaultCacheTTL  = 24 * time.Hour
	requestTimeout   = 10 * time.Second
)

// Client is a TMDB API client with SQLite caching and rate limiting.
type Client struct {
	apiKey       string
	baseURL      string
	imageBaseURL string
	httpClient   *http.Client
	cache        *repository.DB
	cacheTTL     time.Duration
	rateLimit    *time.Ticker
	mapping      config.MappingTable // optional local fallback mapping
}

// Config holds TMDB client configuration.
type Config struct {
	APIKey       string
	BaseURL      string // API base URL (e.g. https://api.themoviedb.org/3)
	ImageBaseURL string // image base URL (e.g. https://image.tmdb.org/t/p/w500)
	Cache        *repository.DB
	CacheTTL     time.Duration
	RateLimit    int // requests per 10 seconds
	Mapping      config.MappingTable // optional local fallback mapping
}

// SetMapping sets the local mapping table for fallback lookups.
func (c *Client) SetMapping(m config.MappingTable) {
	c.mapping = m
}

// New creates a new TMDB client.
func New(cfg Config) *Client {
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	rateLimit := cfg.RateLimit
	if rateLimit <= 0 {
		rateLimit = defaultRateLimit
	}

	// Strip trailing slash from base URLs to avoid double-slash in paths
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	imageBaseURL := strings.TrimRight(cfg.ImageBaseURL, "/")

	interval := (10 * time.Second) / time.Duration(rateLimit)

	return &Client{
		apiKey:      cfg.APIKey,
		baseURL:     baseURL,
		imageBaseURL: imageBaseURL,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		cache:     cfg.Cache,
		cacheTTL:  cfg.CacheTTL,
		rateLimit: time.NewTicker(interval),
	}
}

// SearchResult is a single result from TMDB search.
type SearchResult struct {
	ID           int64   `json:"id"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	Overview     string  `json:"overview"`
	VoteAverage  float64 `json:"vote_average"`
	PosterPath   string  `json:"poster_path"`
}

// searchResponse is the top-level TMDB search API response.
type searchResponse struct {
	Results []SearchResult `json:"results"`
}

// movieDetails is the TMDB movie detail API response (only fields we need).
type movieDetails struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Overview   string `json:"overview"`
	PosterPath string `json:"poster_path"`
}

// tvDetails is the TMDB TV detail API response (only fields we need).
type tvDetails struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Overview   string `json:"overview"`
	PosterPath string `json:"poster_path"`
}

// MovieResult holds the matched movie information.
type MovieResult struct {
	TMDBID      int64
	Title       string
	ReleaseYear int
	Overview    string
	VoteAverage float64
	PosterPath  string
	PosterURL   string
	TMDBURL     string
	FromCache   bool
}

// TVResult holds the matched TV series information.
type TVResult struct {
	TMDBID       int64
	Name         string
	FirstAirYear int
	Overview     string
	VoteAverage  float64
	PosterPath   string
	PosterURL    string
	TMDBURL      string
	FromCache    bool
}

// SearchMovie searches for a movie by name, optionally filtering by year.
// It tries Chinese first, then falls back to English.
func (c *Client) SearchMovie(ctx context.Context, name string, year int) (*MovieResult, error) {
	// Check cache first
	if c.cache != nil {
		result, err := c.searchMovieCached(ctx, name, "movie", year)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
	}

	// Search in Chinese first, then English
	result, err := c.searchMovieLang(ctx, name, year, "zh-CN")
	if err != nil {
		return nil, err
	}
	if result != nil {
		c.saveToCache(ctx, cacheKey(name, year), "movie", result.TMDBID, result)
		return result, nil
	}

	result, err = c.searchMovieLang(ctx, name, year, "en-US")
	if err != nil {
		return nil, err
	}

	if result != nil {
		// Try to get Chinese title/overview/poster for better display
		if details, err := c.fetchMovieDetails(ctx, result.TMDBID, "zh-CN"); err == nil {
			if details.Title != "" {
				result.Title = details.Title
			}
			if details.Overview != "" {
				result.Overview = details.Overview
			}
			if details.PosterPath != "" {
				result.PosterPath = details.PosterPath
				result.PosterURL = c.posterURL(details.PosterPath)
			}
		}
		c.saveToCache(ctx, cacheKey(name, year), "movie", result.TMDBID, result)
	}

	// Fallback: check local mapping
	if result == nil {
		result = c.lookupMovieMapping(name, year)
	}

	return result, nil
}

// SearchTV searches for a TV series by name, optionally filtering by season/year.
func (c *Client) SearchTV(ctx context.Context, name string, season int, year int) (*TVResult, error) {
	// Check cache first
	if c.cache != nil {
		result, err := c.searchTVCached(ctx, name, "tv", season, year)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
	}

	// Search in Chinese first, then English
	result, err := c.searchTVLang(ctx, name, season, year, "zh-CN")
	if err != nil {
		return nil, err
	}
	if result != nil {
		c.saveToCache(ctx, cacheKey(name, year), "tv", result.TMDBID, result)
		return result, nil
	}

	result, err = c.searchTVLang(ctx, name, season, year, "en-US")
	if err != nil {
		return nil, err
	}

	if result != nil {
		// Try to get Chinese name/overview/poster for better display
		if details, err := c.fetchTVDetails(ctx, result.TMDBID, "zh-CN"); err == nil {
			if details.Name != "" {
				result.Name = details.Name
			}
			if details.Overview != "" {
				result.Overview = details.Overview
			}
			if details.PosterPath != "" {
				result.PosterPath = details.PosterPath
				result.PosterURL = c.posterURL(details.PosterPath)
			}
		}
		c.saveToCache(ctx, cacheKey(name, year), "tv", result.TMDBID, result)
	}

	// Fallback: check local mapping
	if result == nil {
		result = c.lookupTVMapping(name, season, year)
	}

	return result, nil
}

// searchMovieLang searches for a movie in a specific language.
func (c *Client) searchMovieLang(ctx context.Context, name string, year int, lang string) (*MovieResult, error) {
	results, err := c.doSearch(ctx, "movie", name, lang)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	// Find best match
	return c.bestMovieMatch(results, name, year), nil
}

// searchTVLang searches for a TV series in a specific language.
func (c *Client) searchTVLang(ctx context.Context, name string, season int, year int, lang string) (*TVResult, error) {
	results, err := c.doSearch(ctx, "tv", name, lang)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return c.bestTVMatch(results, name, season, year), nil
}

// doSearch performs the actual TMDB API search request.
func (c *Client) doSearch(ctx context.Context, mediaType, query, language string) ([]SearchResult, error) {
	// Rate limit
	select {
	case <-c.rateLimit.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	u, _ := url.Parse(c.baseURL + "/search/" + mediaType)
	q := u.Query()
	q.Set("query", query)
	q.Set("language", language)
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tmdb api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return sr.Results, nil
}

// fetchMovieDetails fetches movie details by ID in the specified language.
func (c *Client) fetchMovieDetails(ctx context.Context, id int64, language string) (*movieDetails, error) {
	// Rate limit
	select {
	case <-c.rateLimit.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	u, _ := url.Parse(fmt.Sprintf("%s/movie/%d", c.baseURL, id))
	q := u.Query()
	q.Set("language", language)
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create movie details request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("movie details http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tmdb movie details error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var details movieDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("decode movie details: %w", err)
	}

	return &details, nil
}

// fetchTVDetails fetches TV series details by ID in the specified language.
func (c *Client) fetchTVDetails(ctx context.Context, id int64, language string) (*tvDetails, error) {
	// Rate limit
	select {
	case <-c.rateLimit.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	u, _ := url.Parse(fmt.Sprintf("%s/tv/%d", c.baseURL, id))
	q := u.Query()
	q.Set("language", language)
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create tv details request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tv details http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tmdb tv details error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var details tvDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("decode tv details: %w", err)
	}

	return &details, nil
}

// bestMovieMatch selects the best matching movie from search results.
func (c *Client) bestMovieMatch(results []SearchResult, query string, year int) *MovieResult {
	queryLower := toLower(query)

	// Pass 1: Exact title match — highest confidence, return immediately
	for _, r := range results {
		if toLower(r.Title) == queryLower {
			return c.movieFromResult(r)
		}
	}

	// Pass 2: Exact match plus year filter (when year is known)
	if year > 0 {
		for _, r := range results {
			if extractYear(r.ReleaseDate) == year {
				titleLower := toLower(r.Title)
				if titleLower == queryLower || similar(titleLower, queryLower) {
					return c.movieFromResult(r)
				}
			}
		}
	}

	// Pass 3: Year match only (any title with matching year)
	if year > 0 {
		for _, r := range results {
			if extractYear(r.ReleaseDate) == year {
				return c.movieFromResult(r)
			}
		}
	}

	// Pass 4: Contains match with highest score
	var best *SearchResult
	for i := range results {
		titleLower := toLower(results[i].Title)
		if contains(titleLower, queryLower) || contains(queryLower, titleLower) {
			if best == nil || results[i].VoteAverage > best.VoteAverage {
				best = &results[i]
			}
		}
	}
	if best != nil {
		return c.movieFromResult(*best)
	}

	// Pass 5: Desperate — return highest scored result
	if len(results) > 0 {
		best := results[0]
		for _, r := range results[1:] {
			if r.VoteAverage > best.VoteAverage {
				best = r
			}
		}
		return c.movieFromResult(best)
	}

	return nil
}

// bestTVMatch selects the best matching TV series from search results.
func (c *Client) bestTVMatch(results []SearchResult, query string, season int, year int) *TVResult {
	queryLower := toLower(query)

	// Pass 1: Exact name match — highest confidence
	for _, r := range results {
		if toLower(r.Name) == queryLower {
			return c.tvFromResult(r)
		}
	}

	// Pass 2: Exact/similar match with year filter
	if year > 0 {
		for _, r := range results {
			if extractYear(r.FirstAirDate) == year {
				nameLower := toLower(r.Name)
				if nameLower == queryLower || similar(nameLower, queryLower) {
					return c.tvFromResult(r)
				}
			}
		}
	}

	// Pass 3: Year match only
	if year > 0 {
		for _, r := range results {
			if extractYear(r.FirstAirDate) == year {
				return c.tvFromResult(r)
			}
		}
	}

	// Pass 4: Contains match with highest score
	var best *SearchResult
	for i := range results {
		nameLower := toLower(results[i].Name)
		if contains(nameLower, queryLower) || contains(queryLower, nameLower) {
			if best == nil || results[i].VoteAverage > best.VoteAverage {
				best = &results[i]
			}
		}
	}
	if best != nil {
		return c.tvFromResult(*best)
	}

	// Pass 5: Highest scored result
	if len(results) > 0 {
		best := results[0]
		for _, r := range results[1:] {
			if r.VoteAverage > best.VoteAverage {
				best = r
			}
		}
		return c.tvFromResult(best)
	}

	return nil
}

// --- Cache helpers ---

func (c *Client) searchMovieCached(ctx context.Context, name, mediaType string, year int) (*MovieResult, error) {
	cacheKey := cacheKey(name, year)
	entry, err := c.cache.GetTMDBCache(ctx, cacheKey, mediaType, c.cacheTTL)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result MovieResult
	if err := json.Unmarshal([]byte(entry.Data), &result); err != nil {
		return nil, nil // treat corrupt cache as miss
	}
	result.FromCache = true
	return &result, nil
}

func (c *Client) searchTVCached(ctx context.Context, name, mediaType string, season int, year int) (*TVResult, error) {
	cacheKey := cacheKey(name, year)
	entry, err := c.cache.GetTMDBCache(ctx, cacheKey, mediaType, c.cacheTTL)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result TVResult
	if err := json.Unmarshal([]byte(entry.Data), &result); err != nil {
		return nil, nil
	}
	result.FromCache = true
	return &result, nil
}

func (c *Client) saveToCache(ctx context.Context, name, mediaType string, tmdbID int64, data interface{}) {
	if c.cache == nil {
		fmt.Fprintf(os.Stderr, "TMDB: cache is nil, cannot save %s\n", name)
		return
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TMDB: marshal error for %s: %v\n", name, err)
		return
	}
	if err := c.cache.SaveTMDBCache(ctx, name, mediaType, tmdbID, string(jsonData)); err != nil {
		fmt.Fprintf(os.Stderr, "TMDB: save error for %s: %v\n", name, err)
	} else {
		fmt.Fprintf(os.Stderr, "TMDB: saved %s (%s, id=%d, %d bytes)\n", name, mediaType, tmdbID, len(jsonData))
	}
}

// --- Utility ---

// cacheKey creates a deterministic cache key from name and year.
func cacheKey(name string, year int) string {
	if year > 0 {
		return fmt.Sprintf("%s (%d)", name, year)
	}
	return name
}

// posterURL constructs a full TMDB image URL from a poster path.
func (c *Client) posterURL(path string) string {
	if path == "" {
		return ""
	}
	return c.imageBaseURL + path
}

// tmdbURL constructs a link to the TMDB page for a movie or TV show.
func (c *Client) tmdbURL(id int64, mediaType string) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("https://www.themoviedb.org/%s/%d", mediaType, id)
}

// movieFromResult converts a SearchResult to MovieResult.
func (c *Client) movieFromResult(r SearchResult) *MovieResult {
	return &MovieResult{
		TMDBID:      r.ID,
		Title:       r.Title,
		ReleaseYear: extractYear(r.ReleaseDate),
		Overview:    r.Overview,
		VoteAverage: r.VoteAverage,
		PosterPath:  r.PosterPath,
		PosterURL:   c.posterURL(r.PosterPath),
		TMDBURL:     c.tmdbURL(r.ID, "movie"),
	}
}

// tvFromResult converts a SearchResult to TVResult.
func (c *Client) tvFromResult(r SearchResult) *TVResult {
	return &TVResult{
		TMDBID:       r.ID,
		Name:         r.Name,
		FirstAirYear: extractYear(r.FirstAirDate),
		Overview:     r.Overview,
		VoteAverage:  r.VoteAverage,
		PosterPath:   r.PosterPath,
		PosterURL:    c.posterURL(r.PosterPath),
		TMDBURL:      c.tmdbURL(r.ID, "tv"),
	}
}

// extractYear parses the year from a TMDB date string (YYYY-MM-DD).
func extractYear(dateStr string) int {
	if len(dateStr) < 4 {
		return 0
	}
	var year int
	for i := 0; i < 4; i++ {
		if dateStr[i] < '0' || dateStr[i] > '9' {
			return 0
		}
		year = year*10 + int(dateStr[i]-'0')
	}
	return year
}

// toLower converts a string to lowercase without importing strings.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

// contains checks if s contains substr (both lowercase).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr) >= 0
}

// searchString finds substr in s and returns index or -1.
func searchString(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if len(substr) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// similar checks if two strings are approximately equal (one contains the other).
func similar(a, b string) bool {
	return contains(a, b) || contains(b, a)
}

// lookupMovieMapping checks the local mapping table for a movie match.
func (c *Client) lookupMovieMapping(name string, year int) *MovieResult {
	if c.mapping == nil {
		return nil
	}
	entry, ok := c.mapping.Lookup(name)
	if !ok || entry.Type != "movie" {
		return nil
	}
	return &MovieResult{
		TMDBID:      int64(entry.TMDBID),
		Title:       name,
		ReleaseYear: year,
		TMDBURL:     fmt.Sprintf("https://www.themoviedb.org/movie/%d", entry.TMDBID),
		FromCache:   false,
	}
}

// lookupTVMapping checks the local mapping table for a TV match.
func (c *Client) lookupTVMapping(name string, season int, year int) *TVResult {
	if c.mapping == nil {
		return nil
	}
	entry, ok := c.mapping.Lookup(name)
	if !ok || entry.Type != "tv" {
		return nil
	}
	return &TVResult{
		TMDBID:       int64(entry.TMDBID),
		Name:         name,
		FirstAirYear: year,
		TMDBURL:      fmt.Sprintf("https://www.themoviedb.org/tv/%d/season/%d", entry.TMDBID, season),
		FromCache:    false,
	}
}
