package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is the top-level configuration for the OpenList media scanner.
type Config struct {
	OpenList OpenListConfig `json:"openlist"`
	Scanner  ScannerConfig  `json:"scanner"`
	Database DatabaseConfig `json:"database"`
	TMDB     TMDBConfig     `json:"tmdb"`
	Log      LogConfig      `json:"log"`
	Storage  StorageConfig  `json:"storage"`
}

// OpenListConfig holds connection settings for the OpenList API.
type OpenListConfig struct {
	URL      string `env:"OPENLIST_URL"       json:"url"`
	Username string `env:"OPENLIST_USERNAME"  json:"username"`
	Password string `env:"OPENLIST_PASSWORD"  json:"password"`
	Timeout  int    `env:"OPENLIST_TIMEOUT"   json:"timeout"`
	RetryMax int    `env:"OPENLIST_RETRY_MAX" json:"retry_max"`
}

// ScannerConfig holds scan worker pool settings.
type ScannerConfig struct {
	Workers   int `env:"SCANNER_WORKERS"    json:"workers"`
	QueueSize int `env:"SCANNER_QUEUE_SIZE" json:"queue_size"`
}

// DatabaseConfig holds SQLite database settings.
type DatabaseConfig struct {
	Path string `env:"DATABASE_PATH" json:"path"`
}

// TMDBConfig holds TMDB API settings.
type TMDBConfig struct {
	APIKey       string `env:"TMDB_API_KEY"        json:"api_key"`
	BaseURL      string `env:"TMDB_BASE_URL"       json:"base_url"`
	ImageBaseURL string `env:"TMDB_IMAGE_BASE_URL"  json:"image_base_url"`
	CacheTTL     int    `env:"TMDB_CACHE_TTL"      json:"cache_ttl"`
	RateLimit    int    `env:"TMDB_RATE_LIMIT"     json:"rate_limit"`
	MappingPath  string `env:"TMDB_MAPPING_PATH"   json:"mapping_path"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `env:"LOG_LEVEL"  json:"level"`
	Output string `env:"LOG_OUTPUT" json:"output"`
}

// StorageConfig holds scan paths for each storage type.
type StorageConfig struct {
	LocalPaths  []string `env:"STORAGE_LOCAL_PATHS"  json:"local_paths"`
	QuarkPaths  []string `env:"STORAGE_QUARK_PATHS"  json:"quark_paths"`
	TianyiPaths []string `env:"STORAGE_TIANYI_PATHS" json:"tianyi_paths"`
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		OpenList: OpenListConfig{
			URL:      "http://localhost:5244",
			Username: "",
			Password: "",
			Timeout:  30,
			RetryMax: 3,
		},
		Scanner: ScannerConfig{
			Workers:   32,
			QueueSize: 10000,
		},
		Database: DatabaseConfig{
			Path: "data/media.db",
		},
		TMDB: TMDBConfig{
			BaseURL:      "https://api.themoviedb.org/3",
			ImageBaseURL: "https://image.tmdb.org/t/p/w500",
			CacheTTL:     86400,
			RateLimit:    40,
		},
		Log: LogConfig{
			Level:  "info",
			Output: "stdout",
		},
		Storage: StorageConfig{
			LocalPaths:  []string{},
			QuarkPaths:  []string{},
			TianyiPaths: []string{},
		},
	}
}

// Load reads configuration from a .env file (path), then applies environment
// variable overrides.  OS env vars take highest precedence.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	// Read .env file if it exists (lower precedence than OS env vars)
	if path != "" {
		envMap, err := loadEnvFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load .env file: %w", err)
		}
		applyEnvMap(cfg, envMap)
	}

	// OS environment variables (highest precedence)
	applyOSEnv(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the config for missing or invalid values.
func (c *Config) Validate() error {
	if c.OpenList.URL == "" {
		return fmt.Errorf("openlist.url is required (set OPENLIST_URL in .env)")
	}
	if u, err := url.Parse(c.OpenList.URL); err != nil {
		return fmt.Errorf("openlist.url is not a valid URL: %w", err)
	} else if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("openlist.url must use http or https scheme, got %q", u.Scheme)
	}
	if c.OpenList.Timeout <= 0 {
		return fmt.Errorf("openlist.timeout must be > 0 seconds")
	}
	if c.OpenList.RetryMax < 0 {
		return fmt.Errorf("openlist.retry_max must be >= 0")
	}
	if c.Scanner.Workers <= 0 {
		return fmt.Errorf("scanner.workers must be > 0")
	}
	if c.Scanner.QueueSize <= 0 {
		return fmt.Errorf("scanner.queue_size must be > 0")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required (set DATABASE_PATH in .env)")
	}
	if c.TMDB.CacheTTL < 0 {
		return fmt.Errorf("tmdb.cache_ttl must be >= 0")
	}
	if c.TMDB.RateLimit < 0 {
		return fmt.Errorf("tmdb.rate_limit must be >= 0")
	}
	if c.Log.Level == "" {
		return fmt.Errorf("log.level is required (set LOG_LEVEL in .env)")
	}
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level must be one of debug/info/warn/error, got %q", c.Log.Level)
	}
	return nil
}

// loadEnvFile reads a .env file and returns a map of KEY=VALUE pairs.
// Lines starting with # are treated as comments.  Empty lines are skipped.
func loadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split on first '=' only — values may contain '='
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Remove surrounding quotes if present
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" {
			result[key] = val
		}
	}
	return result, sc.Err()
}

// applyEnvMap applies values from a .env file map to the config struct.
func applyEnvMap(cfg *Config, m map[string]string) {
	if m == nil {
		return
	}
	// Helper closures
	strVal := func(target *string, key string) {
		if v, ok := m[key]; ok {
			*target = v
		}
	}
	intVal := func(target *int, key string) {
		if v, ok := m[key]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				*target = n
			}
		}
	}

	strVal(&cfg.OpenList.URL, "OPENLIST_URL")
	strVal(&cfg.OpenList.Username, "OPENLIST_USERNAME")
	strVal(&cfg.OpenList.Password, "OPENLIST_PASSWORD")
	intVal(&cfg.OpenList.Timeout, "OPENLIST_TIMEOUT")
	intVal(&cfg.OpenList.RetryMax, "OPENLIST_RETRY_MAX")

	intVal(&cfg.Scanner.Workers, "SCANNER_WORKERS")
	intVal(&cfg.Scanner.QueueSize, "SCANNER_QUEUE_SIZE")

	strVal(&cfg.Database.Path, "DATABASE_PATH")

	strVal(&cfg.TMDB.APIKey, "TMDB_API_KEY")
	strVal(&cfg.TMDB.BaseURL, "TMDB_BASE_URL")
	strVal(&cfg.TMDB.ImageBaseURL, "TMDB_IMAGE_BASE_URL")
	intVal(&cfg.TMDB.CacheTTL, "TMDB_CACHE_TTL")
	intVal(&cfg.TMDB.RateLimit, "TMDB_RATE_LIMIT")
	strVal(&cfg.TMDB.MappingPath, "TMDB_MAPPING_PATH")

	strVal(&cfg.Log.Level, "LOG_LEVEL")
	strVal(&cfg.Log.Output, "LOG_OUTPUT")

	if v, ok := m["STORAGE_LOCAL_PATHS"]; ok {
		cfg.Storage.LocalPaths = splitEnv(v)
	}
	if v, ok := m["STORAGE_QUARK_PATHS"]; ok {
		cfg.Storage.QuarkPaths = splitEnv(v)
	}
	if v, ok := m["STORAGE_TIANYI_PATHS"]; ok {
		cfg.Storage.TianyiPaths = splitEnv(v)
	}
}

// applyOSEnv applies overrides from OS environment variables (highest priority).
func applyOSEnv(cfg *Config) {
	override(&cfg.OpenList.URL, os.Getenv("OPENLIST_URL"))
	override(&cfg.OpenList.Username, os.Getenv("OPENLIST_USERNAME"))
	override(&cfg.OpenList.Password, os.Getenv("OPENLIST_PASSWORD"))
	overrideStringToInt(&cfg.OpenList.Timeout, os.Getenv("OPENLIST_TIMEOUT"))
	overrideStringToInt(&cfg.OpenList.RetryMax, os.Getenv("OPENLIST_RETRY_MAX"))

	overrideStringToInt(&cfg.Scanner.Workers, os.Getenv("SCANNER_WORKERS"))
	overrideStringToInt(&cfg.Scanner.QueueSize, os.Getenv("SCANNER_QUEUE_SIZE"))

	override(&cfg.Database.Path, os.Getenv("DATABASE_PATH"))

	override(&cfg.TMDB.APIKey, os.Getenv("TMDB_API_KEY"))
	override(&cfg.TMDB.BaseURL, os.Getenv("TMDB_BASE_URL"))
	override(&cfg.TMDB.ImageBaseURL, os.Getenv("TMDB_IMAGE_BASE_URL"))
	overrideStringToInt(&cfg.TMDB.CacheTTL, os.Getenv("TMDB_CACHE_TTL"))
	overrideStringToInt(&cfg.TMDB.RateLimit, os.Getenv("TMDB_RATE_LIMIT"))
	override(&cfg.TMDB.MappingPath, os.Getenv("TMDB_MAPPING_PATH"))

	override(&cfg.Log.Level, os.Getenv("LOG_LEVEL"))
	override(&cfg.Log.Output, os.Getenv("LOG_OUTPUT"))

	if env := os.Getenv("STORAGE_LOCAL_PATHS"); env != "" {
		cfg.Storage.LocalPaths = splitEnv(env)
	}
	if env := os.Getenv("STORAGE_QUARK_PATHS"); env != "" {
		cfg.Storage.QuarkPaths = splitEnv(env)
	}
	if env := os.Getenv("STORAGE_TIANYI_PATHS"); env != "" {
		cfg.Storage.TianyiPaths = splitEnv(env)
	}
}

func override(target *string, value string) {
	if value != "" {
		*target = value
	}
}

func overrideStringToInt(target *int, value string) {
	if value == "" {
		return
	}
	if n, err := strconv.Atoi(value); err == nil {
		*target = n
	}
}

// splitEnv splits a comma-separated string value.
func splitEnv(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
