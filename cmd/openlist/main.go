package main

import (
	"context"
	"fmt"
	"net/http"
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
	"openlist/internal/scanner"
	"openlist/internal/tmdb"
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
	clearData := hasFlag("--clear-data")

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

	// Clear scan data but preserve TMDB cache
	if clearData {
		log.Info("Clearing scan data, preserving TMDB cache")
		if _, err := db.Exec("DELETE FROM media_files"); err != nil {
			log.Error("Failed to clear media_files", "error", err)
		}
		if _, err := db.Exec("DELETE FROM scan_tasks"); err != nil {
			log.Error("Failed to clear scan_tasks", "error", err)
		}
	}

	ctx := context.Background()

	// OpenList client
	olClient := openlist.New(
		cfg.OpenList.URL,
		cfg.OpenList.Password,
		time.Duration(cfg.OpenList.Timeout)*time.Second,
		cfg.OpenList.RetryMax,
	)
	if cfg.OpenList.Username != "" {
		olClient.SetUsername(cfg.OpenList.Username)
	}

	if modeScan {
		log.Info("Starting scan",
			"openlist_url", cfg.OpenList.URL,
			"workers", cfg.Scanner.Workers,
			"queue_size", cfg.Scanner.QueueSize,
		)

		// Build seed tasks from configured paths
		var seeds []scanner.ScanTask
		for _, p := range cfg.Storage.LocalPaths {
			seeds = append(seeds, scanner.ScanTask{Storage: scanner.StorageLocal, Path: p})
		}
		for _, p := range cfg.Storage.QuarkPaths {
			seeds = append(seeds, scanner.ScanTask{Storage: scanner.StorageQuark, Path: p})
		}
		for _, p := range cfg.Storage.TianyiPaths {
			seeds = append(seeds, scanner.ScanTask{Storage: scanner.StorageTianyi, Path: p})
		}

		if len(seeds) == 0 {
			log.Warn("No scan paths configured, skipping scan")
		} else {
			log.Info("Starting scanner", "seed_count", len(seeds))

			// Login to OpenList API (only if username is configured)
			if cfg.OpenList.Username != "" {
				if err := olClient.Login(ctx); err != nil {
					log.Error("Failed to login to OpenList", "error", err)
					return 1
				}
				log.Info("Login successful", "url", cfg.OpenList.URL)
			} else {
				log.Info("No username configured, using password-based auth")
			}

			// Create scanner
			s := scanner.New(scanner.Config{
				Client:    olClient,
				Workers:   cfg.Scanner.Workers,
				QueueSize: cfg.Scanner.QueueSize,
			})

			// Create batch inserter for writing results to database
			inserter := repository.NewBatchInserter(db)
			inserter.OnFlush(func(count int) {
				log.Debug("Batch flushed", "count", count)
			})

			// Start periodic flush loop (flushes every 5s if buffer not empty)
			flushCtx, flushCancel := context.WithCancel(ctx)
			defer flushCancel()
			inserter.FlushLoop(flushCtx)

			// Start BFS scanning (non-blocking)
			s.Start(ctx, seeds)

			// Consume scan results in background
			consumeDone := make(chan struct{})
			var scannedFiles int64
			var loggedCount int64
			go func() {
				defer close(consumeDone)
				for result := range s.Results() {
					if err := inserter.Insert(ctx, repository.MediaRow{
						Storage:  string(result.Storage),
						Path:     result.Path,
						Name:     result.Name,
						Size:     result.Size,
						IsDir:    result.IsDir,
						Modified: result.Modified,
					}); err != nil {
						log.Error("Failed to insert scan result", "path", result.Path, "error", err)
					}
					scannedFiles++
					// Log progress every 50 files so user can see data flowing
					if scannedFiles%50 == 0 {
						log.Info("Scan progress",
							"files_scanned", scannedFiles,
							"last_file", result.Path,
						)
					}
					if loggedCount < 5 && scannedFiles <= 5 {
						log.Info("Scanned file",
							"storage", result.Storage,
							"path", result.Path,
							"name", result.Name,
							"size", result.Size,
						)
						loggedCount++
					}
				}
			}()

			// Wait for scanner to finish (all workers done)
			s.Wait()
			// Wait for consumer to drain the channel
			<-consumeDone

			// Final flush of any remaining buffered rows
			if err := inserter.Flush(ctx); err != nil {
				log.Error("Failed to flush remaining buffer", "error", err)
			}

			stats := s.Stats()
			log.Info("Scan completed",
				"directories", stats.Directories,
				"files", stats.Files,
				"elapsed", stats.Elapsed.String(),
				"inserted", scannedFiles,
			)
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

			// Look up TMDB poster and metadata for each duplicate group
			tmdbData := make(map[string]report.TMDBItem)
			if cfg.TMDB.APIKey != "" {
				tmdbClient := tmdb.New(tmdb.Config{
					APIKey:      cfg.TMDB.APIKey,
					BaseURL:     cfg.TMDB.BaseURL,
					ImageBaseURL: cfg.TMDB.ImageBaseURL,
					Cache:       db,
					CacheTTL:    time.Duration(cfg.TMDB.CacheTTL) * time.Second,
					RateLimit:   cfg.TMDB.RateLimit,
				})
				// Quick connectivity check for TMDB
				if !tmdbCheckReachable(ctx, cfg.TMDB.BaseURL) {
					log.Warn("TMDB API is not reachable (network blocked), skipping poster lookup")
				} else {
				for _, g := range groups {
					name := g.NormalizedName
					if _, ok := tmdbData[name]; ok {
						continue
					}
					if g.IsEpisode {
						if result, err := tmdbClient.SearchTV(ctx, name, 0, 0); err == nil && result != nil {
							if err != nil {
								log.Warn("TMDB TV search failed", "name", name, "error", err)
							}
								log.Debug("TMDB: TV lookup", "name", name, "found", result != nil)
							tmdbData[name] = report.TMDBItem{
								PosterURL: result.PosterURL,
								Overview:  result.Overview,
								Rating:    result.VoteAverage,
								TMDBURL:   result.TMDBURL,
									Title:     result.Name,
							}
						}
					} else {
						if result, err := tmdbClient.SearchMovie(ctx, name, 0); err == nil && result != nil {
							if err != nil {
								log.Warn("TMDB Movie search failed", "name", name, "error", err)
							}
								log.Debug("TMDB: Movie lookup", "name", name, "found", result != nil)
							tmdbData[name] = report.TMDBItem{
								PosterURL: result.PosterURL,
								Overview:  result.Overview,
								Rating:    result.VoteAverage,
								TMDBURL:   result.TMDBURL,
									Title:     result.Title,
							}
						}
					}
				}
			}
				}

		if modeReport {
			reportData := report.ReportData{
				GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
				MovieGroups: groups,
				Stats:       stats,
				TMDBData:    tmdbData,
				StorageTrees:    report.BuildFileTree(entries, tmdbData, groups, cfg.OpenList.URL),
				OpenListBaseURL: cfg.OpenList.URL,
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

// tmdbCheckReachable tests if TMDB API is accessible.
func tmdbCheckReachable(ctx context.Context, baseURL string) bool {
	if baseURL == "" {
		baseURL = "https://tmdb-proxy.hilon2019.workers.dev"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/3/configuration?api_key=test", nil)
	if err != nil { return false }
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return false }
	resp.Body.Close()
	return resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusOK
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
