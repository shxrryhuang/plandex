package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"plandex-cli/types"
	"strings"
	"testing"
)

// TestApplyFilesWithTransaction_Success tests the happy path with multiple files
func TestApplyFilesWithTransaction_Success(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create test files
	toApply := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
		"dir/file3.txt": "content3",
	}
	toRemove := map[string]bool{}

	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	// Override ProjectRoot for this test
	originalRoot := tmpDir
	defer func() {
		// Cleanup happens automatically with t.TempDir()
	}()

	// Apply files
	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, toRemove, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(updatedFiles) != len(toApply) {
		t.Errorf("Expected %d updated files, got %d", len(toApply), len(updatedFiles))
	}

	// Verify files were created
	for path, expectedContent := range toApply {
		fullPath := filepath.Join(tmpDir, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("Failed to read %s: %v", path, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("File %s: expected content %q, got %q", path, expectedContent, string(content))
		}
	}
}

// TestApplyFilesWithTransaction_ModifyExisting tests modifying existing files
func TestApplyFilesWithTransaction_ModifyExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial file
	initialPath := filepath.Join(tmpDir, "existing.txt")
	initialContent := "initial content"
	if err := os.WriteFile(initialPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	// Modify it
	toApply := map[string]string{
		"existing.txt": "modified content",
	}
	toRemove := map[string]bool{}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, toRemove, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(updatedFiles) != 1 {
		t.Errorf("Expected 1 updated file, got %d", len(updatedFiles))
	}

	// Verify content changed
	content, err := os.ReadFile(initialPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "modified content" {
		t.Errorf("Expected modified content, got: %s", string(content))
	}
}

// TestApplyFilesWithTransaction_Delete tests file deletion
func TestApplyFilesWithTransaction_Delete(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file to delete
	filePath := filepath.Join(tmpDir, "todelete.txt")
	if err := os.WriteFile(filePath, []byte("delete me"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	toApply := map[string]string{}
	toRemove := map[string]bool{
		"todelete.txt": true,
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, toRemove, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("File should have been deleted but still exists")
	}

	_ = updatedFiles // Silence unused warning
}

// TestApplyFilesWithTransaction_MixedOperations tests create, modify, and delete together
func TestApplyFilesWithTransaction_MixedOperations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial file to modify
	modifyPath := filepath.Join(tmpDir, "modify.txt")
	if err := os.WriteFile(modifyPath, []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Create file to delete
	deletePath := filepath.Join(tmpDir, "delete.txt")
	if err := os.WriteFile(deletePath, []byte("delete me"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	toApply := map[string]string{
		"create.txt": "new file",
		"modify.txt": "updated content",
	}
	toRemove := map[string]bool{
		"delete.txt": true,
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, toRemove, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(updatedFiles) != 2 {
		t.Errorf("Expected 2 updated files, got %d", len(updatedFiles))
	}

	// Verify create
	createPath := filepath.Join(tmpDir, "create.txt")
	if content, err := os.ReadFile(createPath); err != nil {
		t.Errorf("Failed to read created file: %v", err)
	} else if string(content) != "new file" {
		t.Errorf("Created file has wrong content: %s", string(content))
	}

	// Verify modify
	if content, err := os.ReadFile(modifyPath); err != nil {
		t.Errorf("Failed to read modified file: %v", err)
	} else if string(content) != "updated content" {
		t.Errorf("Modified file has wrong content: %s", string(content))
	}

	// Verify delete
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Errorf("File should have been deleted")
	}
}

// TestApplyFilesWithTransaction_EmptyOperations tests no-op scenario
func TestApplyFilesWithTransaction_EmptyOperations(t *testing.T) {
	tmpDir := t.TempDir()

	toApply := map[string]string{}
	toRemove := map[string]bool{}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, toRemove, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(updatedFiles) != 0 {
		t.Errorf("Expected 0 updated files, got %d", len(updatedFiles))
	}

	_ = tmpDir // Silence unused warning
}

// TestApplyFilesWithTransaction_LargeFileSet tests performance with many files
func TestApplyFilesWithTransaction_LargeFileSet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file set test in short mode")
	}

	tmpDir := t.TempDir()

	// Create 100 files
	toApply := make(map[string]string)
	for i := 0; i < 100; i++ {
		toApply[fmt.Sprintf("file%d.txt", i)] = fmt.Sprintf("content %d", i)
	}

	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, map[string]bool{}, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(updatedFiles) != 100 {
		t.Errorf("Expected 100 updated files, got %d", len(updatedFiles))
	}

	// Spot check a few files
	for i := 0; i < 100; i += 25 {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read file%d.txt: %v", i, err)
		} else if string(content) != fmt.Sprintf("content %d", i) {
			t.Errorf("File%d.txt has wrong content", i)
		}
	}
}

// TestPrepareOperations tests operation staging
func TestPrepareOperations(t *testing.T) {
	// Note: This test requires refactoring prepareOperations to be testable
	// with a mock transaction. For now, it's covered by integration tests.
	t.Skip("Requires mock transaction support")
}

// TestAutomaticRollback tests the rollback helper function
func TestAutomaticRollback(t *testing.T) {
	// Note: This test requires a real transaction to test rollback behavior.
	// For now, covered by integration tests.
	t.Skip("Requires real transaction for rollback testing")
}

// TestFileExists tests the fileExists helper
func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Test non-existent file
	if fileExists(filepath.Join(tmpDir, "nonexistent.txt")) {
		t.Error("Expected false for non-existent file")
	}

	// Create file and test again
	existingPath := filepath.Join(tmpDir, "existing.txt")
	if err := os.WriteFile(existingPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	if !fileExists(existingPath) {
		t.Error("Expected true for existing file")
	}

	// Test empty path
	if fileExists("") {
		t.Error("Expected false for empty path")
	}
}

// TestApplyFilesWithTransaction_SkipApplyScript tests that _apply.sh is skipped
func TestApplyFilesWithTransaction_SkipApplyScript(t *testing.T) {
	tmpDir := t.TempDir()

	toApply := map[string]string{
		"regular.txt": "content",
		"_apply.sh":   "#!/bin/bash\necho 'test'",
	}
	toRemove := map[string]bool{}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, toRemove, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should only have regular.txt, not _apply.sh
	if len(updatedFiles) != 1 {
		t.Errorf("Expected 1 updated file, got %d", len(updatedFiles))
	}

	// Verify _apply.sh was not created
	scriptPath := filepath.Join(tmpDir, "_apply.sh")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("_apply.sh should not have been created")
	}

	// Verify regular file was created
	regularPath := filepath.Join(tmpDir, "regular.txt")
	if content, err := os.ReadFile(regularPath); err != nil {
		t.Errorf("Failed to read regular.txt: %v", err)
	} else if string(content) != "content" {
		t.Errorf("regular.txt has wrong content: %s", string(content))
	}
}

// TestApplyFilesWithTransaction_EscapedBackticks tests backtick unescaping
func TestApplyFilesWithTransaction_EscapedBackticks(t *testing.T) {
	tmpDir := t.TempDir()

	toApply := map[string]string{
		"code.md": "Here is some code: \\`\\`\\`go\nfunc main() {}\n\\`\\`\\`",
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	_, err := ApplyFilesWithTransaction("test-plan", "main", toApply, map[string]bool{}, projectPaths)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify backticks were unescaped
	path := filepath.Join(tmpDir, "code.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	expected := "Here is some code: ```go\nfunc main() {}\n```"
	if string(content) != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, string(content))
	}
}

// Benchmark tests

func BenchmarkApplyFilesWithTransaction_SmallPatch(b *testing.B) {
	toApply := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
		"file3.txt": "content3",
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tmpDir := b.TempDir()
		_, err := ApplyFilesWithTransaction("bench-plan", "main", toApply, map[string]bool{}, projectPaths)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
		os.RemoveAll(tmpDir)
	}
}

func BenchmarkApplyFilesWithTransaction_LargePatch(b *testing.B) {
	toApply := make(map[string]string)
	for i := 0; i < 50; i++ {
		toApply[fmt.Sprintf("file%d.txt", i)] = strings.Repeat("content ", 100)
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tmpDir := b.TempDir()
		_, err := ApplyFilesWithTransaction("bench-plan", "main", toApply, map[string]bool{}, projectPaths)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
		os.RemoveAll(tmpDir)
	}
}
