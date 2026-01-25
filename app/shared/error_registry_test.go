package shared

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewErrorRegistry(t *testing.T) {
	registry := NewErrorRegistry("", 50, "test-session")

	if registry == nil {
		t.Fatal("Registry should not be nil")
	}
	if registry.maxErrors != 50 {
		t.Errorf("maxErrors = %d, want 50", registry.maxErrors)
	}
	if registry.sessionId != "test-session" {
		t.Errorf("sessionId = %s, want test-session", registry.sessionId)
	}
}

func TestNewErrorRegistry_DefaultMaxErrors(t *testing.T) {
	registry := NewErrorRegistry("", 0, "test")

	if registry.maxErrors != DefaultMaxErrors {
		t.Errorf("maxErrors = %d, want %d", registry.maxErrors, DefaultMaxErrors)
	}
}

func TestErrorRegistry_Store(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	report := NewErrorReport(
		ErrorCategoryProvider,
		"rate_limit",
		"RATE_LIMIT",
		"Too many requests",
	)

	id := registry.Store(report)

	if id == "" {
		t.Error("Store should return non-empty ID")
	}
	if registry.Count() != 1 {
		t.Errorf("Count() = %d, want 1", registry.Count())
	}
}

func TestErrorRegistry_Store_RingBuffer(t *testing.T) {
	registry := NewErrorRegistry("", 3, "test-session")

	// Store 5 errors (exceeds capacity of 3)
	for i := 0; i < 5; i++ {
		report := NewErrorReport(
			ErrorCategoryProvider,
			"test",
			"TEST",
			"Test error",
		)
		registry.Store(report)
	}

	// Should only have 3 errors (ring buffer)
	if registry.Count() != 3 {
		t.Errorf("Count() = %d, want 3 (ring buffer limit)", registry.Count())
	}
}

func TestErrorRegistry_Get(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	report := NewErrorReport(
		ErrorCategoryProvider,
		"rate_limit",
		"RATE_LIMIT",
		"Test error",
	)

	id := registry.Store(report)

	// Get existing
	stored := registry.Get(id)
	if stored == nil {
		t.Fatal("Get should return stored error")
	}
	if stored.Id != id {
		t.Errorf("Id = %s, want %s", stored.Id, id)
	}
	if stored.SessionId != "test-session" {
		t.Errorf("SessionId = %s, want test-session", stored.SessionId)
	}
	if stored.Resolved {
		t.Error("New error should not be resolved")
	}

	// Get non-existing
	notFound := registry.Get("non-existing")
	if notFound != nil {
		t.Error("Get should return nil for non-existing ID")
	}
}

func TestErrorRegistry_List(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	// Store errors with different categories
	report1 := NewErrorReport(ErrorCategoryProvider, "rate_limit", "RL", "Error 1")
	report2 := NewErrorReport(ErrorCategoryFileSystem, "permission", "PERM", "Error 2")
	report3 := NewErrorReport(ErrorCategoryProvider, "quota", "QE", "Error 3")

	registry.Store(report1)
	time.Sleep(time.Millisecond) // Ensure different timestamps
	registry.Store(report2)
	time.Sleep(time.Millisecond)
	registry.Store(report3)

	// List all
	all := registry.List(ErrorFilter{})
	if len(all) != 3 {
		t.Errorf("List all = %d, want 3", len(all))
	}

	// List with limit
	limited := registry.List(ErrorFilter{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("List with limit = %d, want 2", len(limited))
	}

	// List by category
	providerErrors := registry.List(ErrorFilter{Category: ErrorCategoryProvider})
	if len(providerErrors) != 2 {
		t.Errorf("List provider errors = %d, want 2", len(providerErrors))
	}
}

func TestErrorRegistry_List_TimeFilter(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	// Store an error
	report := NewErrorReport(ErrorCategoryProvider, "test", "TEST", "Test")
	registry.Store(report)

	// Filter by time
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	// Should find nothing after future
	results := registry.List(ErrorFilter{Since: &future})
	if len(results) != 0 {
		t.Errorf("List since future = %d, want 0", len(results))
	}

	// Should find error after past
	results = registry.List(ErrorFilter{Since: &past})
	if len(results) != 1 {
		t.Errorf("List since past = %d, want 1", len(results))
	}
}

func TestErrorRegistry_List_ResolvedFilter(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	report1 := NewErrorReport(ErrorCategoryProvider, "test1", "T1", "Test 1")
	report2 := NewErrorReport(ErrorCategoryProvider, "test2", "T2", "Test 2")

	id1 := registry.Store(report1)
	registry.Store(report2)

	// Mark first as resolved
	registry.MarkResolved(id1, "auto-retry")

	// Filter resolved
	resolved := true
	resolvedResults := registry.List(ErrorFilter{Resolved: &resolved})
	if len(resolvedResults) != 1 {
		t.Errorf("List resolved = %d, want 1", len(resolvedResults))
	}

	// Filter unresolved
	unresolved := false
	unresolvedResults := registry.List(ErrorFilter{Resolved: &unresolved})
	if len(unresolvedResults) != 1 {
		t.Errorf("List unresolved = %d, want 1", len(unresolvedResults))
	}
}

func TestErrorRegistry_MarkResolved(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	report := NewErrorReport(ErrorCategoryProvider, "test", "TEST", "Test")
	id := registry.Store(report)

	// Mark resolved
	success := registry.MarkResolved(id, "manual-fix")
	if !success {
		t.Error("MarkResolved should return true")
	}

	stored := registry.Get(id)
	if !stored.Resolved {
		t.Error("Error should be resolved")
	}
	if stored.ResolvedBy != "manual-fix" {
		t.Errorf("ResolvedBy = %s, want manual-fix", stored.ResolvedBy)
	}
	if stored.ResolvedAt == nil {
		t.Error("ResolvedAt should be set")
	}

	// Mark non-existing
	success = registry.MarkResolved("non-existing", "test")
	if success {
		t.Error("MarkResolved should return false for non-existing")
	}
}

func TestErrorRegistry_IncrementRetry(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	report := NewErrorReport(ErrorCategoryProvider, "test", "TEST", "Test")
	id := registry.Store(report)

	count := registry.IncrementRetry(id)
	if count != 1 {
		t.Errorf("IncrementRetry = %d, want 1", count)
	}

	count = registry.IncrementRetry(id)
	if count != 2 {
		t.Errorf("IncrementRetry = %d, want 2", count)
	}

	stored := registry.Get(id)
	if stored.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", stored.RetryCount)
	}

	// Non-existing
	count = registry.IncrementRetry("non-existing")
	if count != 0 {
		t.Errorf("IncrementRetry non-existing = %d, want 0", count)
	}
}

func TestErrorRegistry_Clear(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	registry.Store(NewErrorReport(ErrorCategoryProvider, "test", "T", "Test"))
	registry.Store(NewErrorReport(ErrorCategoryProvider, "test", "T", "Test"))

	if registry.Count() != 2 {
		t.Fatalf("Count before clear = %d, want 2", registry.Count())
	}

	registry.Clear()

	if registry.Count() != 0 {
		t.Errorf("Count after clear = %d, want 0", registry.Count())
	}
}

func TestErrorRegistry_UnresolvedCount(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	id1 := registry.Store(NewErrorReport(ErrorCategoryProvider, "test", "T", "Test 1"))
	registry.Store(NewErrorReport(ErrorCategoryProvider, "test", "T", "Test 2"))
	registry.Store(NewErrorReport(ErrorCategoryProvider, "test", "T", "Test 3"))

	if registry.UnresolvedCount() != 3 {
		t.Errorf("UnresolvedCount = %d, want 3", registry.UnresolvedCount())
	}

	registry.MarkResolved(id1, "test")

	if registry.UnresolvedCount() != 2 {
		t.Errorf("UnresolvedCount after resolve = %d, want 2", registry.UnresolvedCount())
	}
}

func TestErrorRegistry_Stats(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	// Store mixed errors
	id1 := registry.Store(NewErrorReport(ErrorCategoryProvider, "rate_limit", "RL", "Test 1"))
	registry.Store(NewErrorReport(ErrorCategoryProvider, "quota", "QE", "Test 2"))
	registry.Store(NewErrorReport(ErrorCategoryFileSystem, "permission", "PERM", "Test 3"))

	registry.MarkResolved(id1, "auto")

	stats := registry.Stats()

	if stats.TotalErrors != 3 {
		t.Errorf("TotalErrors = %d, want 3", stats.TotalErrors)
	}
	if stats.ResolvedErrors != 1 {
		t.Errorf("ResolvedErrors = %d, want 1", stats.ResolvedErrors)
	}
	if stats.UnresolvedErrors != 2 {
		t.Errorf("UnresolvedErrors = %d, want 2", stats.UnresolvedErrors)
	}
	if stats.ErrorsByCategory["provider"] != 2 {
		t.Errorf("ErrorsByCategory[provider] = %d, want 2", stats.ErrorsByCategory["provider"])
	}
	if stats.ErrorsByCategory["file_system"] != 1 {
		t.Errorf("ErrorsByCategory[file_system] = %d, want 1", stats.ErrorsByCategory["file_system"])
	}
	if stats.ErrorsByType["rate_limit"] != 1 {
		t.Errorf("ErrorsByType[rate_limit] = %d, want 1", stats.ErrorsByType["rate_limit"])
	}
}

func TestErrorRegistry_Persistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "error-registry-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and populate registry
	registry1 := NewErrorRegistry(tmpDir, 10, "session-1")

	report := NewErrorReport(ErrorCategoryProvider, "test", "TEST", "Test error")
	id := registry1.Store(report)

	// Persist
	if err := registry1.Persist(); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Verify file exists
	persistPath := filepath.Join(tmpDir, DefaultErrorsFileName)
	if _, err := os.Stat(persistPath); os.IsNotExist(err) {
		t.Error("Persist file should exist")
	}

	// Load into new registry
	registry2 := NewErrorRegistry(tmpDir, 10, "session-2")
	if err := registry2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if registry2.Count() != 1 {
		t.Errorf("Loaded registry count = %d, want 1", registry2.Count())
	}

	loaded := registry2.Get(id)
	if loaded == nil {
		t.Error("Should find loaded error by ID")
	}
	if loaded.RootCause.Message != "Test error" {
		t.Errorf("Loaded message = %s, want 'Test error'", loaded.RootCause.Message)
	}
}

func TestErrorRegistry_Load_NoFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "error-registry-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry := NewErrorRegistry(tmpDir, 10, "test")

	// Load should not error when file doesn't exist
	if err := registry.Load(); err != nil {
		t.Errorf("Load should not error for missing file: %v", err)
	}
}

func TestErrorRegistry_Export_JSON(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	registry.Store(NewErrorReport(ErrorCategoryProvider, "test", "TEST", "Test error"))

	data, err := registry.Export(ExportFormatJSON, SanitizeLevelStandard)
	if err != nil {
		t.Fatalf("Export JSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Export should produce non-empty data")
	}

	// Should be valid JSON (starts with [)
	if data[0] != '[' {
		t.Error("JSON export should be an array")
	}
}

func TestErrorRegistry_Export_Text(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	registry.Store(NewErrorReport(ErrorCategoryProvider, "test", "TEST", "Test error"))

	data, err := registry.Export(ExportFormatText, SanitizeLevelStandard)
	if err != nil {
		t.Fatalf("Export text failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Export should produce non-empty data")
	}

	// Should contain header
	text := string(data)
	if !contains(text, "ERROR EXPORT") {
		t.Error("Text export should contain header")
	}
}

func TestErrorRegistry_Export_InvalidFormat(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	_, err := registry.Export("invalid", SanitizeLevelStandard)
	if err == nil {
		t.Error("Export should fail for invalid format")
	}
}

func TestErrorRegistry_Store_NilReport(t *testing.T) {
	registry := NewErrorRegistry("", 10, "test-session")

	id := registry.Store(nil)
	if id != "" {
		t.Error("Store nil should return empty ID")
	}
	if registry.Count() != 0 {
		t.Error("Store nil should not add to registry")
	}
}

func TestGlobalRegistry(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "global-registry-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize global registry
	err = InitGlobalRegistry(tmpDir, "global-test")
	if err != nil {
		t.Fatalf("InitGlobalRegistry failed: %v", err)
	}

	if GlobalErrorRegistry == nil {
		t.Fatal("GlobalErrorRegistry should not be nil")
	}

	// Test convenience functions
	report := NewErrorReport(ErrorCategoryProvider, "test", "TEST", "Global test")
	id := StoreError(report)

	if id == "" {
		t.Error("StoreError should return ID")
	}

	stored := GetError(id)
	if stored == nil {
		t.Error("GetError should return stored error")
	}

	errors := ListErrors(ErrorFilter{})
	if len(errors) != 1 {
		t.Errorf("ListErrors = %d, want 1", len(errors))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
