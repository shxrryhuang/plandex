package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// WORKSPACE LIFECYCLE TESTS
// =============================================================================

func TestNewWorkspace_InitialisesFields(t *testing.T) {
	ws := NewWorkspace("plan-1", "main", "proj-1", "/tmp/project", "/tmp/ws")

	if ws.PlanId != "plan-1" {
		t.Errorf("PlanId = %q, want %q", ws.PlanId, "plan-1")
	}
	if ws.Branch != "main" {
		t.Errorf("Branch = %q, want %q", ws.Branch, "main")
	}
	if ws.ProjectId != "proj-1" {
		t.Errorf("ProjectId = %q, want %q", ws.ProjectId, "proj-1")
	}
	if ws.State != WorkspaceStatePending {
		t.Errorf("State = %q, want %q", ws.State, WorkspaceStatePending)
	}
	if ws.ModifiedFiles == nil || ws.CreatedFiles == nil || ws.DeletedFiles == nil {
		t.Error("file tracking maps should be initialised")
	}
	if ws.Id == "" {
		t.Error("Id should be non-empty")
	}
}

func TestWorkspace_SetState(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")

	ws.SetState(WorkspaceStateActive)
	if ws.State != WorkspaceStateActive {
		t.Errorf("State = %q after SetState(Active)", ws.State)
	}

	ws.SetState(WorkspaceStateCommitted)
	if ws.State != WorkspaceStateCommitted {
		t.Errorf("State = %q after SetState(Committed)", ws.State)
	}
}

func TestWorkspace_Touch_UpdatesTimestamp(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")
	before := ws.LastAccessedAt

	time.Sleep(2 * time.Millisecond)
	ws.Touch()

	if !ws.LastAccessedAt.After(before) {
		t.Error("Touch() should update LastAccessedAt")
	}
}

func TestWorkspace_HasChanges(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")

	if ws.HasChanges() {
		t.Error("fresh workspace should have no changes")
	}

	ws.TrackCreatedFile("new.go", "abc123", 0644, 100)
	if !ws.HasChanges() {
		t.Error("workspace should have changes after TrackCreatedFile")
	}
}

func TestWorkspace_GetChangeCount(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")

	ws.TrackModifiedFile("a.go", "old", "new", 0644, 50)
	ws.TrackCreatedFile("b.go", "hash", 0644, 30)
	ws.TrackDeletedFile("c.go")

	mod, cre, del := ws.GetChangeCount()
	if mod != 1 || cre != 1 || del != 1 {
		t.Errorf("GetChangeCount() = (%d,%d,%d), want (1,1,1)", mod, cre, del)
	}
}

// =============================================================================
// FILE TRACKING TESTS
// =============================================================================

func TestTrackModifiedFile_RemovesFromCreatedAndDeleted(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")

	ws.TrackCreatedFile("x.go", "h1", 0644, 10)
	ws.TrackDeletedFile("x.go")
	ws.TrackModifiedFile("x.go", "orig", "curr", 0644, 20)

	if ws.IsFileCreated("x.go") {
		t.Error("file should not be in CreatedFiles after TrackModifiedFile")
	}
	if ws.IsFileDeleted("x.go") {
		t.Error("file should not be in DeletedFiles after TrackModifiedFile")
	}
	if !ws.IsFileModified("x.go") {
		t.Error("file should be in ModifiedFiles")
	}
}

func TestTrackCreatedFile_RemovesFromDeleted(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")

	ws.TrackDeletedFile("x.go")
	ws.TrackCreatedFile("x.go", "h1", 0644, 10)

	if ws.IsFileDeleted("x.go") {
		t.Error("file should not be deleted after TrackCreatedFile")
	}
	if !ws.IsFileCreated("x.go") {
		t.Error("file should be in CreatedFiles")
	}
}

func TestTrackDeletedFile_RemovesFromModifiedAndCreated(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")

	ws.TrackModifiedFile("x.go", "orig", "curr", 0644, 50)
	ws.TrackCreatedFile("y.go", "h1", 0644, 30)

	ws.TrackDeletedFile("x.go")
	ws.TrackDeletedFile("y.go")

	if ws.IsFileModified("x.go") {
		t.Error("deleted file should not be in ModifiedFiles")
	}
	if ws.IsFileCreated("y.go") {
		t.Error("deleted file should not be in CreatedFiles")
	}
}

func TestIsFileInWorkspace(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/tmp/project", "/tmp/ws")

	if ws.IsFileInWorkspace("none.go") {
		t.Error("non-existent file should not be in workspace")
	}

	ws.TrackModifiedFile("mod.go", "o", "n", 0644, 10)
	if !ws.IsFileInWorkspace("mod.go") {
		t.Error("modified file should be in workspace")
	}

	ws.TrackCreatedFile("new.go", "h", 0644, 5)
	if !ws.IsFileInWorkspace("new.go") {
		t.Error("created file should be in workspace")
	}
}

// =============================================================================
// PATH HELPERS TESTS
// =============================================================================

func TestGetFilesDir(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/project", "/workspaces")
	expected := filepath.Join(ws.WorkspaceDir, "files")
	if ws.GetFilesDir() != expected {
		t.Errorf("GetFilesDir() = %q, want %q", ws.GetFilesDir(), expected)
	}
}

func TestGetOriginalPath(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/project", "/workspaces")
	got := ws.GetOriginalPath("src/main.go")
	want := filepath.Join("/project", "src/main.go")
	if got != want {
		t.Errorf("GetOriginalPath() = %q, want %q", got, want)
	}
}

func TestGetShortId(t *testing.T) {
	ws := NewWorkspace("p", "b", "pr", "/project", "/workspaces")
	short := ws.GetShortId()
	if len(short) > 8 {
		t.Errorf("GetShortId() = %q (len %d), want max 8 chars", short, len(short))
	}
	if len(short) == 0 {
		t.Error("GetShortId() should not be empty")
	}
}

// =============================================================================
// SAVE / LOAD ROUND-TRIP TESTS
// =============================================================================

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	ws := NewWorkspace("plan-rt", "main", "proj-rt", "/project", dir)
	ws.TrackModifiedFile("src/a.go", "oldhash", "newhash", 0644, 100)
	ws.TrackCreatedFile("src/b.go", "createdhash", 0644, 50)
	ws.TrackDeletedFile("src/c.go")
	ws.SetState(WorkspaceStateActive)

	if err := ws.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := LoadWorkspace(ws.MetadataPath)
	if err != nil {
		t.Fatalf("LoadWorkspace() failed: %v", err)
	}

	if loaded.Id != ws.Id {
		t.Errorf("Id mismatch: got %q, want %q", loaded.Id, ws.Id)
	}
	if loaded.State != WorkspaceStateActive {
		t.Errorf("State = %q, want Active", loaded.State)
	}
	if !loaded.IsFileModified("src/a.go") {
		t.Error("modified file not preserved after round-trip")
	}
	if !loaded.IsFileCreated("src/b.go") {
		t.Error("created file not preserved after round-trip")
	}
	if !loaded.IsFileDeleted("src/c.go") {
		t.Error("deleted file not preserved after round-trip")
	}
}

func TestLoadWorkspace_NilMaps_Initialised(t *testing.T) {
	// Write a minimal workspace JSON with null maps
	dir := t.TempDir()
	wsPath := filepath.Join(dir, "ws", "workspace.json")
	os.MkdirAll(filepath.Dir(wsPath), 0755)

	data := []byte(`{"id":"ws-test","planId":"p","branch":"b","state":"pending","baseDir":"/tmp","workspaceDir":"` + filepath.Join(dir, "ws") + `","metadataPath":"` + wsPath + `","modifiedFiles":null,"createdFiles":null,"deletedFiles":null,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","lastAccessedAt":"2026-01-01T00:00:00Z"}`)
	os.WriteFile(wsPath, data, 0644)

	ws, err := LoadWorkspace(wsPath)
	if err != nil {
		t.Fatalf("LoadWorkspace() failed: %v", err)
	}

	if ws.ModifiedFiles == nil {
		t.Error("ModifiedFiles should be initialised on load")
	}
	if ws.CreatedFiles == nil {
		t.Error("CreatedFiles should be initialised on load")
	}
	if ws.DeletedFiles == nil {
		t.Error("DeletedFiles should be initialised on load")
	}
}

// =============================================================================
// WORKSPACE REFERENCE TESTS
// =============================================================================

func TestWorkspaceReference_SetAndGet(t *testing.T) {
	ref := NewWorkspaceReference()

	ref.SetWorkspace("plan-1", "main", "ws-abc")
	ref.SetWorkspace("plan-1", "dev", "ws-def")
	ref.SetWorkspace("plan-2", "main", "ws-ghi")

	if got := ref.GetWorkspace("plan-1", "main"); got != "ws-abc" {
		t.Errorf("GetWorkspace(plan-1, main) = %q, want ws-abc", got)
	}
	if got := ref.GetWorkspace("plan-1", "dev"); got != "ws-def" {
		t.Errorf("GetWorkspace(plan-1, dev) = %q, want ws-def", got)
	}
	if got := ref.GetWorkspace("plan-2", "main"); got != "ws-ghi" {
		t.Errorf("GetWorkspace(plan-2, main) = %q, want ws-ghi", got)
	}
	if got := ref.GetWorkspace("plan-3", "main"); got != "" {
		t.Errorf("GetWorkspace for non-existent plan should return empty, got %q", got)
	}
}

func TestWorkspaceReference_RemoveWorkspace(t *testing.T) {
	ref := NewWorkspaceReference()
	ref.SetWorkspace("plan-1", "main", "ws-abc")
	ref.SetWorkspace("plan-1", "dev", "ws-def")

	ref.RemoveWorkspace("plan-1", "main")

	if got := ref.GetWorkspace("plan-1", "main"); got != "" {
		t.Errorf("removed workspace should return empty, got %q", got)
	}
	if got := ref.GetWorkspace("plan-1", "dev"); got != "ws-def" {
		t.Errorf("other branch should be unaffected, got %q", got)
	}
}

func TestWorkspaceReference_RemoveLastBranch_CleansUpPlan(t *testing.T) {
	ref := NewWorkspaceReference()
	ref.SetWorkspace("plan-1", "main", "ws-abc")

	ref.RemoveWorkspace("plan-1", "main")

	// The plan-level map entry should be removed when empty
	if _, exists := ref.Workspaces["plan-1"]; exists {
		t.Error("plan entry should be removed when all branches are gone")
	}
}

func TestWorkspaceReference_SaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	refPath := filepath.Join(dir, "workspaces-v2.json")

	ref := NewWorkspaceReference()
	ref.SetWorkspace("plan-1", "main", "ws-abc")

	if err := ref.Save(refPath); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := LoadWorkspaceReference(refPath)
	if err != nil {
		t.Fatalf("LoadWorkspaceReference() failed: %v", err)
	}

	if got := loaded.GetWorkspace("plan-1", "main"); got != "ws-abc" {
		t.Errorf("round-trip: GetWorkspace = %q, want ws-abc", got)
	}
}

func TestLoadWorkspaceReference_FileNotExist_ReturnsEmpty(t *testing.T) {
	ref, err := LoadWorkspaceReference("/nonexistent/path/ref.json")
	if err != nil {
		t.Fatalf("should not error for missing file, got: %v", err)
	}
	if ref == nil || ref.Workspaces == nil {
		t.Error("should return empty reference, not nil")
	}
}

// =============================================================================
// HASH HELPERS TESTS
// =============================================================================

func TestHashContent_Deterministic(t *testing.T) {
	content := []byte("hello world")
	h1 := HashContent(content)
	h2 := HashContent(content)

	if h1 != h2 {
		t.Error("HashContent should be deterministic")
	}
	if h1 == "" {
		t.Error("HashContent should not return empty string")
	}
}

func TestHashString_MatchesHashContent(t *testing.T) {
	s := "test content"
	if HashString(s) != HashContent([]byte(s)) {
		t.Error("HashString should match HashContent for same input")
	}
}

// =============================================================================
// SUMMARY TESTS
// =============================================================================

func TestGetSummary(t *testing.T) {
	ws := NewWorkspace("plan-s", "main", "proj-s", "/project", "/ws")
	ws.TrackModifiedFile("a.go", "o", "n", 0644, 10)
	ws.TrackCreatedFile("b.go", "h", 0644, 5)

	summary := ws.GetSummary()

	if summary.Id != ws.Id {
		t.Errorf("Summary.Id = %q, want %q", summary.Id, ws.Id)
	}
	if summary.ModifiedCount != 1 {
		t.Errorf("ModifiedCount = %d, want 1", summary.ModifiedCount)
	}
	if summary.CreatedCount != 1 {
		t.Errorf("CreatedCount = %d, want 1", summary.CreatedCount)
	}
	if summary.DeletedCount != 0 {
		t.Errorf("DeletedCount = %d, want 0", summary.DeletedCount)
	}
}
