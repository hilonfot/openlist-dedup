package cleanup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openlist/internal/duplicate"
	"openlist/internal/openlist"
)

// --- CreatePlan tests ---

func TestCreatePlan_EmptyGroups(t *testing.T) {
	e := New(nil, true)
	plan := e.CreatePlan(nil)

	if plan.Stats.TotalEntries != 0 {
		t.Errorf("expected 0 entries, got %d", plan.Stats.TotalEntries)
	}
	if !plan.DryRun {
		t.Error("expected dry_run=true")
	}
}

func TestCreatePlan_WithDuplicates(t *testing.T) {
	e := New(nil, true)
	groups := []duplicate.DuplicateGroup{
		{
			NormalizedName: "Avatar",
			Files: []duplicate.FileEntry{
				{ID: 1, Storage: "local", Path: "/local/avatar.mkv", Name: "Avatar.mkv", Size: 2000000000, Decision: duplicate.Keep},
				{ID: 2, Storage: "quark", Path: "/quark/avatar.mkv", Name: "Avatar.mkv", Size: 2000000000, Decision: duplicate.Delete},
			},
		},
	}

	plan := e.CreatePlan(groups)

	if plan.Stats.TotalEntries != 2 {
		t.Errorf("expected 2 entries, got %d", plan.Stats.TotalEntries)
	}
	if plan.Stats.DeleteEntries != 1 {
		t.Errorf("expected 1 delete entry, got %d", plan.Stats.DeleteEntries)
	}
	if plan.Stats.SavedSpace != 2000000000 {
		t.Errorf("expected saved space 2000000000, got %d", plan.Stats.SavedSpace)
	}
	if plan.Stats.TotalSize != 4000000000 {
		t.Errorf("expected total size 4000000000, got %d", plan.Stats.TotalSize)
	}

	// Verify entry details
	var deleteEntry, keepEntry *PlanEntry
	for i, e := range plan.Entries {
		if e.Action == ActionDelete {
			deleteEntry = &plan.Entries[i]
		} else {
			keepEntry = &plan.Entries[i]
		}
	}

	if deleteEntry == nil {
		t.Fatal("expected a delete entry")
	}
	if keepEntry == nil {
		t.Fatal("expected a keep entry")
	}

	if deleteEntry.Storage != "quark" {
		t.Errorf("expected delete entry storage quark, got %s", deleteEntry.Storage)
	}
	if keepEntry.Storage != "local" {
		t.Errorf("expected keep entry storage local, got %s", keepEntry.Storage)
	}
	if deleteEntry.Path != "/quark/avatar.mkv" {
		t.Errorf("expected delete path, got %s", deleteEntry.Path)
	}
}

func TestCreatePlan_MultipleGroups(t *testing.T) {
	e := New(nil, true)
	groups := []duplicate.DuplicateGroup{
		{
			NormalizedName: "Movie1",
			Files: []duplicate.FileEntry{
				{ID: 1, Storage: "local", Path: "/m1.mkv", Decision: duplicate.Keep},
				{ID: 2, Storage: "quark", Path: "/m1_q.mkv", Decision: duplicate.Delete},
			},
		},
		{
			NormalizedName: "Movie2",
			Files: []duplicate.FileEntry{
				{ID: 3, Storage: "tianyi", Path: "/m2.mkv", Decision: duplicate.Keep},
				{ID: 4, Storage: "quark", Path: "/m2_q.mkv", Decision: duplicate.Delete},
			},
		},
	}

	plan := e.CreatePlan(groups)

	if plan.Stats.DeleteEntries != 2 {
		t.Errorf("expected 2 delete entries, got %d", plan.Stats.DeleteEntries)
	}
	if plan.Stats.TotalEntries != 4 {
		t.Errorf("expected 4 total entries, got %d", plan.Stats.TotalEntries)
	}
}

// --- Save/Load Plan tests ---

func TestSaveAndLoadPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cleanup_plan.json")

	original := Plan{
		DryRun:      true,
		GeneratedAt: "2024-01-01T00:00:00Z",
		Entries: []PlanEntry{
			{FileID: 1, Storage: "quark", Path: "/test.mkv", Action: ActionDelete, Size: 1000},
		},
		Stats: PlanStats{
			TotalEntries:  1,
			DeleteEntries: 1,
			SavedSpace:    1000,
		},
	}

	if err := SavePlan(path, original); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	loaded, err := LoadPlan(path)
	if err != nil {
		t.Fatalf("LoadPlan failed: %v", err)
	}

	if loaded.DryRun != original.DryRun {
		t.Errorf("expected DryRun %v, got %v", original.DryRun, loaded.DryRun)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded.Entries))
	}
	if loaded.Entries[0].Path != "/test.mkv" {
		t.Errorf("expected path /test.mkv, got %s", loaded.Entries[0].Path)
	}
	if loaded.Stats.SavedSpace != 1000 {
		t.Errorf("expected saved space 1000, got %d", loaded.Stats.SavedSpace)
	}
}

func TestLoadPlan_NonExistent(t *testing.T) {
	_, err := LoadPlan("/nonexistent/path/plan.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestSavePlan_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.json")

	plan := Plan{
		DryRun: false,
		Entries: []PlanEntry{
			{FileID: 1, Storage: "local", Path: "/f.mkv", Size: 500, Action: ActionKeep, Reason: "original"},
		},
		Stats: PlanStats{TotalEntries: 1},
	}

	if err := SavePlan(path, plan); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var decoded Plan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
}

// --- Execute tests ---

func TestExecute_DryRun(t *testing.T) {
	e := New(nil, true)

	plan := Plan{
		DryRun: true,
		Entries: []PlanEntry{
			{FileID: 1, Storage: "quark", Path: "/del.mkv", Action: ActionDelete, Size: 1000},
			{FileID: 2, Storage: "local", Path: "/keep.mkv", Action: ActionKeep, Size: 1000},
		},
		Stats: PlanStats{DeleteEntries: 1},
	}

	result, err := e.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.DryRun {
		t.Error("expected dry_run=true")
	}
	if result.Simulated != 1 {
		t.Errorf("expected 1 simulated deletion, got %d", result.Simulated)
	}
	if result.Deleted != 0 {
		t.Errorf("expected 0 real deletions, got %d", result.Deleted)
	}
	if result.Kept != 1 {
		t.Errorf("expected 1 kept, got %d", result.Kept)
	}
	if result.SavedSpace != 1000 {
		t.Errorf("expected saved space 1000, got %d", result.SavedSpace)
	}

	summary := result.Summary()
	if !strings.Contains(summary, "DRY RUN") {
		t.Error("expected DRY RUN in summary")
	}
}

func TestExecute_RealDelete(t *testing.T) {
	var deletedPath string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		deletedPath = req.Path
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
		})
	}))
	defer mock.Close()

	client := openlist.New(mock.URL, "", 0, 0)
	e := New(client, false)

	plan := Plan{
		DryRun: false,
		Entries: []PlanEntry{
			{FileID: 1, Storage: "quark", Path: "/movies/del.mkv", Size: 500000000, Action: ActionDelete},
			{FileID: 2, Storage: "local", Path: "/movies/keep.mkv", Size: 500000000, Action: ActionKeep},
		},
	}

	result, err := e.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", result.Deleted)
	}
	if result.Kept != 1 {
		t.Errorf("expected 1 kept, got %d", result.Kept)
	}
	if result.SavedSpace != 500000000 {
		t.Errorf("expected saved space 500000000, got %d", result.SavedSpace)
	}
	if deletedPath != "/movies/del.mkv" {
		t.Errorf("expected deleted path /movies/del.mkv, got %s", deletedPath)
	}
}

func TestExecute_DeleteError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    500,
			"message": "internal error",
		})
	}))
	defer mock.Close()

	client := openlist.New(mock.URL, "", 0, 1)
	e := New(client, false)

	plan := Plan{
		Entries: []PlanEntry{
			{FileID: 1, Storage: "quark", Path: "/fail.mkv", Action: ActionDelete},
		},
	}

	result, err := e.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
	if result.Deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", result.Deleted)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestExecute_EmptyPlan(t *testing.T) {
	e := New(nil, false)
	plan := Plan{Entries: []PlanEntry{}}

	result, err := e.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	summary := result.Summary()
	if !strings.Contains(summary, "No entries") {
		t.Error("expected 'No entries' in summary for empty plan")
	}
}

func TestIsDryRun(t *testing.T) {
	e1 := New(nil, true)
	if !e1.IsDryRun() {
		t.Error("expected IsDryRun=true")
	}

	e2 := New(nil, false)
	if e2.IsDryRun() {
		t.Error("expected IsDryRun=false")
	}
}

func TestCreatePlan_GeneratedAt(t *testing.T) {
	e := New(nil, true)
	plan := e.CreatePlan(nil)

	if plan.GeneratedAt == "" {
		t.Error("expected GeneratedAt to be set")
	}
}

func TestExecute_DryRunMode(t *testing.T) {
	// Executor in dry-run mode but plan says apply — dry run should win
	e := New(nil, true)
	plan := Plan{
		DryRun: false,
		Entries: []PlanEntry{
			{FileID: 1, Storage: "quark", Path: "/f.mkv", Action: ActionDelete},
		},
	}

	result, err := e.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.DryRun {
		t.Error("expected dry_run=true when executor is in dry-run mode")
	}
	if result.Simulated != 1 {
		t.Errorf("expected 1 simulated, got %d", result.Simulated)
	}
}
