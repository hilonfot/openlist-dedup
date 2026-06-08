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
	FileID  int64  `json:"file_id"`
	Storage string `json:"storage"`
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Action  Action `json:"action"`
	Reason  string `json:"reason"`
}

// Plan is the complete cleanup plan.
type Plan struct {
	DryRun      bool        `json:"dry_run"`
	GeneratedAt string      `json:"generated_at"`
	Entries     []PlanEntry `json:"entries"`
	Stats       PlanStats   `json:"stats"`
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
	client *openlist.Client
	dryRun bool
}

// New creates a new cleanup Executor. When dryRun is true, Execute will
// simulate deletions without calling the OpenList API.
func New(client *openlist.Client, dryRun bool) *Executor {
	return &Executor{
		client: client,
		dryRun: dryRun,
	}
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
		for _, f := range g.Files {
			entry := PlanEntry{
				FileID:  f.ID,
				Storage: f.Storage,
				Path:    f.Path,
				Name:    f.Name,
				Size:    f.Size,
				Reason:  fmt.Sprintf("duplicate of %s in group %s", f.Storage, g.NormalizedName),
			}

			if f.Decision == duplicate.Delete {
				entry.Action = ActionDelete
				deleteCount++
				savedSpace += f.Size
			} else {
				entry.Action = ActionKeep
			}

			entries = append(entries, entry)
			totalSize += f.Size
		}
	}

	return Plan{
		DryRun:      e.dryRun,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
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
// Returns a summary of what was done.
func (e *Executor) Execute(ctx context.Context, plan Plan) (ExecutionResult, error) {
	result := ExecutionResult{
		DryRun:      e.dryRun || plan.DryRun,
		TotalEntries: len(plan.Entries),
	}

	for _, entry := range plan.Entries {
		if entry.Action != ActionDelete {
			result.Kept++
			continue
		}

		if result.DryRun {
			result.Simulated++
			result.SavedSpace += entry.Size
			continue
		}

		// Real deletion
		if err := e.client.Delete(ctx, entry.Path); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, DeleteError{
				Path:  entry.Path,
				Error: err.Error(),
			})
			continue
		}

		result.Deleted++
		result.SavedSpace += entry.Size
	}

	return result, nil
}

// ExecutionResult summarizes what happened during plan execution.
type ExecutionResult struct {
	DryRun       bool          `json:"dry_run"`
	TotalEntries int           `json:"total_entries"`
	Kept         int           `json:"kept"`
	Simulated    int           `json:"simulated"`
	Deleted      int           `json:"deleted"`
	Failed       int           `json:"failed"`
	SavedSpace   int64         `json:"saved_space"`
	Errors       []DeleteError `json:"errors,omitempty"`
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
