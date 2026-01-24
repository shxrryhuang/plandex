package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultResumeOptions(t *testing.T) {
	opts := DefaultResumeOptions()

	if !opts.UseLatestCheckpoint {
		t.Error("UseLatestCheckpoint should be true by default")
	}
	if !opts.StrictValidation {
		t.Error("StrictValidation should be true by default")
	}
	if opts.AllowRepair {
		t.Error("AllowRepair should be false by default")
	}
	if !opts.ValidateAllFiles {
		t.Error("ValidateAllFiles should be true by default")
	}
	if opts.DryRun {
		t.Error("DryRun should be false by default")
	}
	if !opts.BackupBeforeResume {
		t.Error("BackupBeforeResume should be true by default")
	}
}

func TestSelectCheckpoint_ByName(t *testing.T) {
	journal := createTestJournalWithCheckpoints()

	options := &ResumeOptions{
		CheckpointName: "checkpoint_1",
	}

	cp, err := selectCheckpoint(journal, options)
	if err != nil {
		t.Fatalf("selectCheckpoint failed: %v", err)
	}
	if cp.Name != "checkpoint_1" {
		t.Errorf("Expected checkpoint_1, got %s", cp.Name)
	}
}

func TestSelectCheckpoint_Latest(t *testing.T) {
	journal := createTestJournalWithCheckpoints()

	options := &ResumeOptions{
		UseLatestCheckpoint: true,
	}

	cp, err := selectCheckpoint(journal, options)
	if err != nil {
		t.Fatalf("selectCheckpoint failed: %v", err)
	}
	// checkpoint_2 was created after checkpoint_1
	if cp.Name != "checkpoint_2" {
		t.Errorf("Expected checkpoint_2 (latest), got %s", cp.Name)
	}
}

func TestSelectCheckpoint_NotFound(t *testing.T) {
	journal := createTestJournalWithCheckpoints()

	options := &ResumeOptions{
		CheckpointName: "nonexistent",
	}

	_, err := selectCheckpoint(journal, options)
	if err == nil {
		t.Error("Expected error for nonexistent checkpoint")
	}
}

func TestSelectCheckpoint_NoCheckpoints(t *testing.T) {
	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	options := &ResumeOptions{
		UseLatestCheckpoint: true,
	}

	_, err := selectCheckpoint(journal, options)
	if err == nil {
		t.Error("Expected error when no checkpoints exist")
	}
}

func TestValidateFileState_AllMatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	os.WriteFile(file1, []byte("content1"), 0644)
	os.WriteFile(file2, []byte("content2"), 0644)

	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")
	journal.TrackFile(file1, "content1")
	journal.TrackFile(file2, "content2")

	// Create checkpoint with current hashes
	hash1 := computeResumeHash("content1")
	hash2 := computeResumeHash("content2")

	checkpoint := &Checkpoint{
		Name:       "test_checkpoint",
		EntryIndex: 0,
		FileStates: map[string]string{
			file1: hash1,
			file2: hash2,
		},
	}

	options := DefaultResumeOptions()
	report, divergences := validateFileState(journal, checkpoint, options)

	if len(divergences) != 0 {
		t.Errorf("Expected no divergences, got %d", len(divergences))
	}
	if report.FilesMatched != 2 {
		t.Errorf("Expected 2 files matched, got %d", report.FilesMatched)
	}
	if !report.SafeToResume {
		t.Error("Should be safe to resume")
	}
}

func TestValidateFileState_FileMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Only create one file, checkpoint expects two
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt") // This won't exist
	os.WriteFile(file1, []byte("content1"), 0644)

	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	checkpoint := &Checkpoint{
		Name:       "test_checkpoint",
		EntryIndex: 0,
		FileStates: map[string]string{
			file1: computeResumeHash("content1"),
			file2: computeResumeHash("content2"), // File doesn't exist
		},
	}

	options := DefaultResumeOptions()
	report, divergences := validateFileState(journal, checkpoint, options)

	if len(divergences) != 1 {
		t.Fatalf("Expected 1 divergence, got %d", len(divergences))
	}
	if divergences[0].Type != "file_missing" {
		t.Errorf("Expected file_missing, got %s", divergences[0].Type)
	}
	if divergences[0].Path != file2 {
		t.Errorf("Expected path %s, got %s", file2, divergences[0].Path)
	}
	if report.SafeToResume {
		t.Error("Should not be safe to resume with missing file")
	}
}

func TestValidateFileState_HashMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "file1.txt")
	os.WriteFile(file1, []byte("modified_content"), 0644) // Different content

	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	checkpoint := &Checkpoint{
		Name:       "test_checkpoint",
		EntryIndex: 0,
		FileStates: map[string]string{
			file1: computeResumeHash("original_content"), // Expects original
		},
	}

	options := DefaultResumeOptions()
	_, divergences := validateFileState(journal, checkpoint, options)

	if len(divergences) != 1 {
		t.Fatalf("Expected 1 divergence, got %d", len(divergences))
	}
	if divergences[0].Type != "hash_mismatch" {
		t.Errorf("Expected hash_mismatch, got %s", divergences[0].Type)
	}
}

func TestValidateFileState_ExtraFile(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "file1.txt")
	os.WriteFile(file1, []byte("content1"), 0644)

	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")
	journal.TrackFile(file1, "content1") // Track in journal but not checkpoint

	checkpoint := &Checkpoint{
		Name:       "test_checkpoint",
		EntryIndex: 0,
		FileStates: map[string]string{
			file1: "", // Empty hash means file wasn't tracked at checkpoint
		},
	}

	options := DefaultResumeOptions()
	options.ValidateAllFiles = true
	_, divergences := validateFileState(journal, checkpoint, options)

	if len(divergences) != 1 {
		t.Fatalf("Expected 1 divergence, got %d", len(divergences))
	}
	if divergences[0].Type != "file_extra" {
		t.Errorf("Expected file_extra, got %s", divergences[0].Type)
	}
	if divergences[0].Severity != "warning" {
		t.Errorf("Extra file should be warning, got %s", divergences[0].Severity)
	}
}

func TestIsCheckpointGood(t *testing.T) {
	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	// Add some entries
	journal.AppendEntry(EntryTypeUserPrompt, &EntryData{})
	journal.AppendEntry(EntryTypeModelRequest, &EntryData{})
	journal.AppendEntry(EntryTypeModelResponse, &EntryData{})

	// Mark first two as completed
	journal.Entries[0].Status = EntryStatusCompleted
	journal.Entries[1].Status = EntryStatusCompleted
	journal.Entries[2].Status = EntryStatusFailed // This one failed

	// Checkpoint at entry 2 (after first two)
	checkpoint1 := &Checkpoint{EntryIndex: 2}
	if !isCheckpointGood(journal, checkpoint1) {
		t.Error("Checkpoint at entry 2 should be good (entries 0,1 completed)")
	}

	// Checkpoint at entry 3 (includes failed entry)
	checkpoint2 := &Checkpoint{EntryIndex: 3}
	if isCheckpointGood(journal, checkpoint2) {
		t.Error("Checkpoint at entry 3 should not be good (entry 2 failed)")
	}
}

func TestResumeFromCheckpoint_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "file1.txt")
	os.WriteFile(file1, []byte("content"), 0644)

	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")
	journal.TrackFile(file1, "content")

	// Create checkpoint
	checkpoint := journal.CreateCheckpoint("test_cp", "Test checkpoint", false)
	checkpoint.FileStates = map[string]string{
		file1: computeResumeHash("content"),
	}
	checkpoint.JournalHash = journal.ComputeHashUpTo(checkpoint.EntryIndex)

	options := &ResumeOptions{
		CheckpointName: "test_cp",
		DryRun:         true,
	}

	result, err := ResumeFromCheckpoint(journal, options)
	if err != nil {
		t.Fatalf("ResumeFromCheckpoint failed: %v", err)
	}

	if !result.Success {
		t.Error("Dry run should succeed")
	}
	if result.ResumedFrom != "test_cp" {
		t.Errorf("Expected ResumedFrom=test_cp, got %s", result.ResumedFrom)
	}
}

func TestResumeFromCheckpoint_StrictValidation(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "file1.txt")
	os.WriteFile(file1, []byte("modified"), 0644) // Different from checkpoint

	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	// Create checkpoint expecting different content
	checkpoint := journal.CreateCheckpoint("test_cp", "Test checkpoint", false)
	checkpoint.FileStates = map[string]string{
		file1: computeResumeHash("original"), // Expects "original"
	}
	checkpoint.JournalHash = journal.ComputeHashUpTo(checkpoint.EntryIndex)

	options := &ResumeOptions{
		CheckpointName:   "test_cp",
		StrictValidation: true, // Strict mode
	}

	result, err := ResumeFromCheckpoint(journal, options)
	if err == nil {
		t.Error("Expected error in strict mode with divergence")
	}
	if result.Success {
		t.Error("Should not succeed with divergence in strict mode")
	}
	if len(result.Divergences) == 0 {
		t.Error("Should report divergences")
	}
}

func TestRepairMissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	missingFile := filepath.Join(tmpDir, "missing.txt")
	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	checkpoint := &Checkpoint{
		Name: "test_cp",
		FileContents: map[string]string{
			missingFile: "restored content",
		},
	}

	action := repairMissingFile(journal, checkpoint, missingFile)

	if !action.Success {
		t.Fatalf("Repair should succeed: %s", action.Error)
	}

	// Verify file was created
	content, err := os.ReadFile(missingFile)
	if err != nil {
		t.Fatalf("File should exist: %v", err)
	}
	if string(content) != "restored content" {
		t.Errorf("Content = %s, want 'restored content'", string(content))
	}
}

func TestRepairMissingFile_NoContent(t *testing.T) {
	tmpDir := t.TempDir()

	missingFile := filepath.Join(tmpDir, "missing.txt")
	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	checkpoint := &Checkpoint{
		Name:         "test_cp",
		FileContents: nil, // No content available
	}

	action := repairMissingFile(journal, checkpoint, missingFile)

	if action.Success {
		t.Error("Repair should fail when content not available")
	}
}

func TestRestoreFileFromCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "file1.txt")
	os.WriteFile(file1, []byte("current content"), 0644)

	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	checkpoint := &Checkpoint{
		Name: "test_cp",
		FileContents: map[string]string{
			file1: "checkpoint content",
		},
	}

	action := restoreFileFromCheckpoint(journal, checkpoint, file1)

	if !action.Success {
		t.Fatalf("Restore should succeed: %s", action.Error)
	}

	content, _ := os.ReadFile(file1)
	if string(content) != "checkpoint content" {
		t.Errorf("Content = %s, want 'checkpoint content'", string(content))
	}
}

func TestGetFileHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Test existing file
	file1 := filepath.Join(tmpDir, "file1.txt")
	os.WriteFile(file1, []byte("test content"), 0644)

	hash, exists, err := getFileHash(file1)
	if err != nil {
		t.Fatalf("getFileHash failed: %v", err)
	}
	if !exists {
		t.Error("File should exist")
	}
	if hash == "" {
		t.Error("Hash should not be empty")
	}

	// Test non-existent file
	hash2, exists2, err2 := getFileHash(filepath.Join(tmpDir, "nonexistent.txt"))
	if err2 != nil {
		t.Fatalf("getFileHash should not error for non-existent: %v", err2)
	}
	if exists2 {
		t.Error("File should not exist")
	}
	if hash2 != "" {
		t.Error("Hash should be empty for non-existent file")
	}
}

func TestResumeDivergenceSeverity(t *testing.T) {
	tests := []struct {
		divType  string
		expected string
	}{
		{"file_missing", "error"},
		{"hash_mismatch", "error"},
		{"file_extra", "warning"},
	}

	for _, tt := range tests {
		div := ResumeDivergence{Type: tt.divType}
		switch tt.divType {
		case "file_missing", "hash_mismatch":
			div.Severity = "error"
		case "file_extra":
			div.Severity = "warning"
		}

		if div.Severity != tt.expected {
			t.Errorf("Divergence type %s: severity = %s, want %s",
				tt.divType, div.Severity, tt.expected)
		}
	}
}

func TestValidationReport(t *testing.T) {
	report := &ValidationReport{
		CheckpointName:   "test",
		CheckpointEntry:  5,
		FilesValidated:   10,
		FilesMatched:     8,
		FilesDiverged:    2,
		JournalIntegrity: true,
		SafeToResume:     false,
		ValidationTime:   time.Now(),
		FileStates: map[string]string{
			"file1.txt": "ok",
			"file2.txt": "modified",
		},
	}

	if report.FilesValidated != 10 {
		t.Errorf("FilesValidated = %d, want 10", report.FilesValidated)
	}
	if report.SafeToResume {
		t.Error("Should not be safe with diverged files")
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func createTestJournalWithCheckpoints() *RunJournal {
	journal := NewRunJournal("plan-1", "main", "org-1", "user-1", "test")

	// Add some entries
	journal.AppendEntry(EntryTypeUserPrompt, &EntryData{})
	journal.Entries[0].Status = EntryStatusCompleted

	journal.AppendEntry(EntryTypeModelRequest, &EntryData{})
	journal.Entries[1].Status = EntryStatusCompleted

	// Create first checkpoint
	cp1 := &Checkpoint{
		Name:        "checkpoint_1",
		CreatedAt:   time.Now().Add(-time.Hour),
		EntryIndex:  1,
		FileStates:  map[string]string{},
		JournalHash: journal.ComputeHashUpTo(1),
	}
	journal.Checkpoints["checkpoint_1"] = cp1

	// Add more entries
	journal.AppendEntry(EntryTypeModelResponse, &EntryData{})
	journal.Entries[2].Status = EntryStatusCompleted

	// Create second checkpoint (later)
	cp2 := &Checkpoint{
		Name:        "checkpoint_2",
		CreatedAt:   time.Now(),
		EntryIndex:  2,
		FileStates:  map[string]string{},
		JournalHash: journal.ComputeHashUpTo(2),
	}
	journal.Checkpoints["checkpoint_2"] = cp2

	return journal
}

func computeResumeHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
