package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// IDEMPOTENCY TRACKING
// =============================================================================
//
// This file provides idempotency tracking to prevent duplicate side effects
// during retry operations. When an operation is retried, we need to ensure
// that irreversible side effects (like file modifications) are not repeated.
//
// =============================================================================

// IdempotencyManager tracks operation execution to prevent duplicates
type IdempotencyManager struct {
	mu     sync.RWMutex
	store  map[string]*IdempotencyRecord
	maxAge time.Duration

	// Optional callback when duplicates are detected
	onDuplicate func(key string, record *IdempotencyRecord)
}

// IdempotencyRecord tracks a single operation
type IdempotencyRecord struct {
	// Key uniquely identifies this operation
	Key string `json:"key"`

	// RequestHash is a hash of the request parameters for verification
	RequestHash string `json:"requestHash"`

	// Timestamps
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Status of the operation
	Status IdempotencyStatus `json:"status"`

	// Attempt tracking
	AttemptNumber int `json:"attemptNumber"`

	// Result tracking (if completed)
	Success    bool   `json:"success,omitempty"`
	ResultHash string `json:"resultHash,omitempty"`
	Error      string `json:"error,omitempty"`

	// File operations tracked for this idempotent operation
	FileChanges []FileChangeRecord `json:"fileChanges,omitempty"`

	// Metadata for debugging
	Metadata map[string]string `json:"metadata,omitempty"`
}

// IdempotentFileOp represents the type of file operation for idempotency tracking
type IdempotentFileOp string

const (
	IdempotentFileOpCreate IdempotentFileOp = "create"
	IdempotentFileOpModify IdempotentFileOp = "modify"
	IdempotentFileOpDelete IdempotentFileOp = "delete"
	IdempotentFileOpRename IdempotentFileOp = "rename"
)

// FileChangeRecord tracks a file change for idempotency
type FileChangeRecord struct {
	// Path is the file path that was changed
	Path string `json:"path"`

	// Operation type: create, modify, delete, rename
	Operation IdempotentFileOp `json:"operation"`

	// Hashes for verification
	BeforeHash string `json:"beforeHash,omitempty"`
	AfterHash  string `json:"afterHash,omitempty"`

	// Applied indicates if this change was successfully applied
	Applied bool `json:"applied"`

	// AppliedAt is when the change was applied
	AppliedAt *time.Time `json:"appliedAt,omitempty"`

	// RollbackAvailable indicates if the change can be undone
	RollbackAvailable bool `json:"rollbackAvailable,omitempty"`
}

// IdempotencyStatus tracks the status of an idempotent operation
type IdempotencyStatus string

const (
	// IdempotencyPending - operation registered but not started
	IdempotencyPending IdempotencyStatus = "pending"

	// IdempotencyInProgress - operation is currently executing
	IdempotencyInProgress IdempotencyStatus = "in_progress"

	// IdempotencyCompleted - operation completed successfully
	IdempotencyCompleted IdempotencyStatus = "completed"

	// IdempotencyFailed - operation failed
	IdempotencyFailed IdempotencyStatus = "failed"

	// IdempotencyRolledBack - operation was rolled back
	IdempotencyRolledBack IdempotencyStatus = "rolled_back"
)

// IdempotencyCheckResult indicates the result of checking for duplicates
type IdempotencyCheckResult struct {
	// IsDuplicate is true if this operation was already executed
	IsDuplicate bool `json:"isDuplicate"`

	// Record is the existing record if this is a duplicate
	Record *IdempotencyRecord `json:"record,omitempty"`

	// ShouldProceed indicates if the caller should proceed with the operation
	// This is false if the operation is already in progress or completed
	ShouldProceed bool `json:"shouldProceed"`

	// Reason explains the decision
	Reason string `json:"reason"`
}

// =============================================================================
// MANAGER LIFECYCLE
// =============================================================================

// NewIdempotencyManager creates a new idempotency manager
// maxAge specifies how long to keep records (default: 24 hours)
func NewIdempotencyManager(maxAge time.Duration) *IdempotencyManager {
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}

	return &IdempotencyManager{
		store:  make(map[string]*IdempotencyRecord),
		maxAge: maxAge,
	}
}

// SetDuplicateCallback sets a callback for when duplicates are detected
func (m *IdempotencyManager) SetDuplicateCallback(callback func(key string, record *IdempotencyRecord)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDuplicate = callback
}

// =============================================================================
// CORE OPERATIONS
// =============================================================================

// Check checks if an operation was already executed
// Returns detailed information about whether to proceed
func (m *IdempotencyManager) Check(key string, requestData interface{}) IdempotencyCheckResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, exists := m.store[key]
	if !exists {
		return IdempotencyCheckResult{
			IsDuplicate:   false,
			ShouldProceed: true,
			Reason:        "new operation",
		}
	}

	// Verify request hash matches
	newHash := hashData(requestData)
	if record.RequestHash != newHash {
		return IdempotencyCheckResult{
			IsDuplicate:   false,
			ShouldProceed: true,
			Reason:        "different request with same key",
		}
	}

	// Check if record is too old
	if time.Since(record.CreatedAt) > m.maxAge {
		return IdempotencyCheckResult{
			IsDuplicate:   false,
			ShouldProceed: true,
			Reason:        "record expired",
		}
	}

	// Record exists and matches - check status
	switch record.Status {
	case IdempotencyCompleted:
		if record.Success {
			return IdempotencyCheckResult{
				IsDuplicate:   true,
				Record:        record,
				ShouldProceed: false,
				Reason:        "operation already completed successfully",
			}
		}
		// Completed but failed - allow retry
		return IdempotencyCheckResult{
			IsDuplicate:   true,
			Record:        record,
			ShouldProceed: true,
			Reason:        "previous attempt failed, retry allowed",
		}

	case IdempotencyInProgress:
		return IdempotencyCheckResult{
			IsDuplicate:   true,
			Record:        record,
			ShouldProceed: false,
			Reason:        "operation currently in progress",
		}

	case IdempotencyFailed:
		return IdempotencyCheckResult{
			IsDuplicate:   true,
			Record:        record,
			ShouldProceed: true,
			Reason:        "previous attempt failed, retry allowed",
		}

	case IdempotencyRolledBack:
		return IdempotencyCheckResult{
			IsDuplicate:   true,
			Record:        record,
			ShouldProceed: true,
			Reason:        "previous attempt was rolled back, retry allowed",
		}

	default:
		return IdempotencyCheckResult{
			IsDuplicate:   true,
			Record:        record,
			ShouldProceed: true,
			Reason:        "unknown status, allowing retry",
		}
	}
}

// Start marks an operation as started and returns the record
func (m *IdempotencyManager) Start(key string, requestData interface{}) *IdempotencyRecord {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if record already exists
	existing, exists := m.store[key]
	if exists {
		// Update existing record for retry
		now := time.Now()
		existing.StartedAt = &now
		existing.Status = IdempotencyInProgress
		existing.AttemptNumber++
		existing.CompletedAt = nil
		existing.Error = ""
		return existing
	}

	// Create new record
	now := time.Now()
	record := &IdempotencyRecord{
		Key:           key,
		RequestHash:   hashData(requestData),
		CreatedAt:     now,
		StartedAt:     &now,
		Status:        IdempotencyInProgress,
		AttemptNumber: 1,
		FileChanges:   []FileChangeRecord{},
		Metadata:      make(map[string]string),
	}

	m.store[key] = record
	return record
}

// Complete marks an operation as completed
func (m *IdempotencyManager) Complete(key string, success bool, result interface{}, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, exists := m.store[key]
	if !exists {
		return
	}

	now := time.Now()
	record.CompletedAt = &now
	record.Success = success

	if success {
		record.Status = IdempotencyCompleted
		if result != nil {
			record.ResultHash = hashData(result)
		}
	} else {
		record.Status = IdempotencyFailed
		if err != nil {
			record.Error = err.Error()
		}
	}
}

// RecordFileChange records a file change for an operation
func (m *IdempotencyManager) RecordFileChange(key string, change FileChangeRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, exists := m.store[key]
	if !exists {
		return
	}

	record.FileChanges = append(record.FileChanges, change)
}

// MarkFileChangeApplied marks a specific file change as applied
func (m *IdempotencyManager) MarkFileChangeApplied(key string, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, exists := m.store[key]
	if !exists {
		return
	}

	now := time.Now()
	for i := range record.FileChanges {
		if record.FileChanges[i].Path == path && !record.FileChanges[i].Applied {
			record.FileChanges[i].Applied = true
			record.FileChanges[i].AppliedAt = &now
			break
		}
	}
}

// SetMetadata sets metadata on an idempotency record
func (m *IdempotencyManager) SetMetadata(key string, metaKey string, metaValue string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, exists := m.store[key]
	if !exists {
		return
	}

	if record.Metadata == nil {
		record.Metadata = make(map[string]string)
	}
	record.Metadata[metaKey] = metaValue
}

// =============================================================================
// QUERIES
// =============================================================================

// Get retrieves an idempotency record by key
func (m *IdempotencyManager) Get(key string) *IdempotencyRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, exists := m.store[key]
	if !exists {
		return nil
	}

	// Return a copy
	copy := *record
	return &copy
}

// GetAppliedChanges returns file changes that were applied for an operation
func (m *IdempotencyManager) GetAppliedChanges(key string) []FileChangeRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, exists := m.store[key]
	if !exists {
		return nil
	}

	var applied []FileChangeRecord
	for _, c := range record.FileChanges {
		if c.Applied {
			applied = append(applied, c)
		}
	}
	return applied
}

// GetPendingChanges returns file changes that were not yet applied
func (m *IdempotencyManager) GetPendingChanges(key string) []FileChangeRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, exists := m.store[key]
	if !exists {
		return nil
	}

	var pending []FileChangeRecord
	for _, c := range record.FileChanges {
		if !c.Applied {
			pending = append(pending, c)
		}
	}
	return pending
}

// HasAppliedChanges checks if any file changes were applied for an operation
func (m *IdempotencyManager) HasAppliedChanges(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, exists := m.store[key]
	if !exists {
		return false
	}

	for _, c := range record.FileChanges {
		if c.Applied {
			return true
		}
	}
	return false
}

// =============================================================================
// CLEANUP
// =============================================================================

// Cleanup removes old records that have exceeded maxAge
func (m *IdempotencyManager) Cleanup() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-m.maxAge)
	removed := 0

	for key, record := range m.store {
		if record.CreatedAt.Before(cutoff) {
			delete(m.store, key)
			removed++
		}
	}

	return removed
}

// Remove removes a specific record
func (m *IdempotencyManager) Remove(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.store, key)
}

// Clear removes all records
func (m *IdempotencyManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.store = make(map[string]*IdempotencyRecord)
}

// =============================================================================
// STATISTICS
// =============================================================================

// IdempotencyStats provides statistics about the idempotency manager
type IdempotencyStats struct {
	TotalRecords    int `json:"totalRecords"`
	PendingRecords  int `json:"pendingRecords"`
	InProgress      int `json:"inProgress"`
	CompletedRecords int `json:"completedRecords"`
	FailedRecords   int `json:"failedRecords"`
	TotalRetries    int `json:"totalRetries"`
}

// GetStats returns statistics about the idempotency manager
func (m *IdempotencyManager) GetStats() IdempotencyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := IdempotencyStats{
		TotalRecords: len(m.store),
	}

	for _, record := range m.store {
		switch record.Status {
		case IdempotencyPending:
			stats.PendingRecords++
		case IdempotencyInProgress:
			stats.InProgress++
		case IdempotencyCompleted:
			stats.CompletedRecords++
		case IdempotencyFailed:
			stats.FailedRecords++
		}

		if record.AttemptNumber > 1 {
			stats.TotalRetries += record.AttemptNumber - 1
		}
	}

	return stats
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// hashData creates a SHA256 hash of the data
func hashData(data interface{}) string {
	if data == nil {
		return ""
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		// Fallback to string representation
		bytes = []byte(fmt.Sprintf("%v", data))
	}

	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:])
}

// GenerateIdempotencyKey creates a deterministic key from operation parameters
func GenerateIdempotencyKey(planId, branch, operation string, params map[string]string) string {
	data := map[string]interface{}{
		"planId":    planId,
		"branch":    branch,
		"operation": operation,
		"params":    params,
	}
	return hashData(data)
}

// GenerateRequestIdempotencyKey creates a key for a specific request
func GenerateRequestIdempotencyKey(provider, model, requestHash string) string {
	data := map[string]string{
		"provider":    provider,
		"model":       model,
		"requestHash": requestHash,
	}
	return hashData(data)
}

// GenerateIdWithPrefix creates a unique ID with the given prefix
func GenerateIdWithPrefix(prefix string) string {
	timestamp := time.Now().UnixNano()
	random := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d", timestamp))))[:8]
	return fmt.Sprintf("%s_%d_%s", prefix, timestamp, random)
}
