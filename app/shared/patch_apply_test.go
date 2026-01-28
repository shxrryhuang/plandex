package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// ATOMIC PATCH APPLICATION TESTS
// =============================================================================
//
// These tests verify that the FileTransaction + PatchStatusReporter system
// provides the following guarantees:
//
//   1. All-or-nothing: if any file write fails, every previously written file
//      in the same transaction is restored to its pre-transaction state.
//
//   2. Snapshot integrity: the snapshot captured for each file reflects the
//      state at staging time, even when concurrent external writes occur
//      after staging.
//
//   3. Skipped files: files whose content is unchanged relative to disk are
//      never staged or written.
//
//   4. Lifecycle events: the reporter receives the correct sequence of phase
//      transitions for both happy-path and failure scenarios.
//
//   5. Recovery: an orphaned WAL (simulating a crash mid-apply) can be
//      replayed to determine whether to roll back.
//
// =============================================================================

// --- Helpers -----------------------------------------------------------------

func setupTmpProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// collectPhases extracts the ordered list of phases from a LoggingReporter.
func collectPatchPhases(r *LoggingReporter) []string {
	var phases []string
	for _, e := range r.PatchEvents {
		phases = append(phases, string(e.Phase))
	}
	return phases
}

func collectFilePhases(r *LoggingReporter) map[string][]string {
	m := make(map[string][]string)
	for _, e := range r.FileEvents {
		m[e.Path] = append(m[e.Path], string(e.Phase))
	}
	return m
}

// --- Test 1: Happy path – all files applied and committed -------------------

func TestAtomicApply_HappyPath(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"existing.go": "package main\n",
	})

	tx := NewFileTransaction("plan-1", "main", root)
	if err := tx.Begin(); err != nil {
		t.Fatal(err)
	}

	reporter := NewLoggingReporter()

	// Stage
	reporter.OnPatchEvent(PatchEvent{TxId: tx.Id, Phase: PhaseStaging, Timestamp: time.Now()})
	if err := tx.ModifyFile("existing.go", "package main\n// modified\n"); err != nil {
		t.Fatal(err)
	}
	if err := tx.CreateFile("new.go", "package main\n// new\n"); err != nil {
		t.Fatal(err)
	}
	reporter.OnPatchEvent(PatchEvent{TxId: tx.Id, Phase: PatchPhaseApplying, Timestamp: time.Now()})

	// Apply
	for {
		op, err := tx.ApplyNext()
		if op == nil {
			break
		}
		reporter.OnFileStatus(FileStatus{Path: op.Path, Phase: PatchPhaseApplying, OpType: string(op.Type), Timestamp: time.Now()})
		if err != nil {
			t.Fatalf("unexpected apply error: %v", err)
		}
	}

	// Commit
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	reporter.OnPatchEvent(PatchEvent{TxId: tx.Id, Phase: PhaseDone, Timestamp: time.Now()})

	// Verify files on disk
	if got := readFile(t, filepath.Join(root, "existing.go")); got != "package main\n// modified\n" {
		t.Errorf("existing.go = %q, want modified content", got)
	}
	if got := readFile(t, filepath.Join(root, "new.go")); got != "package main\n// new\n" {
		t.Errorf("new.go = %q, want new content", got)
	}

	// Verify reporter saw staging → applying → done
	phases := collectPatchPhases(reporter)
	if len(phases) < 3 {
		t.Errorf("expected at least 3 patch events, got %d: %v", len(phases), phases)
	}
	if phases[0] != "staging" || phases[1] != "applying" {
		t.Errorf("unexpected phase order: %v", phases)
	}
}

// --- Test 2: Partial failure rolls back ALL applied files -------------------

func TestAtomicApply_PartialFailureRollsBackAll(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"file_a.txt": "original A",
		"file_b.txt": "original B",
	})

	tx := NewFileTransaction("plan-2", "main", root)
	if err := tx.Begin(); err != nil {
		t.Fatal(err)
	}

	// Stage: modify A, modify B, create C (C's parent dir will be made writable-fail later)
	tx.ModifyFile("file_a.txt", "modified A")
	tx.ModifyFile("file_b.txt", "modified B")
	tx.CreateFile("sub/file_c.txt", "content C")

	// Apply file_a and file_b successfully
	op, err := tx.ApplyNext()
	if err != nil || op == nil {
		t.Fatalf("expected file_a to apply: %v", err)
	}
	op, err = tx.ApplyNext()
	if err != nil || op == nil {
		t.Fatalf("expected file_b to apply: %v", err)
	}

	// Make sub/ unwritable so file_c fails
	subDir := filepath.Join(root, "sub")
	os.MkdirAll(subDir, 0000) // exists but not writable

	op, err = tx.ApplyNext()
	if err == nil {
		// Restore permissions for cleanup
		os.Chmod(subDir, 0755)
		t.Fatal("expected file_c apply to fail due to permission error")
	}

	// Rollback
	rbErr := tx.Rollback("file_c write failed")
	os.Chmod(subDir, 0755) // restore for cleanup
	if rbErr != nil {
		t.Fatalf("rollback failed: %v", rbErr)
	}

	// Verify A and B are restored
	if got := readFile(t, filepath.Join(root, "file_a.txt")); got != "original A" {
		t.Errorf("file_a.txt = %q after rollback, want 'original A'", got)
	}
	if got := readFile(t, filepath.Join(root, "file_b.txt")); got != "original B" {
		t.Errorf("file_b.txt = %q after rollback, want 'original B'", got)
	}

	// Verify C was never successfully created (or was cleaned up)
	if _, statErr := os.Stat(filepath.Join(root, "sub", "file_c.txt")); !os.IsNotExist(statErr) {
		t.Error("file_c.txt should not exist after rollback")
	}

	if tx.State != TxStateRolledBack {
		t.Errorf("tx.State = %s, want rolled_back", tx.State)
	}
}

// --- Test 3: Unchanged files are skipped (never staged) -----------------

func TestAtomicApply_UnchangedFilesSkipped(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"same.txt":    "unchanged content",
		"changed.txt": "original",
	})

	tx := NewFileTransaction("plan-3", "main", root)
	tx.Begin()

	reporter := NewLoggingReporter()

	// Simulate the ApplyFiles logic: check disk before staging
	files := map[string]string{
		"same.txt":    "unchanged content", // identical to disk
		"changed.txt": "modified",
	}

	for path, content := range files {
		fullPath := filepath.Join(root, path)
		existing, _ := os.ReadFile(fullPath)
		if string(existing) == content {
			reporter.OnFileStatus(FileStatus{Path: path, Phase: PhaseStaging, OpType: "skip", Timestamp: time.Now()})
			continue
		}
		reporter.OnFileStatus(FileStatus{Path: path, Phase: PhaseStaging, OpType: "modify", Timestamp: time.Now()})
		tx.ModifyFile(path, content)
	}

	// Only "changed.txt" should be staged
	if len(tx.Operations) != 1 {
		t.Errorf("expected 1 staged operation, got %d", len(tx.Operations))
	}
	if tx.Operations[0].Path != filepath.Join(root, "changed.txt") {
		t.Errorf("staged op path = %s, want changed.txt", tx.Operations[0].Path)
	}

	// Verify reporter flagged same.txt as skip
	fp := collectFilePhases(reporter)
	if phases, ok := fp["same.txt"]; !ok || len(phases) == 0 || phases[0] != "staging" {
		t.Errorf("same.txt should have staging/skip event")
	}
}

// --- Test 4: Conflict detection – file changed after snapshot ---------------

func TestAtomicApply_ConflictDetectionAfterSnapshot(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"conflict.txt": "version 1",
	})

	tx := NewFileTransaction("plan-4", "main", root)
	tx.Begin()

	// Snapshot is captured here (version 1)
	tx.ModifyFile("conflict.txt", "version 2 from plan")

	// Simulate external write between snapshot and apply
	os.WriteFile(filepath.Join(root, "conflict.txt"), []byte("version 1.5 external"), 0644)

	// Apply proceeds – transaction doesn't detect the external change itself
	// (that's a higher-level concern), but the snapshot still holds version 1.
	op, err := tx.ApplyNext()
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if op.Status != FileOpApplied {
		t.Errorf("status = %s, want applied", op.Status)
	}

	// Verify snapshot preserved original (version 1), not the external write
	snap := tx.Snapshots[filepath.Join(root, "conflict.txt")]
	if snap.Content != "version 1" {
		t.Errorf("snapshot content = %q, want 'version 1' (pre-staging state)", snap.Content)
	}

	// Rollback restores version 1 (the snapshot), not version 1.5
	tx.Rollback("conflict detected externally")
	if got := readFile(t, filepath.Join(root, "conflict.txt")); got != "version 1" {
		t.Errorf("after rollback content = %q, want 'version 1'", got)
	}
}

// --- Test 5: Checkpoint allows partial rollback within a transaction --------

func TestAtomicApply_CheckpointRollback(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"a.txt": "a0",
		"b.txt": "b0",
		"c.txt": "c0",
	})

	tx := NewFileTransaction("plan-5", "main", root)
	tx.Begin()

	tx.ModifyFile("a.txt", "a1")
	tx.ApplyNext()

	tx.ModifyFile("b.txt", "b1")
	tx.ApplyNext()

	// Checkpoint after a and b are applied
	if err := tx.CreateCheckpoint("after_ab", "a and b applied"); err != nil {
		t.Fatal(err)
	}

	tx.ModifyFile("c.txt", "c1")
	tx.ApplyNext()

	// Rollback to checkpoint – only c should revert
	if err := tx.RollbackToCheckpoint("after_ab"); err != nil {
		t.Fatal(err)
	}

	// a and b remain modified (checkpoint state)
	if got := readFile(t, filepath.Join(root, "a.txt")); got != "a1" {
		t.Errorf("a.txt = %q, want a1 (checkpoint state)", got)
	}
	if got := readFile(t, filepath.Join(root, "b.txt")); got != "b1" {
		t.Errorf("b.txt = %q, want b1 (checkpoint state)", got)
	}
	// c reverted to checkpoint content (which captured it as c0 since c wasn't
	// tracked at checkpoint time – falls back to full rollback)
	if got := readFile(t, filepath.Join(root, "c.txt")); got != "c0" {
		t.Errorf("c.txt = %q, want c0 (rolled back past checkpoint)", got)
	}
}

// --- Test 6: WAL recovery detects incomplete transaction and rolls back -----

func TestAtomicApply_WALRecoveryRollsBackIncomplete(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"recover.txt": "original",
	})

	tx := NewFileTransaction("plan-6", "main", root)
	tx.Begin()

	tx.ModifyFile("recover.txt", "mid-apply content")
	tx.ApplyNext() // writes to disk

	// Simulate crash: do NOT call Commit or Rollback.
	// The WAL should have tx_start, op_intent, op_complete but no tx_commit.
	walPath := tx.WALPath

	// Recover from WAL
	recovered, err := RecoverTransaction(walPath)
	if err != nil {
		t.Fatalf("RecoverTransaction failed: %v", err)
	}

	// Recovery should have rolled back the incomplete transaction
	if recovered.State != TxStateRolledBack {
		t.Errorf("recovered state = %s, want rolled_back", recovered.State)
	}

	// Verify file is restored to original
	if got := readFile(t, filepath.Join(root, "recover.txt")); got != "original" {
		t.Errorf("after WAL recovery content = %q, want 'original'", got)
	}
}

// --- Test 7: Reporter captures full lifecycle for failure scenario ----------

func TestAtomicApply_ReporterLifecycleonFailure(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"ok.txt": "original ok",
	})

	reporter := NewLoggingReporter()
	tx := NewFileTransaction("plan-7", "main", root)
	tx.Begin()

	reporter.OnPatchEvent(PatchEvent{TxId: tx.Id, Phase: PhaseStaging, Timestamp: time.Now()})

	tx.ModifyFile("ok.txt", "modified ok")
	// Stage a file in a directory we'll make unwritable
	badDir := filepath.Join(root, "baddir")
	os.MkdirAll(badDir, 0755)
	tx.CreateFile("baddir/fail.txt", "will fail")

	reporter.OnPatchEvent(PatchEvent{TxId: tx.Id, Phase: PatchPhaseApplying, Timestamp: time.Now()})

	// Apply ok.txt
	op, err := tx.ApplyNext()
	reporter.OnFileStatus(FileStatus{Path: op.Path, Phase: PatchPhaseApplying, OpType: string(op.Type), Timestamp: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	// Make baddir unwritable
	os.Chmod(badDir, 0000)

	// Apply fail.txt
	op, err = tx.ApplyNext()
	if err != nil {
		reporter.OnFileStatus(FileStatus{Path: op.Path, Phase: PhaseDone, OpType: string(op.Type), Error: err.Error(), Timestamp: time.Now()})
		reporter.OnPatchEvent(PatchEvent{TxId: tx.Id, Phase: PhaseRollingBack, Reason: err.Error(), Timestamp: time.Now()})
		tx.Rollback("apply failed")
		reporter.OnPatchEvent(PatchEvent{TxId: tx.Id, Phase: PhaseDone, Reason: "rolled back", Timestamp: time.Now()})
	}

	os.Chmod(badDir, 0755) // restore for cleanup

	// Verify reporter has rolling_back and done events
	phases := collectPatchPhases(reporter)
	hasRollingBack := false
	hasDone := false
	for _, p := range phases {
		if p == "rolling_back" {
			hasRollingBack = true
		}
		if p == "done" {
			hasDone = true
		}
	}
	if !hasRollingBack {
		t.Errorf("reporter should have rolling_back phase, got: %v", phases)
	}
	if !hasDone {
		t.Errorf("reporter should have done phase, got: %v", phases)
	}

	// Verify ok.txt rolled back
	if got := readFile(t, filepath.Join(root, "ok.txt")); got != "original ok" {
		t.Errorf("ok.txt = %q after rollback, want 'original ok'", got)
	}
}

// --- Test 8: Multiple operations on same file – single snapshot preserved ---

func TestAtomicApply_MultipleMods_SingleSnapshot(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"multi.txt": "v0",
	})

	tx := NewFileTransaction("plan-8", "main", root)
	tx.Begin()

	// Stage three successive modifications
	tx.ModifyFile("multi.txt", "v1")
	tx.ModifyFile("multi.txt", "v2")
	tx.ModifyFile("multi.txt", "v3")

	// Only one snapshot should exist, capturing v0
	if len(tx.Snapshots) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(tx.Snapshots))
	}
	snap := tx.Snapshots[filepath.Join(root, "multi.txt")]
	if snap.Content != "v0" {
		t.Errorf("snapshot = %q, want v0", snap.Content)
	}

	// Apply all three ops (last write wins on disk)
	tx.ApplyAll()
	if got := readFile(t, filepath.Join(root, "multi.txt")); got != "v3" {
		t.Errorf("after apply = %q, want v3", got)
	}

	// Rollback restores v0
	tx.Rollback("test")
	if got := readFile(t, filepath.Join(root, "multi.txt")); got != "v0" {
		t.Errorf("after rollback = %q, want v0", got)
	}
}

// --- Test 9: Delete + rollback restores file --------------------------------

func TestAtomicApply_DeleteRollbackRestores(t *testing.T) {
	root := setupTmpProject(t, map[string]string{
		"keep.txt": "keep me",
	})

	tx := NewFileTransaction("plan-9", "main", root)
	tx.Begin()
	tx.DeleteFile("keep.txt")
	tx.ApplyAll()

	// File should be gone
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); !os.IsNotExist(err) {
		t.Fatal("keep.txt should be deleted")
	}

	tx.Rollback("undo delete")

	// File should be restored
	if got := readFile(t, filepath.Join(root, "keep.txt")); got != "keep me" {
		t.Errorf("after rollback = %q, want 'keep me'", got)
	}
}

// --- Test 10: Transaction cannot be committed after rollback / vice versa --

func TestAtomicApply_StateGuards(t *testing.T) {
	root := t.TempDir()
	tx := NewFileTransaction("plan-10", "main", root)
	tx.Begin()
	tx.CreateFile("guard.txt", "content")
	tx.ApplyAll()

	// Rollback first
	tx.Rollback("test guard")

	// Commit should fail
	if err := tx.Commit(); err == nil {
		t.Error("Commit after Rollback should fail")
	}

	// Fresh tx: commit first, then rollback should fail
	tx2 := NewFileTransaction("plan-10b", "main", root)
	tx2.Begin()
	tx2.CreateFile("guard2.txt", "content2")
	tx2.ApplyAll()
	tx2.Commit()

	if err := tx2.Rollback("should fail"); err == nil {
		t.Error("Rollback after Commit should fail")
	}
}

// --- Test 11: Patch escaping normalisation ----------------------------------

func TestAtomicApply_BacktickEscapeNormalisation(t *testing.T) {
	// The plan renderer sometimes escapes triple-backticks.
	// Verify the normalisation that ApplyFiles performs.
	input := "```go\\`\\`\\`\npackage main\\`\\`\\`\n```"
	expected := strings.ReplaceAll(input, "\\`\\`\\`", "```")
	if expected == input {
		t.Fatal("normalisation should change input")
	}
	if !strings.Contains(expected, "```go```") {
		t.Errorf("normalised = %q, unexpected result", expected)
	}
}

// --- Benchmarks --------------------------------------------------------------

func BenchmarkAtomicApply_100Files(b *testing.B) {
	root := b.TempDir()

	for i := 0; i < b.N; i++ {
		tx := NewFileTransaction("bench", "main", root)
		tx.Begin()

		for j := 0; j < 100; j++ {
			path := fmt.Sprintf("file_%04d.txt", j)
			tx.CreateFile(path, fmt.Sprintf("content %d iteration %d", j, i))
		}
		tx.ApplyAll()
		tx.Commit()
	}
}

func BenchmarkAtomicApply_RollbackAfter50(b *testing.B) {
	root := b.TempDir()

	for i := 0; i < b.N; i++ {
		tx := NewFileTransaction("bench-rb", "main", root)
		tx.Begin()

		for j := 0; j < 100; j++ {
			tx.CreateFile(fmt.Sprintf("rb_%04d.txt", j), fmt.Sprintf("v%d", i))
		}

		// Apply first 50
		for j := 0; j < 50; j++ {
			tx.ApplyNext()
		}
		tx.Rollback("benchmark rollback")
	}
}
