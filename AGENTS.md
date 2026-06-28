# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build everything
go build ./...

# Build the binary
make build  # outputs to build/openlist

# Run all tests with coverage
make test   # go test ./... -cover -count=1 -timeout 120s

# Run a single package's tests
go test ./internal/duplicate/ -v -count=1

# Run a single test
go test ./internal/media/ -run TestNormalize_ChineseMovie -v

# Vet
go vet ./...

# Run the app
./build/openlist --scan
./build/openlist --report
./build/openlist --cleanup --plan-path cleanup_plan.json
```

## Architecture Overview

The app is a pipeline of independent modules in `internal/`, connected by shared types:

```
Config → Logger
    ↓
OpenList SDK ←→ Scanner (BFS Worker Pool)
    ↓
Repository (SQLite BatchInserter)
    ↓
Media (Normalize) → Duplicate (Detect) → TMDB (Match)
    ↓
Report (HTML) → Cleanup (Plan + Execute)
```

### Module Integration Points

| Module | Consumes | Produces |
|---|---|---|
| `config` | YAML file + env vars | `Config` struct |
| `logger` | level string | `*Logger` (JSON structured) |
| `openlist` | base URL + password | `*Client` (List/Get/Delete) |
| `scanner` | `*openlist.Client`, seeds | `[]ScanResult` via channel |
| `repository` | DB path | `*DB`, `*BatchInserter` |
| `media` | filename string | `MediaInfo{Title, EpisodeTag}` |
| `duplicate` | `[]FileEntry` | `[]DuplicateGroup`, `Stats` |
| `tmdb` | name + year | `*MovieResult`, `*TVResult` (cached) |
| `report` | `ReportData` | `report.html` file |
| `cleanup` | `[]DuplicateGroup`, `*openlist.Client` | `Plan` + execution |

### Dependency Chain

No circular dependencies. Direction: `config → logger`, `scanner → openlist`, `repository` (standalone), `duplicate → media`, `report → duplicate`, `cleanup → duplicate + openlist`, `tmdb → repository`, `cmd` imports everything.

### Data Flow (Full Pipeline)

```
seeds → Scanner.Start()
  → taskCh → Worker (openlist.List)
    → resultCh → BatchInserter.Insert()
      → SQLite media_files table
        → duplicate.Detector.Detect()
          → report.Generate() or cleanup.CreatePlan()
```

### Key Patterns

- **All public APIs accept `context.Context`** as first parameter
- **No global state** — everything is struct-based, injected via constructors
- **Zero external HTTP deps** — uses only `net/http` standard library
- **Batch writes** only — never insert one row at a time
- **Mock servers** for tests — `httptest.NewServer` with file trees for scanner/openlist, response fixtures for TMDB
- **Channel-based concurrency** — buffered channels for task/result queues, `sync.WaitGroup` for pending tracking

### Configuration Precedence

Defaults → YAML file → Environment variable overrides. Env vars use uppercase prefix convention: `OPENLIST_URL`, `SCANNER_WORKERS`, `DATABASE_PATH`, `LOG_LEVEL`, `TMDB_API_KEY`, etc.

## Testing Conventions

- Use `testing.TB` temp dirs for filesystem tests: `t.TempDir()`
- SQLite tests use `:memory:` or `t.TempDir()` for isolation
- Mock servers use `httptest.NewServer` in `t.Cleanup`
- 100k-file benchmark test is gated with `testing.Short()`
- All tests expected to pass; the repo has ~140+ tests across 10 packages

## Dependencies (Minimal)

- `gopkg.in/yaml.v3` — YAML config parsing
- `modernc.org/sqlite` — Pure Go SQLite (no CGO)
