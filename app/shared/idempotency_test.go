package shared

import (
	"testing"
	"time"
)

func TestIdempotencyManager_NewOperation(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	key := "test-key-1"
	data := map[string]string{"foo": "bar"}

	// First check should allow proceeding
	result := m.Check(key, data)
	if result.IsDuplicate {
		t.Error("First check should not be a duplicate")
	}
	if !result.ShouldProceed {
		t.Error("First check should allow proceeding")
	}
}

func TestIdempotencyManager_StartAndComplete(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	key := "test-key-2"
	data := map[string]string{"foo": "bar"}

	// Start the operation
	record := m.Start(key, data)
	if record == nil {
		t.Fatal("Start should return a record")
	}
	if record.Status != IdempotencyInProgress {
		t.Errorf("Status = %s, want in_progress", record.Status)
	}
	if record.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", record.AttemptNumber)
	}

	// Check should indicate in progress
	result := m.Check(key, data)
	if !result.IsDuplicate {
		t.Error("Should be a duplicate while in progress")
	}
	if result.ShouldProceed {
		t.Error("Should not proceed while in progress")
	}

	// Complete the operation
	m.Complete(key, true, "result", nil)

	// Check should indicate completed
	result = m.Check(key, data)
	if !result.IsDuplicate {
		t.Error("Should be a duplicate after completion")
	}
	if result.ShouldProceed {
		t.Error("Should not proceed after successful completion")
	}
}

func TestIdempotencyManager_FailedRetry(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	key := "test-key-3"
	data := map[string]string{"foo": "bar"}

	// Start and fail
	m.Start(key, data)
	m.Complete(key, false, nil, &mockError{msg: "test error"})

	// Check should allow retry
	result := m.Check(key, data)
	if !result.IsDuplicate {
		t.Error("Should be a duplicate")
	}
	if !result.ShouldProceed {
		t.Error("Should allow retry after failure")
	}
	if result.Reason != "previous attempt failed, retry allowed" {
		t.Errorf("Reason = %s, unexpected", result.Reason)
	}
}

func TestIdempotencyManager_DifferentRequest(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	key := "test-key-4"
	data1 := map[string]string{"foo": "bar"}
	data2 := map[string]string{"foo": "baz"} // Different data

	// Start with data1
	m.Start(key, data1)
	m.Complete(key, true, nil, nil)

	// Check with different data should allow proceeding
	result := m.Check(key, data2)
	if result.IsDuplicate {
		t.Error("Different request data should not be a duplicate")
	}
	if !result.ShouldProceed {
		t.Error("Different request data should allow proceeding")
	}
}

func TestIdempotencyManager_FileChangeTracking(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	key := "test-key-5"
	m.Start(key, nil)

	// Record file changes
	m.RecordFileChange(key, FileChangeRecord{
		Path:      "/tmp/file1.txt",
		Operation: IdempotentFileOpCreate,
		Applied:   false,
	})
	m.RecordFileChange(key, FileChangeRecord{
		Path:      "/tmp/file2.txt",
		Operation: IdempotentFileOpModify,
		Applied:   false,
	})

	// Mark one as applied
	m.MarkFileChangeApplied(key, "/tmp/file1.txt")

	// Check applied changes
	applied := m.GetAppliedChanges(key)
	if len(applied) != 1 {
		t.Errorf("Applied changes = %d, want 1", len(applied))
	}
	if applied[0].Path != "/tmp/file1.txt" {
		t.Errorf("Applied path = %s, want /tmp/file1.txt", applied[0].Path)
	}

	// Check pending changes
	pending := m.GetPendingChanges(key)
	if len(pending) != 1 {
		t.Errorf("Pending changes = %d, want 1", len(pending))
	}
	if pending[0].Path != "/tmp/file2.txt" {
		t.Errorf("Pending path = %s, want /tmp/file2.txt", pending[0].Path)
	}

	// Check HasAppliedChanges
	if !m.HasAppliedChanges(key) {
		t.Error("HasAppliedChanges should be true")
	}
}

func TestIdempotencyManager_Metadata(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	key := "test-key-6"
	m.Start(key, nil)

	// Set metadata
	m.SetMetadata(key, "provider", "openai")
	m.SetMetadata(key, "model", "gpt-4")

	// Verify metadata
	record := m.Get(key)
	if record == nil {
		t.Fatal("Record should exist")
	}
	if record.Metadata["provider"] != "openai" {
		t.Errorf("Metadata[provider] = %s, want openai", record.Metadata["provider"])
	}
	if record.Metadata["model"] != "gpt-4" {
		t.Errorf("Metadata[model] = %s, want gpt-4", record.Metadata["model"])
	}
}

func TestIdempotencyManager_Cleanup(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Millisecond) // Very short TTL

	// Start some operations
	m.Start("key1", nil)
	m.Start("key2", nil)

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	// Cleanup should remove old records
	removed := m.Cleanup()
	if removed != 2 {
		t.Errorf("Cleanup removed %d, want 2", removed)
	}

	// Records should be gone
	if m.Get("key1") != nil {
		t.Error("key1 should be removed")
	}
	if m.Get("key2") != nil {
		t.Error("key2 should be removed")
	}
}

func TestIdempotencyManager_Clear(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	m.Start("key1", nil)
	m.Start("key2", nil)

	m.Clear()

	if m.Get("key1") != nil {
		t.Error("key1 should be cleared")
	}
	if m.Get("key2") != nil {
		t.Error("key2 should be cleared")
	}
}

func TestIdempotencyManager_Stats(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	m.Start("key1", nil)
	m.Complete("key1", true, nil, nil)

	m.Start("key2", nil)
	m.Complete("key2", false, nil, &mockError{msg: "error"})

	m.Start("key3", nil) // Still in progress

	// Retry key2
	m.Start("key2", nil)

	stats := m.GetStats()

	if stats.TotalRecords != 3 {
		t.Errorf("TotalRecords = %d, want 3", stats.TotalRecords)
	}
	if stats.CompletedRecords != 1 {
		t.Errorf("CompletedRecords = %d, want 1", stats.CompletedRecords)
	}
	if stats.InProgress != 2 {
		t.Errorf("InProgress = %d, want 2", stats.InProgress)
	}
	if stats.TotalRetries != 1 {
		t.Errorf("TotalRetries = %d, want 1", stats.TotalRetries)
	}
}

func TestGenerateIdempotencyKey(t *testing.T) {
	key1 := GenerateIdempotencyKey("plan1", "branch1", "op1", nil)
	key2 := GenerateIdempotencyKey("plan1", "branch1", "op1", nil)
	key3 := GenerateIdempotencyKey("plan1", "branch2", "op1", nil)

	// Same inputs should produce same key
	if key1 != key2 {
		t.Error("Same inputs should produce same key")
	}

	// Different inputs should produce different key
	if key1 == key3 {
		t.Error("Different inputs should produce different key")
	}

	// Key should be non-empty
	if key1 == "" {
		t.Error("Key should not be empty")
	}
}

func TestGenerateRequestIdempotencyKey(t *testing.T) {
	key1 := GenerateRequestIdempotencyKey("openai", "gpt-4", "hash123")
	key2 := GenerateRequestIdempotencyKey("openai", "gpt-4", "hash123")
	key3 := GenerateRequestIdempotencyKey("anthropic", "claude-3", "hash123")

	// Same inputs should produce same key
	if key1 != key2 {
		t.Error("Same inputs should produce same key")
	}

	// Different inputs should produce different key
	if key1 == key3 {
		t.Error("Different inputs should produce different key")
	}
}

func TestIdempotencyManager_RolledBackState(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Hour)

	key := "test-key-rollback"
	data := map[string]string{"foo": "bar"}

	// Start the operation
	record := m.Start(key, data)
	record.Status = IdempotencyRolledBack // Simulate rollback

	// Check should allow retry after rollback
	result := m.Check(key, data)
	if !result.IsDuplicate {
		t.Error("Should be a duplicate")
	}
	if !result.ShouldProceed {
		t.Error("Should allow retry after rollback")
	}
}

func TestIdempotencyManager_ExpiredRecord(t *testing.T) {
	m := NewIdempotencyManager(1 * time.Millisecond) // Very short TTL

	key := "test-key-expired"
	data := map[string]string{"foo": "bar"}

	// Start and complete
	m.Start(key, data)
	m.Complete(key, true, nil, nil)

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	// Check should allow proceeding (record expired)
	result := m.Check(key, data)
	if result.IsDuplicate {
		t.Error("Expired record should not be a duplicate")
	}
	if !result.ShouldProceed {
		t.Error("Should allow proceeding after expiry")
	}
}
