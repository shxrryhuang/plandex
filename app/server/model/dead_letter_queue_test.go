package model

import (
	shared "plandex-shared"
	"testing"
	"time"
)

func TestDeadLetterQueue_Add(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
		Message:  "Rate limited",
	}

	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	if item == nil {
		t.Fatal("Should return item")
	}
	if item.Id == "" {
		t.Error("Should have ID")
	}
	if item.OperationType != "model_request" {
		t.Errorf("OperationType = %s, want model_request", item.OperationType)
	}
	if item.Provider != "openai" {
		t.Errorf("Provider = %s, want openai", item.Provider)
	}
	if item.FailureType != shared.FailureTypeRateLimit {
		t.Errorf("FailureType = %s, want rate_limit", item.FailureType)
	}
	// Default config has AutoRetryEnabled=true, so status should be scheduled
	if item.Status != DLQStatusScheduled {
		t.Errorf("Status = %s, want scheduled (auto-retry enabled)", item.Status)
	}
}

func TestDeadLetterQueue_Get(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	added := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	retrieved := q.Get(added.Id)
	if retrieved == nil {
		t.Fatal("Should find item")
	}
	if retrieved.Id != added.Id {
		t.Errorf("Id = %s, want %s", retrieved.Id, added.Id)
	}

	// Non-existent
	notFound := q.Get("nonexistent")
	if notFound != nil {
		t.Error("Should not find non-existent item")
	}
}

func TestDeadLetterQueue_List(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure1 := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit, Provider: "openai"}
	failure2 := &shared.ProviderFailure{Type: shared.FailureTypeOverloaded, Provider: "anthropic"}

	q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure1, 3)
	q.Add("model_request", "anthropic", "claude-3", "plan-456", nil, failure2, 2)
	q.Add("file_write", "openai", "", "plan-123", nil, failure1, 1)

	// Filter by provider
	openaiItems := q.List(DLQFilter{Provider: "openai"})
	if len(openaiItems) != 2 {
		t.Errorf("OpenAI items = %d, want 2", len(openaiItems))
	}

	// Filter by failure type
	rateLimitItems := q.GetByFailureType(shared.FailureTypeRateLimit)
	if len(rateLimitItems) != 2 {
		t.Errorf("Rate limit items = %d, want 2", len(rateLimitItems))
	}

	// Filter by operation type
	fileWriteItems := q.List(DLQFilter{OperationType: "file_write"})
	if len(fileWriteItems) != 1 {
		t.Errorf("File write items = %d, want 1", len(fileWriteItems))
	}
}

func TestDeadLetterQueue_MarkForRetry(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	err := q.MarkForRetry(item.Id, 1*time.Hour)
	if err != nil {
		t.Fatalf("MarkForRetry error: %v", err)
	}

	updated := q.Get(item.Id)
	if updated.Status != DLQStatusScheduled {
		t.Errorf("Status = %s, want scheduled", updated.Status)
	}
	if updated.NextRetryAt == nil {
		t.Error("NextRetryAt should be set")
	}
}

func TestDeadLetterQueue_StartAndCompleteRetry(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	// Start retry
	started, err := q.StartRetry(item.Id)
	if err != nil {
		t.Fatalf("StartRetry error: %v", err)
	}
	if started.Status != DLQStatusProcessing {
		t.Errorf("Status = %s, want processing", started.Status)
	}

	// Complete with success
	q.CompleteRetry(item.Id, true, "")

	updated := q.Get(item.Id)
	if updated.Status != DLQStatusResolved {
		t.Errorf("Status = %s, want resolved", updated.Status)
	}
	if updated.Resolution != "retried_success" {
		t.Errorf("Resolution = %s, want retried_success", updated.Resolution)
	}
}

func TestDeadLetterQueue_RetryWithFailure(t *testing.T) {
	config := &DLQConfig{
		MaxItems:          100,
		DefaultTTL:        1 * time.Hour,
		AutoRetryEnabled:  true,
		AutoRetryDelay:    1 * time.Minute,
		AutoRetryMaxCount: 3,
		CleanupInterval:   1 * time.Hour,
		KeepResolved:      1 * time.Hour,
	}
	q := NewDeadLetterQueue(config)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	// Start and fail retry
	q.StartRetry(item.Id)
	q.CompleteRetry(item.Id, false, "Still failing")

	updated := q.Get(item.Id)
	if updated.Status != DLQStatusScheduled {
		t.Errorf("Status = %s, want scheduled (for next retry)", updated.Status)
	}
	if len(updated.FailureHistory) != 2 {
		t.Errorf("FailureHistory length = %d, want 2", len(updated.FailureHistory))
	}
}

func TestDeadLetterQueue_MaxRetries(t *testing.T) {
	config := &DLQConfig{
		MaxItems:          100,
		DefaultTTL:        1 * time.Hour,
		AutoRetryEnabled:  true,
		AutoRetryDelay:    1 * time.Minute,
		AutoRetryMaxCount: 2,
		CleanupInterval:   1 * time.Hour,
		KeepResolved:      1 * time.Hour,
	}
	q := NewDeadLetterQueue(config)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	// Exhaust retries
	for i := 0; i < 3; i++ {
		q.StartRetry(item.Id)
		q.CompleteRetry(item.Id, false, "Failed")
	}

	updated := q.Get(item.Id)
	if updated.Status != DLQStatusPending {
		t.Errorf("Status = %s, want pending (max retries exceeded)", updated.Status)
	}
	if updated.NextRetryAt != nil {
		t.Error("NextRetryAt should be nil after max retries")
	}
}

func TestDeadLetterQueue_Resolve(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	err := q.Resolve(item.Id, "manual fix", "admin")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	updated := q.Get(item.Id)
	if updated.Status != DLQStatusResolved {
		t.Errorf("Status = %s, want resolved", updated.Status)
	}
	if updated.Resolution != "manual fix" {
		t.Errorf("Resolution = %s, want manual fix", updated.Resolution)
	}
	if updated.ResolvedBy != "admin" {
		t.Errorf("ResolvedBy = %s, want admin", updated.ResolvedBy)
	}
}

func TestDeadLetterQueue_Discard(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	err := q.Discard(item.Id, "not needed")
	if err != nil {
		t.Fatalf("Discard error: %v", err)
	}

	updated := q.Get(item.Id)
	if updated.Status != DLQStatusDiscarded {
		t.Errorf("Status = %s, want discarded", updated.Status)
	}
}

func TestDeadLetterQueue_GetPendingItems(t *testing.T) {
	// Use config with auto-retry disabled so items are pending
	config := &DLQConfig{
		MaxItems:          100,
		DefaultTTL:        1 * time.Hour,
		AutoRetryEnabled:  false, // Disable auto-retry so items are pending
		CleanupInterval:   1 * time.Hour,
		KeepResolved:      1 * time.Hour,
	}
	q := NewDeadLetterQueue(config)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}

	q.Add("model_request", "openai", "gpt-4", "plan-1", nil, failure, 3)
	item2 := q.Add("model_request", "openai", "gpt-4", "plan-2", nil, failure, 3)
	q.Add("model_request", "openai", "gpt-4", "plan-3", nil, failure, 3)

	// Resolve one
	q.Resolve(item2.Id, "fixed", "admin")

	pending := q.GetPendingItems()
	if len(pending) != 2 {
		t.Errorf("Pending items = %d, want 2", len(pending))
	}
}

func TestDeadLetterQueue_GetItemsDueForRetry(t *testing.T) {
	config := &DLQConfig{
		MaxItems:          100,
		DefaultTTL:        1 * time.Hour,
		AutoRetryEnabled:  true,
		AutoRetryDelay:    1 * time.Millisecond, // Very short for testing
		AutoRetryMaxCount: 3,
		CleanupInterval:   1 * time.Hour,
		KeepResolved:      1 * time.Hour,
	}
	q := NewDeadLetterQueue(config)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	// Wait for retry time
	time.Sleep(10 * time.Millisecond)

	due := q.GetItemsDueForRetry()
	if len(due) != 1 {
		t.Errorf("Items due = %d, want 1", len(due))
	}
}

func TestDeadLetterQueue_MaxItems(t *testing.T) {
	config := &DLQConfig{
		MaxItems:        3,
		DefaultTTL:      1 * time.Hour,
		CleanupInterval: 1 * time.Hour,
		KeepResolved:    1 * time.Hour,
	}
	q := NewDeadLetterQueue(config)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}

	// Add items up to limit
	q.Add("model_request", "openai", "gpt-4", "plan-1", nil, failure, 1)
	q.Add("model_request", "openai", "gpt-4", "plan-2", nil, failure, 1)
	q.Add("model_request", "openai", "gpt-4", "plan-3", nil, failure, 1)

	// Add one more - should evict oldest
	q.Add("model_request", "openai", "gpt-4", "plan-4", nil, failure, 1)

	stats := q.GetStats()
	if stats.CurrentSize != 3 {
		t.Errorf("CurrentSize = %d, want 3 (max)", stats.CurrentSize)
	}
}

func TestDeadLetterQueue_Stats(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}

	q.Add("model_request", "openai", "gpt-4", "plan-1", nil, failure, 1)
	item2 := q.Add("model_request", "openai", "gpt-4", "plan-2", nil, failure, 1)
	item3 := q.Add("model_request", "openai", "gpt-4", "plan-3", nil, failure, 1)

	q.Resolve(item2.Id, "fixed", "admin")
	q.Discard(item3.Id, "not needed")

	stats := q.GetStats()

	if stats.TotalAdded != 3 {
		t.Errorf("TotalAdded = %d, want 3", stats.TotalAdded)
	}
	if stats.TotalResolved != 1 {
		t.Errorf("TotalResolved = %d, want 1", stats.TotalResolved)
	}
	if stats.TotalDiscarded != 1 {
		t.Errorf("TotalDiscarded = %d, want 1", stats.TotalDiscarded)
	}
	if stats.CurrentSize != 3 {
		t.Errorf("CurrentSize = %d, want 3", stats.CurrentSize)
	}
}

func TestDeadLetterQueue_Metrics(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	failure1 := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	failure2 := &shared.ProviderFailure{Type: shared.FailureTypeOverloaded}

	q.Add("model_request", "openai", "gpt-4", "plan-1", nil, failure1, 1)
	q.Add("model_request", "anthropic", "claude-3", "plan-2", nil, failure2, 1)
	q.Add("file_write", "openai", "", "plan-3", nil, failure1, 1)

	metrics := q.GetMetrics()

	if metrics.ByProvider["openai"] != 2 {
		t.Errorf("ByProvider[openai] = %d, want 2", metrics.ByProvider["openai"])
	}
	if metrics.ByProvider["anthropic"] != 1 {
		t.Errorf("ByProvider[anthropic] = %d, want 1", metrics.ByProvider["anthropic"])
	}
	if metrics.ByFailureType[shared.FailureTypeRateLimit] != 2 {
		t.Errorf("ByFailureType[rate_limit] = %d, want 2", metrics.ByFailureType[shared.FailureTypeRateLimit])
	}
	if metrics.ByOperationType["model_request"] != 2 {
		t.Errorf("ByOperationType[model_request] = %d, want 2", metrics.ByOperationType["model_request"])
	}
}

func TestDeadLetterQueue_Callbacks(t *testing.T) {
	q := NewDeadLetterQueue(nil)

	addedCalled := false
	retriedCalled := false
	var retriedSuccess bool

	q.SetItemAddedCallback(func(item *DeadLetterItem) {
		addedCalled = true
	})

	q.SetItemRetriedCallback(func(item *DeadLetterItem, success bool) {
		retriedCalled = true
		retriedSuccess = success
	})

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	item := q.Add("model_request", "openai", "gpt-4", "plan-123", nil, failure, 3)

	// Give callback time to run
	time.Sleep(10 * time.Millisecond)

	if !addedCalled {
		t.Error("Add callback should have been called")
	}

	// Retry
	q.StartRetry(item.Id)
	q.CompleteRetry(item.Id, true, "")

	// Give callback time to run
	time.Sleep(10 * time.Millisecond)

	if !retriedCalled {
		t.Error("Retry callback should have been called")
	}
	if !retriedSuccess {
		t.Error("Retry should have been successful")
	}
}
