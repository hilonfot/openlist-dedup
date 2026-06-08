package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the OpenList media scanner.
type Config struct {
	OpenList OpenListConfig `yaml:"openlist" json:"openlist"`
	Scanner  ScannerConfig  `yaml:"scanner"  json:"scanner"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	TMDB     TMDBConfig     `yaml:"tmdb"     json:"tmdb"`
	Log      LogConfig      `yaml:"log"      json:"log"`
	Storage  StorageConfig  `yaml:"storage"  json:"storage"`
}

// OpenListConfig holds connection settings for the OpenList API.
type OpenListConfig struct {
	URL      string `yaml:"url"       env:"OPENLIST_URL"       json:"url"`
	Password string `yaml:"password"  env:"OPENLIST_PASSWORD"  json:"password"`
	Timeout  int    `yaml:"timeout"   env:"OPENLIST_TIMEOUT"   json:"timeout"`
	RetryMax int    `yaml:"retry_max" env:"OPENLIST_RETRY_MAX" json:"retry_max"`
}

// ScannerConfig holds scan worker pool settings.
type ScannerConfig struct {
	Workers   int `yaml:"workers"    env:"SCANNER_WORKERS"    json:"workers"`
	QueueSize int `yaml:"queue_size" env:"SCANNER_QUEUE_SIZE" json:"queue_size"`
}

// DatabaseConfig holds SQLite database settings.
type DatabaseConfig struct {
	Path string `yaml:"path" env:"DATABASE_PATH" json:"path"`
}

// TMDBConfig holds TMDB API settings.
type TMDBConfig struct {
	APIKey    string `yaml:"api_key"     env:"TMDB_API_KEY"     json:"api_key"`
	CacheTTL  int    `yaml:"cache_ttl"   env:"TMDB_CACHE_TTL"   json:"cache_ttl"`
	RateLimit int    `yaml:"rate_limit"  env:"TMDB_RATE_LIMIT"  json:"rate_limit"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `yaml:"level"  env:"LOG_LEVEL"  json:"level"`
	Output string `yaml:"output" env:"LOG_OUTPUT" json:"output"`
}

// StorageConfig holds scan paths for each storage type.
type StorageConfig struct {
	LocalPaths  []string `yaml:"local_paths"  env:"STORAGE_LOCAL_PATHS"  json:"local_paths"`
	QuarkPaths  []string `yaml:"quark_paths"  env:"STORAGE_QUARK_PATHS"  json:"quark_paths"`
	TianyiPaths []string `yaml:"tianyi_paths" env:"STORAGE_TIANYI_PATHS" json:"tianyi_paths"`
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		OpenList: OpenListConfig{
			URL:      "http://localhost:5244",
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
			APIKey:    "",
			CacheTTL:  86400,
			RateLimit: 40,
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

// LoadFromFile reads a YAML config file and merges it on top of defaults.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	return cfg, nil
}

// Load loads config from the given path (if non-empty) then applies
// environment variable overrides.
func Load(path string) (*Config, error) {
	cfg, err := LoadFromFile(path)
	if err != nil {
		return nil, err
	}

	cfg.applyEnvOverrides()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the config for missing or invalid values.
func (c *Config) Validate() error {
	if c.OpenList.URL == "" {
		return fmt.Errorf("openlist.url is required")
	}
	if c.Scanner.Workers <= 0 {
		return fmt.Errorf("scanner.workers must be > 0")
	}
	if c.Scanner.QueueSize <= 0 {
		return fmt.Errorf("scanner.queue_size must be > 0")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if c.Log.Level == "" {
		return fmt.Errorf("log.level is required")
	}
	return nil
}

// applyEnvOverrides walks the config and replaces fields tagged with `env`
// when the corresponding environment variable is set.
func (c *Config) applyEnvOverrides() {
	override(&c.OpenList.URL, os.Getenv("OPENLIST_URL"))
	override(&c.OpenList.Password, os.Getenv("OPENLIST_PASSWORD"))
	overrideStringToInt(&c.OpenList.Timeout, os.Getenv("OPENLIST_TIMEOUT"))
	overrideStringToInt(&c.OpenList.RetryMax, os.Getenv("OPENLIST_RETRY_MAX"))

	overrideStringToInt(&c.Scanner.Workers, os.Getenv("SCANNER_WORKERS"))
	overrideStringToInt(&c.Scanner.QueueSize, os.Getenv("SCANNER_QUEUE_SIZE"))

	override(&c.Database.Path, os.Getenv("DATABASE_PATH"))

	override(&c.TMDB.APIKey, os.Getenv("TMDB_API_KEY"))
	overrideStringToInt(&c.TMDB.CacheTTL, os.Getenv("TMDB_CACHE_TTL"))
	overrideStringToInt(&c.TMDB.RateLimit, os.Getenv("TMDB_RATE_LIMIT"))

	override(&c.Log.Level, os.Getenv("LOG_LEVEL"))
	override(&c.Log.Output, os.Getenv("LOG_OUTPUT"))

	if env := os.Getenv("STORAGE_LOCAL_PATHS"); env != "" {
		c.Storage.LocalPaths = splitEnv(env)
	}
	if env := os.Getenv("STORAGE_QUARK_PATHS"); env != "" {
		c.Storage.QuarkPaths = splitEnv(env)
	}
	if env := os.Getenv("STORAGE_TIANYI_PATHS"); env != "" {
		c.Storage.TianyiPaths = splitEnv(env)
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

// splitEnv splits a comma-separated environment variable value.
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
