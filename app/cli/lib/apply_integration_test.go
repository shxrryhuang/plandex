package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"plandex-cli/fs"
	"plandex-cli/types"
	"testing"
)

// Integration tests for transactional patch application
// These tests require a real project directory structure

// TestFullApplyFlow_WithTransaction tests the complete apply flow with transactions
func TestFullApplyFlow_WithTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary project directory
	tmpDir := t.TempDir()
	originalRoot := fs.ProjectRoot
	fs.ProjectRoot = tmpDir
	defer func() {
		fs.ProjectRoot = originalRoot
	}()

	// Create initial project structure
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	toApply := map[string]string{
		"test.txt":     "modified",
		"new_file.txt": "new content",
	}
	toRemove := map[string]bool{}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	updatedFiles, err := ApplyFilesWithTransaction("test-plan", "main", toApply, toRemove, projectPaths)

	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	if len(updatedFiles) != 2 {
		t.Errorf("Expected 2 updated files, got %d", len(updatedFiles))
	}

	// Verify changes
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test.txt: %v", err)
	}
	if string(content) != "modified" {
		t.Errorf("test.txt: expected 'modified', got '%s'", string(content))
	}

	newFilePath := filepath.Join(tmpDir, "new_file.txt")
	content, err = os.ReadFile(newFilePath)
	if err != nil {
		t.Fatalf("Failed to read new_file.txt: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("new_file.txt: expected 'new content', got '%s'", string(content))
	}

	// Verify WAL was cleaned up
	walDir := filepath.Join(tmpDir, ".plandex", "wal")
	entries, err := os.ReadDir(walDir)
	if err == nil && len(entries) > 0 {
		t.Errorf("WAL directory should be empty after commit, found %d entries", len(entries))
	}

	// Verify snapshots were cleaned up
	snapshotDir := filepath.Join(tmpDir, ".plandex", "snapshots")
	entries, err = os.ReadDir(snapshotDir)
	if err == nil && len(entries) > 0 {
		t.Errorf("Snapshot directory should be empty after commit, found %d entries", len(entries))
	}
}

// TestTransactionRollback_OnFileError tests automatic rollback on file write error
func TestTransactionRollback_OnFileError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	originalRoot := fs.ProjectRoot
	fs.ProjectRoot = tmpDir
	defer func() {
		fs.ProjectRoot = originalRoot
	}()

	// Create initial file
	testFile := filepath.Join(tmpDir, "test.txt")
	initialContent := "initial content"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a directory structure where one file will fail to write
	// Make a subdirectory read-only
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0755); err != nil {
		t.Fatalf("Failed to create readonly dir: %v", err)
	}
	// Make it read-only after creation
	if err := os.Chmod(readOnlyDir, 0444); err != nil {
		t.Fatalf("Failed to chmod readonly dir: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0755) // Restore for cleanup

	toApply := map[string]string{
		"test.txt":          "modified",
		"readonly/fail.txt": "this should fail",
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	_, err := ApplyFilesWithTransaction("test-plan", "main", toApply, map[string]bool{}, projectPaths)

	// Should get an error
	if err == nil {
		t.Fatal("Expected an error due to read-only directory, got nil")
	}

	// Verify rollback: test.txt should still have initial content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test.txt: %v", err)
	}
	if string(content) != initialContent {
		t.Errorf("test.txt should have been rolled back to '%s', got '%s'", initialContent, string(content))
	}

	// Verify the failed file was not created
	failPath := filepath.Join(readOnlyDir, "fail.txt")
	if _, err := os.Stat(failPath); !os.IsNotExist(err) {
		t.Error("fail.txt should not exist after rollback")
	}
}

// TestTransactionRollback_RestoresContent tests that rollback perfectly restores original content
func TestTransactionRollback_RestoresContent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	originalRoot := fs.ProjectRoot
	fs.ProjectRoot = tmpDir
	defer func() {
		fs.ProjectRoot = originalRoot
	}()

	// Create multiple files with specific content
	files := map[string]string{
		"file1.txt": "original content 1",
		"file2.txt": "original content 2",
		"file3.txt": "original content 3",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", path, err)
		}
	}

	// Attempt to modify all files, but one will fail
	readOnlyDir := filepath.Join(tmpDir, "protected")
	if err := os.Mkdir(readOnlyDir, 0444); err != nil {
		t.Fatalf("Failed to create protected dir: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0755)

	toApply := map[string]string{
		"file1.txt":        "modified 1",
		"file2.txt":        "modified 2",
		"file3.txt":        "modified 3",
		"protected/new.txt": "will fail",
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	_, err := ApplyFilesWithTransaction("test-plan", "main", toApply, map[string]bool{}, projectPaths)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Verify ALL files were rolled back to original content
	for path, expectedContent := range files {
		fullPath := filepath.Join(tmpDir, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("Failed to read %s: %v", path, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("%s: expected '%s', got '%s'", path, expectedContent, string(content))
		}
	}
}

// TestMultiplePatchApplications tests sequential transaction applications
func TestMultiplePatchApplications(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	originalRoot := fs.ProjectRoot
	fs.ProjectRoot = tmpDir
	defer func() {
		fs.ProjectRoot = originalRoot
	}()

	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	// First transaction: create files
	toApply1 := map[string]string{
		"file1.txt": "version 1",
		"file2.txt": "version 1",
	}
	_, err := ApplyFilesWithTransaction("plan-1", "main", toApply1, map[string]bool{}, projectPaths)
	if err != nil {
		t.Fatalf("First transaction failed: %v", err)
	}

	// Second transaction: modify files
	toApply2 := map[string]string{
		"file1.txt": "version 2",
		"file2.txt": "version 2",
	}
	_, err = ApplyFilesWithTransaction("plan-2", "main", toApply2, map[string]bool{}, projectPaths)
	if err != nil {
		t.Fatalf("Second transaction failed: %v", err)
	}

	// Third transaction: delete one, modify one
	toApply3 := map[string]string{
		"file1.txt": "version 3",
	}
	toRemove3 := map[string]bool{
		"file2.txt": true,
	}
	_, err = ApplyFilesWithTransaction("plan-3", "main", toApply3, toRemove3, projectPaths)
	if err != nil {
		t.Fatalf("Third transaction failed: %v", err)
	}

	// Verify final state
	file1Path := filepath.Join(tmpDir, "file1.txt")
	content, err := os.ReadFile(file1Path)
	if err != nil {
		t.Fatalf("Failed to read file1.txt: %v", err)
	}
	if string(content) != "version 3" {
		t.Errorf("file1.txt: expected 'version 3', got '%s'", string(content))
	}

	file2Path := filepath.Join(tmpDir, "file2.txt")
	if _, err := os.Stat(file2Path); !os.IsNotExist(err) {
		t.Error("file2.txt should have been deleted")
	}
}

// TestTransactionWithNestedDirectories tests creating files in nested directories
func TestTransactionWithNestedDirectories(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	originalRoot := fs.ProjectRoot
	fs.ProjectRoot = tmpDir
	defer func() {
		fs.ProjectRoot = originalRoot
	}()

	toApply := map[string]string{
		"a/b/c/deep.txt":   "deep file",
		"x/y/another.txt":  "another deep file",
		"single.txt":       "single file",
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	_, err := ApplyFilesWithTransaction("test-plan", "main", toApply, map[string]bool{}, projectPaths)

	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	// Verify all files were created
	for path, expectedContent := range toApply {
		fullPath := filepath.Join(tmpDir, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("Failed to read %s: %v", path, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("%s: expected '%s', got '%s'", path, expectedContent, string(content))
		}
	}
}

// TestTransactionCleanup tests that WAL and snapshots are cleaned up
func TestTransactionCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	originalRoot := fs.ProjectRoot
	fs.ProjectRoot = tmpDir
	defer func() {
		fs.ProjectRoot = originalRoot
	}()

	toApply := map[string]string{
		"test.txt": "content",
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	_, err := ApplyFilesWithTransaction("test-plan", "main", toApply, map[string]bool{}, projectPaths)

	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	// Check that .plandex directory exists (it's created during transaction)
	plandexDir := filepath.Join(tmpDir, ".plandex")
	if _, err := os.Stat(plandexDir); err != nil {
		// It's ok if it doesn't exist - might be cleaned up
		return
	}

	// If it exists, WAL and snapshots should be cleaned up
	walDir := filepath.Join(plandexDir, "wal")
	if entries, err := os.ReadDir(walDir); err == nil {
		if len(entries) > 0 {
			t.Errorf("WAL directory should be empty after successful commit, found %d entries", len(entries))
			for _, entry := range entries {
				t.Logf("  - %s", entry.Name())
			}
		}
	}

	snapshotDir := filepath.Join(plandexDir, "snapshots")
	if entries, err := os.ReadDir(snapshotDir); err == nil {
		if len(entries) > 0 {
			t.Errorf("Snapshot directory should be empty after successful commit, found %d entries", len(entries))
			for _, entry := range entries {
				t.Logf("  - %s", entry.Name())
			}
		}
	}
}

// Helper function to create test scenarios
func setupTestScenario(t *testing.T, files map[string]string) string {
	tmpDir := t.TempDir()
	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}
	return tmpDir
}

// TestTransactionPerformance_SmallPatch benchmarks small patch performance
func TestTransactionPerformance_SmallPatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tmpDir := t.TempDir()
	originalRoot := fs.ProjectRoot
	fs.ProjectRoot = tmpDir
	defer func() {
		fs.ProjectRoot = originalRoot
	}()

	toApply := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
		"file3.txt": "content3",
	}
	projectPaths := &types.ProjectPaths{
		AllPaths: make(map[string]bool),
	}

	// Run multiple times to get average
	iterations := 10
	for i := 0; i < iterations; i++ {
		_, err := ApplyFilesWithTransaction(
			fmt.Sprintf("test-plan-%d", i),
			"main",
			toApply,
			map[string]bool{},
			projectPaths,
		)
		if err != nil {
			t.Fatalf("Transaction %d failed: %v", i, err)
		}
	}

	t.Logf("Successfully completed %d small patch transactions", iterations)
}
