package cleanup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"openlist/internal/duplicate"
	"openlist/internal/openlist"
)

// Action is the cleanup action for a file.
type Action string

const (
	ActionKeep   Action = "keep"
	ActionDelete Action = "delete"
)

// PlanEntry represents a single file decision in the cleanup plan.
type PlanEntry struct {
	FileID    int64  `json:"file_id"`
	Storage   string `json:"storage"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Action    Action `json:"action"`
	Reason    string `json:"reason"`
	DeletedAt string `json:"deleted_at,omitempty"` // populated when actually deleted
}

// Plan is the complete cleanup plan.
type Plan struct {
	DryRun      bool        `json:"dry_run"`
	GeneratedAt string      `json:"generated_at"`
	TrashDir    string      `json:"trash_dir,omitempty"` // conceptual trash directory
	Entries     []PlanEntry `json:"entries"`
	Stats       PlanStats   `json:"stats"`
}

// RecoveryEntry holds the minimum info needed to locate a deleted file.
type RecoveryEntry struct {
	Storage   string `json:"storage"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Reason    string `json:"reason"`
	DeletedAt string `json:"deleted_at"`
}

// PlanStats holds summary statistics for the plan.
type PlanStats struct {
	TotalEntries  int   `json:"total_entries"`
	DeleteEntries int   `json:"delete_entries"`
	TotalSize     int64 `json:"total_size"`
	SavedSpace    int64 `json:"saved_space"`
}

// Executor handles cleanup plan creation and execution.
type Executor struct {
	client   *openlist.Client
	dryRun   bool
	trashDir string
}

// TrashDirDefault is the default conceptual trash directory.
const TrashDirDefault = "/.openlist-trash"

// New creates a new cleanup Executor. When dryRun is true, Execute will
// simulate deletions without calling the OpenList API.
func New(client *openlist.Client, dryRun bool) *Executor {
	return &Executor{
		client:   client,
		dryRun:   dryRun,
		trashDir: TrashDirDefault,
	}
}

// SetTrashDir sets the conceptual trash directory path.
func (e *Executor) SetTrashDir(dir string) {
	e.trashDir = dir
}

// CreatePlan builds a cleanup plan from duplicate groups. Files marked as
// Delete in the groups become delete entries in the plan. Files marked as
// Keep are included for reference.
func (e *Executor) CreatePlan(groups []duplicate.DuplicateGroup) Plan {
	var entries []PlanEntry
	var totalSize int64
	var savedSpace int64
	var deleteCount int

	for _, g := range groups {
		// Safety invariant: every group must retain at least one file. If the
		// detector somehow marked every file in a group for deletion, fall back
		// to keeping all of them rather than wiping the only copies.
		keepCount := 0
		for _, f := range g.Files {
			if f.Decision != duplicate.Delete {
				keepCount++
			}
		}
		forceKeep := keepCount == 0 && len(g.Files) > 0

		for _, f := range g.Files {
			entry := PlanEntry{
				FileID:  f.ID,
				Storage: f.Storage,
				Path:    f.Path,
				Name:    f.Name,
				Size:    f.Size,
				Reason:  fmt.Sprintf("duplicate of %s in group %s", f.Storage, g.NormalizedName),
			}

			if f.Decision == duplicate.Delete && !forceKeep {
				entry.Action = ActionDelete
				deleteCount++
				savedSpace += f.Size
			} else {
				entry.Action = ActionKeep
				if forceKeep {
					entry.Reason = "kept: group would otherwise lose all copies"
				}
			}

			entries = append(entries, entry)
			totalSize += f.Size
		}
	}

	return Plan{
		DryRun:      e.dryRun,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TrashDir:    e.trashDir,
		Entries:     entries,
		Stats: PlanStats{
			TotalEntries:  len(entries),
			DeleteEntries: deleteCount,
			TotalSize:     totalSize,
			SavedSpace:    savedSpace,
		},
	}
}

// SavePlan writes the plan to a JSON file.
func SavePlan(path string, plan Plan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}

// LoadPlan reads a plan from a JSON file.
func LoadPlan(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read plan: %w", err)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, fmt.Errorf("unmarshal plan: %w", err)
	}
	return plan, nil
}

// Execute applies the cleanup plan. In dry-run mode it only logs actions.
// In apply mode it calls the OpenList Delete API for each delete entry.
// Returns a summary of what was done and a list of recovery entries for
// any files that were actually deleted.
func (e *Executor) Execute(ctx context.Context, plan Plan) (ExecutionResult, error) {
	result := ExecutionResult{
		DryRun:       e.dryRun || plan.DryRun,
		TotalEntries: len(plan.Entries),
		Recovery:     make([]RecoveryEntry, 0),
	}
	now := time.Now().UTC().Format(time.RFC3339)

	for i := range plan.Entries {
		entry := &plan.Entries[i]
		if entry.Action != ActionDelete {
			result.Kept++
			continue
		}

		if result.DryRun {
			result.Simulated++
			result.SavedSpace += entry.Size
			continue
		}

		// Real deletion. Verify the file still matches the plan before removing
		// it: the plan may have been generated in an earlier run and the file
		// could have been moved, replaced, or resized since. Deletion is
		// permanent, so a mismatch must abort this entry rather than risk
		// removing the wrong content.
		info, err := e.client.Get(ctx, entry.Path)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, DeleteError{
				Path:  entry.Path,
				Error: fmt.Sprintf("verify before delete: %v", err),
			})
			continue
		}
		if info.IsDir || info.Size != entry.Size {
			result.Failed++
			result.Errors = append(result.Errors, DeleteError{
				Path: entry.Path,
				Error: fmt.Sprintf("stale plan: file changed (is_dir=%t, size %d != expected %d), skipped",
					info.IsDir, info.Size, entry.Size),
			})
			continue
		}

		if err := e.client.Delete(ctx, entry.Path); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, DeleteError{
				Path:  entry.Path,
				Error: err.Error(),
			})
			continue
		}

		entry.DeletedAt = now
		result.Deleted++
		result.SavedSpace += entry.Size
		result.Recovery = append(result.Recovery, RecoveryEntry{
			Storage:   entry.Storage,
			Path:      entry.Path,
			Name:      entry.Name,
			Size:      entry.Size,
			Reason:    entry.Reason,
			DeletedAt: now,
		})
	}

	return result, nil
}

// ExecutionResult summarizes what happened during plan execution.
type ExecutionResult struct {
	DryRun       bool            `json:"dry_run"`
	TotalEntries int             `json:"total_entries"`
	Kept         int             `json:"kept"`
	Simulated    int             `json:"simulated"`
	Deleted      int             `json:"deleted"`
	Failed       int             `json:"failed"`
	SavedSpace   int64           `json:"saved_space"`
	Errors       []DeleteError   `json:"errors,omitempty"`
	Recovery     []RecoveryEntry `json:"recovery,omitempty"` // only populated for real deletions
}

// DeleteError represents a failed deletion.
type DeleteError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Summary returns a human-readable summary of the execution result.
func (r ExecutionResult) Summary() string {
	if r.TotalEntries == 0 {
		return "No entries to process."
	}

	s := fmt.Sprintf("Total: %d entries\n", r.TotalEntries)
	s += fmt.Sprintf("  Keep:     %d\n", r.Kept)

	if r.DryRun {
		s += fmt.Sprintf("  Delete (simulated): %d\n", r.Simulated)
		s += fmt.Sprintf("  Space saved:       %s\n", formatSize(r.SavedSpace))
		s += "\n  DRY RUN — no files were actually deleted."
	} else {
		s += fmt.Sprintf("  Deleted:  %d\n", r.Deleted)
		if r.Failed > 0 {
			s += fmt.Sprintf("  Failed:   %d\n", r.Failed)
		}
		s += fmt.Sprintf("  Space saved: %s\n", formatSize(r.SavedSpace))
	}

	return s
}

// IsDryRun returns true if the executor is in dry-run mode.
func (e *Executor) IsDryRun() bool {
	return e.dryRun
}

// formatSize converts bytes to a human-readable string.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.2f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// SaveResult writes the execution result as JSON to a file.
func SaveResult(path string, result ExecutionResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

// LoadResult reads an execution result from a JSON file.
func LoadResult(path string) (ExecutionResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("read result: %w", err)
	}
	var result ExecutionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return ExecutionResult{}, fmt.Errorf("unmarshal result: %w", err)
	}
	return result, nil
}

// GenerateRestoreGuide returns a human-readable text guide describing what
// was deleted and how to attempt recovery.
func GenerateRestoreGuide(result ExecutionResult) string {
	if len(result.Recovery) == 0 {
		return "No files were deleted — nothing to restore."
	}

	s := fmt.Sprintf("=== RESTORE GUIDE ===\n")
	s += fmt.Sprintf("Deleted at: %s\n", time.Now().UTC().Format(time.RFC3339))
	s += fmt.Sprintf("Files deleted: %d\n", len(result.Recovery))
	s += fmt.Sprintf("Total space: %s\n\n", formatSize(result.SavedSpace))
	s += "The following files were permanently deleted from OpenList.\n"
	s += "To restore, re-upload the original media files to the indicated paths.\n\n"
	s += fmt.Sprintf("%-10s %-60s %s\n", "STORAGE", "PATH", "SIZE")
	s += "-------------------------------------------------------------------------------\n"

	for _, r := range result.Recovery {
		s += fmt.Sprintf("%-10s %-60s %s\n", r.Storage, r.Path, formatSize(r.Size))
	}

	s += "\n---\n"
	s += "Note: OpenList does not support a move-to-trash API.\n"
	s += "Files were permanently deleted via /api/fs/remove.\n"
	s += "Consider keeping your original media source as a backup.\n"
	return s
}
