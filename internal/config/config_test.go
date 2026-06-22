package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.OpenList.URL != "http://localhost:5244" {
		t.Errorf("expected default URL http://localhost:5244, got %s", cfg.OpenList.URL)
	}
	if cfg.Scanner.Workers != 32 {
		t.Errorf("expected default workers 32, got %d", cfg.Scanner.Workers)
	}
	if cfg.Scanner.QueueSize != 10000 {
		t.Errorf("expected default queue_size 10000, got %d", cfg.Scanner.QueueSize)
	}
	if cfg.Database.Path != "data/media.db" {
		t.Errorf("expected default db path data/media.db, got %s", cfg.Database.Path)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Log.Level)
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := []byte(`
OPENLIST_URL=http://example:8080
OPENLIST_PASSWORD=secret123
SCANNER_WORKERS=16
SCANNER_QUEUE_SIZE=5000
DATABASE_PATH=/tmp/test.db
TMDB_API_KEY=tmdb_key_123
TMDB_CACHE_TTL=3600
LOG_LEVEL=debug
STORAGE_LOCAL_PATHS=/mnt/media
STORAGE_QUARK_PATHS=/quark/movies
STORAGE_TIANYI_PATHS=/tianyi/tv
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.OpenList.URL != "http://example:8080" {
		t.Errorf("expected URL http://example:8080, got %s", cfg.OpenList.URL)
	}
	if cfg.OpenList.Password != "secret123" {
		t.Errorf("expected password secret123, got %s", cfg.OpenList.Password)
	}
	if cfg.Scanner.Workers != 16 {
		t.Errorf("expected workers 16, got %d", cfg.Scanner.Workers)
	}
	if cfg.Scanner.QueueSize != 5000 {
		t.Errorf("expected queue_size 5000, got %d", cfg.Scanner.QueueSize)
	}
	if cfg.Database.Path != "/tmp/test.db" {
		t.Errorf("expected db path /tmp/test.db, got %s", cfg.Database.Path)
	}
	if cfg.TMDB.APIKey != "tmdb_key_123" {
		t.Errorf("expected tmdb key tmdb_key_123, got %s", cfg.TMDB.APIKey)
	}
	if cfg.TMDB.CacheTTL != 3600 {
		t.Errorf("expected cache_ttl 3600, got %d", cfg.TMDB.CacheTTL)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Log.Level)
	}
	if len(cfg.Storage.LocalPaths) != 1 || cfg.Storage.LocalPaths[0] != "/mnt/media" {
		t.Errorf("expected local_paths [/mnt/media], got %v", cfg.Storage.LocalPaths)
	}
	if len(cfg.Storage.QuarkPaths) != 1 || cfg.Storage.QuarkPaths[0] != "/quark/movies" {
		t.Errorf("expected quark_paths [/quark/movies], got %v", cfg.Storage.QuarkPaths)
	}
	if len(cfg.Storage.TianyiPaths) != 1 || cfg.Storage.TianyiPaths[0] != "/tianyi/tv" {
		t.Errorf("expected tianyi_paths [/tianyi/tv], got %v", cfg.Storage.TianyiPaths)
	}
}

func TestLoad_NonExistentEnv(t *testing.T) {
	// Load with empty path → should get defaults
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected nil error for empty path, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config from defaults")
	}
	if cfg.OpenList.URL != "http://localhost:5244" {
		t.Errorf("expected default URL, got %s", cfg.OpenList.URL)
	}

	// Load with non-existent file → should get defaults (no error)
	cfg, err = Load("/nonexistent/path/.env")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config from defaults")
	}
}

func TestEnvOverride(t *testing.T) {
	// Set env vars
	os.Setenv("OPENLIST_URL", "http://env-override:9090")
	os.Setenv("SCANNER_WORKERS", "64")
	os.Setenv("DATABASE_PATH", "/env/test.db")
	os.Setenv("LOG_LEVEL", "warn")
	os.Setenv("STORAGE_LOCAL_PATHS", "/env/path1,/env/path2")
	defer func() {
		os.Unsetenv("OPENLIST_URL")
		os.Unsetenv("SCANNER_WORKERS")
		os.Unsetenv("DATABASE_PATH")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("STORAGE_LOCAL_PATHS")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.OpenList.URL != "http://env-override:9090" {
		t.Errorf("expected URL http://env-override:9090, got %s", cfg.OpenList.URL)
	}
	if cfg.Scanner.Workers != 64 {
		t.Errorf("expected workers 64, got %d", cfg.Scanner.Workers)
	}
	if cfg.Database.Path != "/env/test.db" {
		t.Errorf("expected db path /env/test.db, got %s", cfg.Database.Path)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("expected log level warn, got %s", cfg.Log.Level)
	}
	if len(cfg.Storage.LocalPaths) != 2 || cfg.Storage.LocalPaths[0] != "/env/path1" {
		t.Errorf("expected local_paths [/env/path1 /env/path2], got %v", cfg.Storage.LocalPaths)
	}
}

func TestEnvOverride_InvalidInt(t *testing.T) {
	os.Setenv("SCANNER_WORKERS", "not-a-number")
	defer os.Unsetenv("SCANNER_WORKERS")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Should keep default when env var is not a valid int
	if cfg.Scanner.Workers != 32 {
		t.Errorf("expected default workers 32, got %d", cfg.Scanner.Workers)
	}
}

func TestValidate_EmptyURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OpenList.URL = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty openlist.url")
	}
}

func TestValidate_InvalidWorkers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Scanner.Workers = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for workers <= 0")
	}
}

func TestValidate_EmptyDBPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.Path = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty database.path")
	}
}

func TestLoad_FileWithEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := []byte(`
OPENLIST_URL=http://file-value:8080
SCANNER_WORKERS=8
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	os.Setenv("SCANNER_WORKERS", "99")
	defer os.Unsetenv("SCANNER_WORKERS")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// File value should be present
	if cfg.OpenList.URL != "http://file-value:8080" {
		t.Errorf("expected URL from file http://file-value:8080, got %s", cfg.OpenList.URL)
	}
	// Env override should win over file value
	if cfg.Scanner.Workers != 99 {
		t.Errorf("expected workers from env 99, got %d", cfg.Scanner.Workers)
	}
}

func TestLoadEnvFile_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := []byte(`
OPENLIST_URL="http://quoted:8080"
OPENLIST_PASSWORD='secret'
SCANNER_WORKERS=32
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.OpenList.URL != "http://quoted:8080" {
		t.Errorf("expected URL http://quoted:8080, got %s", cfg.OpenList.URL)
	}
	if cfg.OpenList.Password != "secret" {
		t.Errorf("expected password secret, got %s", cfg.OpenList.Password)
	}
}

func TestSplitEnv(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"/a,/b,/c", []string{"/a", "/b", "/c"}},
		{"/a, /b, /c", []string{"/a", "/b", "/c"}},
		{"", []string{}},
		{"/single", []string{"/single"}},
		{",,/a,,", []string{"/a"}},
	}
	for _, tt := range tests {
		got := splitEnv(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitEnv(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitEnv(%q) = %v, want %v", tt.input, got, tt.want)
				break
			}
		}
	}
}
