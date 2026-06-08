package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"openlist/internal/cleanup"
	"openlist/internal/config"
	"openlist/internal/duplicate"
	"openlist/internal/logger"
	"openlist/internal/openlist"
	"openlist/internal/report"
	"openlist/internal/repository"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Parse command-line flags manually (no external deps)
	cfgPath := flag("--config", "configs/config.yaml")
	modeScan := hasFlag("--scan")
	modeReport := hasFlag("--report")
	modeCleanup := hasFlag("--cleanup")
	applyCleanup := hasFlag("--apply")
	dbOverride := flag("--db", "")
	workers := flagInt("--workers", 32)

	// Load config
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}
	if dbOverride != "" {
		cfg.Database.Path = dbOverride
	}
	if workers > 0 {
		cfg.Scanner.Workers = workers
	}

	// Initialize logger
	log := logger.New(cfg.Log.Level, os.Stdout)
	log = log.With("app", "openlist", "version", "0.1.0")

	// If no mode specified, default to scan
	if !modeScan && !modeReport && !modeCleanup {
		modeScan = true
	}

	// Ensure data directory exists
	dbDir := filepath.Dir(cfg.Database.Path)
	if dbDir != "" && dbDir != "." {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			log.Error("Failed to create data directory", "path", dbDir, "error", err)
			return 1
		}
	}

	// Open database
	db, err := repository.Open(cfg.Database.Path)
	if err != nil {
		log.Error("Failed to open database", "path", cfg.Database.Path, "error", err)
		return 1
	}
	defer db.Close()

	ctx := context.Background()

	// OpenList client
	olClient := openlist.New(
		cfg.OpenList.URL,
		cfg.OpenList.Password,
		time.Duration(cfg.OpenList.Timeout)*time.Second,
		cfg.OpenList.RetryMax,
	)

	if modeScan {
		log.Info("Starting scan",
			"openlist_url", cfg.OpenList.URL,
			"workers", cfg.Scanner.Workers,
			"queue_size", cfg.Scanner.QueueSize,
		)

		// Build seed tasks from configured paths
		var seeds []struct {
			storage string
			paths   []string
		}
		if len(cfg.Storage.LocalPaths) > 0 {
			seeds = append(seeds, struct {
				storage string
				paths   []string
			}{"local", cfg.Storage.LocalPaths})
		}
		if len(cfg.Storage.QuarkPaths) > 0 {
			seeds = append(seeds, struct {
				storage string
				paths   []string
			}{"quark", cfg.Storage.QuarkPaths})
		}
		if len(cfg.Storage.TianyiPaths) > 0 {
			seeds = append(seeds, struct {
				storage string
				paths   []string
			}{"tianyi", cfg.Storage.TianyiPaths})
		}

		if len(seeds) == 0 {
			log.Warn("No scan paths configured, skipping scan")
		} else {
			// TODO: Phase 10 does not require a full scan integration.
			// The scanner package framework is ready for integration.
			log.Info("Scanner is initialized", "seed_storages", len(seeds))
			_ = olClient
			_ = seeds
		}
	}

	if modeReport || modeCleanup {
		// Query all files from database
		files, err := db.QueryAllFiles(ctx)
		if err != nil {
			log.Error("Failed to query files", "error", err)
			return 1
		}
		if len(files) == 0 {
			log.Warn("No files in database, run --scan first")
			if modeReport {
				// Generate an empty report
				emptyData := report.ReportData{
					GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
				}
				reportPath := "report.html"
				if err := report.Generate(reportPath, emptyData); err != nil {
					log.Error("Failed to generate report", "error", err)
					return 1
				}
				log.Info("Report generated (no data)", "path", reportPath)
			}
			return 0
		}

		log.Info("Loaded files from database", "count", len(files))

		// Run duplicate detection
		detector := duplicate.New()
		var entries []duplicate.FileEntry
		for _, f := range files {
			entries = append(entries, duplicate.FileEntry{
				ID:      f.ID,
				Storage: f.Storage,
				Path:    f.Path,
				Name:    f.Name,
				Size:    f.Size,
				IsDir:   f.IsDir,
			})
		}

		groups, stats := detector.Detect(entries)
		log.Info("Duplicate detection complete",
			"total_files", stats.TotalFiles,
			"duplicate_sets", stats.DuplicateSets,
			"duplicate_files", stats.DuplicateFiles,
			"saved_space", fmt.Sprintf("%d bytes", stats.DuplicateSize),
		)

		if modeReport {
			reportData := report.ReportData{
				GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
				MovieGroups: groups,
				Stats:       stats,
			}
			reportPath := flag("--report-path", "report.html")
			if err := report.Generate(reportPath, reportData); err != nil {
				log.Error("Failed to generate report", "error", err)
				return 1
			}
			log.Info("Report generated", "path", reportPath)
		}

		if modeCleanup {
			executor := cleanup.New(olClient, !applyCleanup)
			plan := executor.CreatePlan(groups)
			planPath := flag("--plan-path", "cleanup_plan.json")
			if err := cleanup.SavePlan(planPath, plan); err != nil {
				log.Error("Failed to save cleanup plan", "error", err)
				return 1
			}
			log.Info("Cleanup plan saved",
				"path", planPath,
				"delete_entries", plan.Stats.DeleteEntries,
				"saved_space", fmt.Sprintf("%d bytes", plan.Stats.SavedSpace),
				"dry_run", plan.DryRun,
			)

			if applyCleanup {
				result, err := executor.Execute(ctx, plan)
				if err != nil {
					log.Error("Cleanup execution failed", "error", err)
					return 1
				}
				log.Info("Cleanup executed",
					"deleted", result.Deleted,
					"failed", result.Failed,
					"saved_space", fmt.Sprintf("%d bytes", result.SavedSpace),
				)
				for _, e := range result.Errors {
					log.Warn("Delete error", "path", e.Path, "error", e.Error)
				}
			}
		}
	}

	log.Info("Completed")
	return 0
}

// flag returns the value after the given flag, or defaultValue if not found.
func flag(name, defaultValue string) string {
	args := os.Args[1:]
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return defaultValue
}

// hasFlag checks if a boolean flag is present.
func hasFlag(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}

// flagInt parses an integer flag value.
func flagInt(name string, defaultValue int) int {
	args := os.Args[1:]
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			n := 0
			for _, c := range args[i+1] {
				if c >= '0' && c <= '9' {
					n = n*10 + int(c-'0')
				} else {
					break
				}
			}
			if n > 0 {
				return n
			}
		}
	}
	return defaultValue
}

