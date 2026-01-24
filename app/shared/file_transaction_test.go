package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileTransaction(t *testing.T) {
	tx := NewFileTransaction("plan-123", "main", "/tmp/test")

	if tx.Id == "" {
		t.Error("Transaction ID should not be empty")
	}
	if tx.PlanId != "plan-123" {
		t.Errorf("PlanId = %s, want plan-123", tx.PlanId)
	}
	if tx.Branch != "main" {
		t.Errorf("Branch = %s, want main", tx.Branch)
	}
	if tx.State != TxStateActive {
		t.Errorf("State = %s, want active", tx.State)
	}
	if len(tx.Operations) != 0 {
		t.Errorf("Operations should be empty, got %d", len(tx.Operations))
	}
}

func TestFileTransactionCreateFile(t *testing.T) {
	tmpDir := t.TempDir()
	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	err := tx.CreateFile("test.txt", "hello world")
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	if len(tx.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(tx.Operations))
	}

	op := tx.Operations[0]
	if op.Type != FileOpCreate {
		t.Errorf("Type = %s, want create", op.Type)
	}
	if op.Content != "hello world" {
		t.Errorf("Content = %s, want 'hello world'", op.Content)
	}
	if op.Status != FileOpPending {
		t.Errorf("Status = %s, want pending", op.Status)
	}
}

func TestFileTransactionApplyCreate(t *testing.T) {
	tmpDir := t.TempDir()
	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	filePath := "subdir/test.txt"
	content := "hello world"

	tx.CreateFile(filePath, content)

	op, err := tx.ApplyNext()
	if err != nil {
		t.Fatalf("ApplyNext failed: %v", err)
	}
	if op.Status != FileOpApplied {
		t.Errorf("Status = %s, want applied", op.Status)
	}

	// Verify file was created
	fullPath := filepath.Join(tmpDir, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}
	if string(data) != content {
		t.Errorf("File content = %s, want %s", string(data), content)
	}
}

func TestFileTransactionApplyModify(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing file
	existingPath := filepath.Join(tmpDir, "existing.txt")
	originalContent := "original content"
	os.WriteFile(existingPath, []byte(originalContent), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	newContent := "modified content"
	tx.ModifyFile("existing.txt", newContent)

	op, err := tx.ApplyNext()
	if err != nil {
		t.Fatalf("ApplyNext failed: %v", err)
	}
	if op.Status != FileOpApplied {
		t.Errorf("Status = %s, want applied", op.Status)
	}

	// Verify file was modified
	data, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}
	if string(data) != newContent {
		t.Errorf("File content = %s, want %s", string(data), newContent)
	}

	// Verify snapshot captured original
	snapshot := tx.Snapshots[existingPath]
	if snapshot == nil {
		t.Fatal("Snapshot should exist")
	}
	if snapshot.Content != originalContent {
		t.Errorf("Snapshot content = %s, want %s", snapshot.Content, originalContent)
	}
}

func TestFileTransactionApplyDelete(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file to delete
	filePath := filepath.Join(tmpDir, "to_delete.txt")
	os.WriteFile(filePath, []byte("delete me"), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.DeleteFile("to_delete.txt")

	op, err := tx.ApplyNext()
	if err != nil {
		t.Fatalf("ApplyNext failed: %v", err)
	}
	if op.Status != FileOpApplied {
		t.Errorf("Status = %s, want applied", op.Status)
	}

	// Verify file was deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("File should have been deleted")
	}
}

func TestFileTransactionRollback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing file
	existingPath := filepath.Join(tmpDir, "existing.txt")
	originalContent := "original content"
	os.WriteFile(existingPath, []byte(originalContent), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	// Modify the file
	tx.ModifyFile("existing.txt", "modified content")
	tx.ApplyNext()

	// Create a new file
	tx.CreateFile("new_file.txt", "new content")
	tx.ApplyNext()

	// Verify changes were applied
	newFilePath := filepath.Join(tmpDir, "new_file.txt")
	if _, err := os.Stat(newFilePath); err != nil {
		t.Fatal("New file should exist before rollback")
	}

	// Rollback
	err := tx.Rollback("test rollback")
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify state
	if tx.State != TxStateRolledBack {
		t.Errorf("State = %s, want rolled_back", tx.State)
	}

	// Verify original file restored
	data, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}
	if string(data) != originalContent {
		t.Errorf("Restored content = %s, want %s", string(data), originalContent)
	}

	// Verify new file was removed (it didn't exist before)
	if _, err := os.Stat(newFilePath); !os.IsNotExist(err) {
		t.Error("New file should have been removed during rollback")
	}
}

func TestFileTransactionRollbackOnProviderFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing file
	existingPath := filepath.Join(tmpDir, "existing.txt")
	originalContent := "original"
	os.WriteFile(existingPath, []byte(originalContent), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.ModifyFile("existing.txt", "modified")
	tx.ApplyNext()

	// Simulate provider failure
	failure := &ProviderFailure{
		Type:      FailureTypeRateLimit,
		Category:  FailureCategoryRetryable,
		HTTPCode:  429,
		Message:   "Rate limit exceeded",
		Retryable: true,
	}

	err := tx.RollbackOnProviderFailure(failure)
	if err != nil {
		t.Fatalf("RollbackOnProviderFailure failed: %v", err)
	}

	// Verify rollback happened
	if tx.State != TxStateRolledBack {
		t.Errorf("State = %s, want rolled_back", tx.State)
	}

	// Verify provider error recorded
	if tx.ProviderError == nil {
		t.Error("ProviderError should be set")
	}
	if tx.ProviderError.Type != FailureTypeRateLimit {
		t.Errorf("ProviderError.Type = %s, want rate_limit", tx.ProviderError.Type)
	}

	// Verify file restored
	data, _ := os.ReadFile(existingPath)
	if string(data) != originalContent {
		t.Errorf("Content = %s, want %s", string(data), originalContent)
	}
}

func TestFileTransactionCommit(t *testing.T) {
	tmpDir := t.TempDir()
	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.CreateFile("test.txt", "content")
	tx.ApplyAll()

	err := tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if tx.State != TxStateCommitted {
		t.Errorf("State = %s, want committed", tx.State)
	}
}

func TestFileTransactionCommitFailsWithPendingOps(t *testing.T) {
	tmpDir := t.TempDir()
	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.CreateFile("test.txt", "content")
	// Don't apply - try to commit with pending operation

	err := tx.Commit()
	if err == nil {
		t.Error("Commit should fail with pending operations")
	}
}

func TestFileTransactionCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing file
	existingPath := filepath.Join(tmpDir, "existing.txt")
	os.WriteFile(existingPath, []byte("original"), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	// First modification
	tx.ModifyFile("existing.txt", "modified1")
	tx.ApplyNext()

	// Create checkpoint
	err := tx.CreateCheckpoint("checkpoint1", "After first modification")
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}

	// Second modification
	tx.ModifyFile("existing.txt", "modified2")
	tx.ApplyNext()

	// Verify current state
	data, _ := os.ReadFile(existingPath)
	if string(data) != "modified2" {
		t.Errorf("Content = %s, want modified2", string(data))
	}

	// Rollback to checkpoint
	err = tx.RollbackToCheckpoint("checkpoint1")
	if err != nil {
		t.Fatalf("RollbackToCheckpoint failed: %v", err)
	}

	// Verify state at checkpoint (first modification)
	data, _ = os.ReadFile(existingPath)
	if string(data) != "modified1" {
		t.Errorf("Content after rollback = %s, want modified1", string(data))
	}
}

func TestFileTransactionRename(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	os.WriteFile(srcPath, []byte("content"), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.RenameFile("source.txt", "subdir/dest.txt")
	tx.ApplyAll()

	// Verify source is gone
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("Source file should not exist")
	}

	// Verify destination exists
	destPath := filepath.Join(tmpDir, "subdir/dest.txt")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read destination: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("Content = %s, want content", string(data))
	}
}

func TestFileTransactionRenameRollback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := "original content"
	os.WriteFile(srcPath, []byte(content), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.RenameFile("source.txt", "dest.txt")
	tx.ApplyAll()

	// Rollback
	tx.Rollback("test")

	// Verify source is back
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("Source should exist after rollback: %v", err)
	}
	if string(data) != content {
		t.Errorf("Content = %s, want %s", string(data), content)
	}

	// Verify destination is gone
	destPath := filepath.Join(tmpDir, "dest.txt")
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Error("Destination should not exist after rollback")
	}
}

func TestFileTransactionMultipleOperations(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup initial files
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	// Multiple operations
	tx.CreateFile("new.txt", "new content")
	tx.ModifyFile("file1.txt", "modified1")
	tx.DeleteFile("file2.txt")
	tx.RenameFile("new.txt", "renamed.txt")

	// Apply all
	err := tx.ApplyAll()
	if err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}

	// Verify all operations applied
	for _, op := range tx.Operations {
		if op.Status != FileOpApplied {
			t.Errorf("Operation %d status = %s, want applied", op.Seq, op.Status)
		}
	}
}

func TestFileTransactionOperationStatuses(t *testing.T) {
	tests := []struct {
		status   FileOpStatus
		expected string
	}{
		{FileOpPending, "pending"},
		{FileOpApplied, "applied"},
		{FileOpRolledBack, "rolled_back"},
		{FileOpFailed, "failed"},
		{FileOpSkipped, "skipped"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("FileOpStatus %v = %s, want %s", tt.status, string(tt.status), tt.expected)
		}
	}
}

func TestFileTransactionStates(t *testing.T) {
	tests := []struct {
		state    TransactionState
		expected string
	}{
		{TxStateActive, "active"},
		{TxStatePreparing, "preparing"},
		{TxStateCommitted, "committed"},
		{TxStateRolledBack, "rolled_back"},
		{TxStateFailed, "failed"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.expected {
			t.Errorf("TransactionState %v = %s, want %s", tt.state, string(tt.state), tt.expected)
		}
	}
}

func TestFileOperationTypes(t *testing.T) {
	tests := []struct {
		opType   FileOperationType
		expected string
	}{
		{FileOpCreate, "create"},
		{FileOpModify, "modify"},
		{FileOpDelete, "delete"},
		{FileOpRename, "rename"},
	}

	for _, tt := range tests {
		if string(tt.opType) != tt.expected {
			t.Errorf("FileOperationType %v = %s, want %s", tt.opType, string(tt.opType), tt.expected)
		}
	}
}

func TestHashString(t *testing.T) {
	hash1 := hashString("hello")
	hash2 := hashString("hello")
	hash3 := hashString("world")

	if hash1 != hash2 {
		t.Error("Same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("Different input should produce different hash")
	}
	if len(hash1) != 64 {
		t.Errorf("Hash length = %d, want 64 (SHA256 hex)", len(hash1))
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"a\nb\n", []string{"a", "b"}}, // Trailing newline doesn't create empty line
		{"", []string{}},               // Empty string returns empty slice
	}

	for _, tt := range tests {
		result := splitLines(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitLines(%q) returned %d lines, want %d", tt.input, len(result), len(tt.expected))
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestFileTransactionCannotCommitAfterRollback(t *testing.T) {
	tmpDir := t.TempDir()
	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.CreateFile("test.txt", "content")
	tx.ApplyAll()
	tx.Rollback("test")

	err := tx.Commit()
	if err == nil {
		t.Error("Should not be able to commit after rollback")
	}
}

func TestFileTransactionCannotRollbackAfterCommit(t *testing.T) {
	tmpDir := t.TempDir()
	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	tx.CreateFile("test.txt", "content")
	tx.ApplyAll()
	tx.Commit()

	err := tx.Rollback("test")
	if err == nil {
		t.Error("Should not be able to rollback after commit")
	}
}

func TestFileTransactionSnapshotCapturedOnce(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing file
	existingPath := filepath.Join(tmpDir, "existing.txt")
	os.WriteFile(existingPath, []byte("original"), 0644)

	tx := NewFileTransaction("plan-123", "main", tmpDir)
	tx.Begin()

	// Multiple modifications to same file
	tx.ModifyFile("existing.txt", "modified1")
	tx.ModifyFile("existing.txt", "modified2")
	tx.ModifyFile("existing.txt", "modified3")

	// Should only have one snapshot
	if len(tx.Snapshots) != 1 {
		t.Errorf("Should have 1 snapshot, got %d", len(tx.Snapshots))
	}

	// Snapshot should contain original content
	snapshot := tx.Snapshots[existingPath]
	if snapshot.Content != "original" {
		t.Errorf("Snapshot content = %s, want original", snapshot.Content)
	}
}
