package model

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	shared "plandex-shared"
)

// =============================================================================
// DEAD LETTER QUEUE
// =============================================================================
//
// The Dead Letter Queue (DLQ) stores failed operations that couldn't be
// completed after all retry attempts. This enables:
//
// - Manual review and retry of failed operations
// - Debugging and root cause analysis
// - Recovery when provider issues are resolved
// - Audit trail of all failed operations
//
// Operations in the DLQ can be:
// - Manually retried when conditions improve
// - Automatically retried after a cooling period
// - Exported for offline analysis
// - Discarded after review
//
// =============================================================================

// DeadLetterQueue stores failed operations for later processing
type DeadLetterQueue struct {
	mu sync.RWMutex

	// Queue storage
	items map[string]*DeadLetterItem

	// Configuration
	config DLQConfig

	// Statistics
	stats DLQStats

	// Optional callbacks
	onItemAdded   func(item *DeadLetterItem)
	onItemRetried func(item *DeadLetterItem, success bool)
}

// DeadLetterItem represents a failed operation in the queue
type DeadLetterItem struct {
	// Identity
	Id        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Operation context
	OperationType string            `json:"operationType"` // "model_request", "file_write", etc.
	Provider      string            `json:"provider,omitempty"`
	Model         string            `json:"model,omitempty"`
	PlanId        string            `json:"planId,omitempty"`
	Branch        string            `json:"branch,omitempty"`
	UserId        string            `json:"userId,omitempty"`

	// Request data (serialized for storage)
	RequestData json.RawMessage `json:"requestData,omitempty"`

	// Failure information
	FailureType    shared.FailureType `json:"failureType"`
	LastError      string             `json:"lastError"`
	HTTPCode       int                `json:"httpCode,omitempty"`
	TotalAttempts  int                `json:"totalAttempts"`
	FailureHistory []FailureRecord    `json:"failureHistory"`

	// Status
	Status       DLQItemStatus `json:"status"`
	RetryCount   int           `json:"retryCount"`
	NextRetryAt  *time.Time    `json:"nextRetryAt,omitempty"`
	ExpiresAt    *time.Time    `json:"expiresAt,omitempty"`

	// Resolution
	ResolvedAt   *time.Time `json:"resolvedAt,omitempty"`
	Resolution   string     `json:"resolution,omitempty"` // "retried_success", "discarded", "expired"
	ResolvedBy   string     `json:"resolvedBy,omitempty"` // "auto", "manual", "system"

	// Metadata
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// FailureRecord tracks a single failure occurrence
type FailureRecord struct {
	Timestamp   time.Time          `json:"timestamp"`
	FailureType shared.FailureType `json:"failureType"`
	Error       string             `json:"error"`
	HTTPCode    int                `json:"httpCode,omitempty"`
	AttemptNum  int                `json:"attemptNum"`
}

// DLQItemStatus represents the status of a DLQ item
type DLQItemStatus string

const (
	DLQStatusPending    DLQItemStatus = "pending"     // Waiting for retry
	DLQStatusScheduled  DLQItemStatus = "scheduled"   // Scheduled for auto-retry
	DLQStatusProcessing DLQItemStatus = "processing"  // Currently being retried
	DLQStatusResolved   DLQItemStatus = "resolved"    // Successfully resolved
	DLQStatusDiscarded  DLQItemStatus = "discarded"   // Manually discarded
	DLQStatusExpired    DLQItemStatus = "expired"     // Expired without resolution
)

// DLQConfig configures the dead letter queue
type DLQConfig struct {
	// Maximum items to store
	MaxItems int `json:"maxItems"`

	// Item expiration
	DefaultTTL time.Duration `json:"defaultTTL"`

	// Auto-retry settings
	AutoRetryEnabled  bool          `json:"autoRetryEnabled"`
	AutoRetryDelay    time.Duration `json:"autoRetryDelay"`
	AutoRetryMaxCount int           `json:"autoRetryMaxCount"`

	// Cleanup settings
	CleanupInterval time.Duration `json:"cleanupInterval"`
	KeepResolved    time.Duration `json:"keepResolved"` // How long to keep resolved items

	// Notification settings
	NotifyOnAdd       bool `json:"notifyOnAdd"`
	NotifyOnThreshold int  `json:"notifyOnThreshold"` // Notify when queue exceeds this size
}

// DefaultDLQConfig provides sensible defaults
var DefaultDLQConfig = DLQConfig{
	MaxItems:          1000,
	DefaultTTL:        7 * 24 * time.Hour, // 7 days
	AutoRetryEnabled:  true,
	AutoRetryDelay:    1 * time.Hour,
	AutoRetryMaxCount: 3,
	CleanupInterval:   1 * time.Hour,
	KeepResolved:      24 * time.Hour,
	NotifyOnAdd:       true,
	NotifyOnThreshold: 100,
}

// DLQStats tracks queue statistics
type DLQStats struct {
	TotalAdded      int64     `json:"totalAdded"`
	TotalResolved   int64     `json:"totalResolved"`
	TotalDiscarded  int64     `json:"totalDiscarded"`
	TotalExpired    int64     `json:"totalExpired"`
	TotalAutoRetried int64    `json:"totalAutoRetried"`
	CurrentSize     int       `json:"currentSize"`
	OldestItem      time.Time `json:"oldestItem,omitempty"`
}

// GlobalDeadLetterQueue is the singleton instance
var GlobalDeadLetterQueue *DeadLetterQueue

// InitGlobalDeadLetterQueue initializes the global DLQ
func InitGlobalDeadLetterQueue() {
	GlobalDeadLetterQueue = NewDeadLetterQueue(nil)
}

// InitGlobalDeadLetterQueueWithConfig initializes with custom config
func InitGlobalDeadLetterQueueWithConfig(config *DLQConfig) {
	GlobalDeadLetterQueue = NewDeadLetterQueue(config)
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(config *DLQConfig) *DeadLetterQueue {
	if config == nil {
		config = &DefaultDLQConfig
	}

	dlq := &DeadLetterQueue{
		items:  make(map[string]*DeadLetterItem),
		config: *config,
		stats:  DLQStats{},
	}

	// Start cleanup goroutine
	go dlq.cleanupLoop()

	return dlq
}

// SetItemAddedCallback sets a callback for when items are added
func (q *DeadLetterQueue) SetItemAddedCallback(callback func(item *DeadLetterItem)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onItemAdded = callback
}

// SetItemRetriedCallback sets a callback for when items are retried
func (q *DeadLetterQueue) SetItemRetriedCallback(callback func(item *DeadLetterItem, success bool)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onItemRetried = callback
}

// =============================================================================
// QUEUE OPERATIONS
// =============================================================================

// Add adds a failed operation to the queue
func (q *DeadLetterQueue) Add(
	operationType string,
	provider string,
	model string,
	planId string,
	requestData interface{},
	failure *shared.ProviderFailure,
	totalAttempts int,
) *DeadLetterItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check capacity
	if len(q.items) >= q.config.MaxItems {
		q.evictOldestLocked()
	}

	// Serialize request data
	var requestBytes json.RawMessage
	if requestData != nil {
		if bytes, err := json.Marshal(requestData); err == nil {
			requestBytes = bytes
		}
	}

	now := time.Now()
	expiresAt := now.Add(q.config.DefaultTTL)

	var nextRetryAt *time.Time
	var status DLQItemStatus = DLQStatusPending
	if q.config.AutoRetryEnabled {
		retry := now.Add(q.config.AutoRetryDelay)
		nextRetryAt = &retry
		status = DLQStatusScheduled
	}

	item := &DeadLetterItem{
		Id:            shared.GenerateIdWithPrefix("dlq"),
		CreatedAt:     now,
		UpdatedAt:     now,
		OperationType: operationType,
		Provider:      provider,
		Model:         model,
		PlanId:        planId,
		RequestData:   requestBytes,
		TotalAttempts: totalAttempts,
		Status:        status,
		NextRetryAt:   nextRetryAt,
		ExpiresAt:     &expiresAt,
		Metadata:      make(map[string]string),
	}

	if failure != nil {
		item.FailureType = failure.Type
		item.LastError = failure.Message
		item.HTTPCode = failure.HTTPCode
		item.FailureHistory = []FailureRecord{
			{
				Timestamp:   now,
				FailureType: failure.Type,
				Error:       failure.Message,
				HTTPCode:    failure.HTTPCode,
				AttemptNum:  totalAttempts,
			},
		}
	}

	q.items[item.Id] = item
	q.stats.TotalAdded++
	q.stats.CurrentSize = len(q.items)

	log.Printf("[DLQ] Added item: id=%s, type=%s, provider=%s, failure=%s",
		item.Id, operationType, provider, item.FailureType)

	// Invoke callback
	if q.onItemAdded != nil {
		go q.onItemAdded(item)
	}

	// Check threshold notification
	if q.config.NotifyOnThreshold > 0 && len(q.items) >= q.config.NotifyOnThreshold {
		log.Printf("[DLQ] WARNING: Queue size (%d) exceeds threshold (%d)",
			len(q.items), q.config.NotifyOnThreshold)
	}

	return item
}

// Get retrieves an item by ID
func (q *DeadLetterQueue) Get(id string) *DeadLetterItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	item, exists := q.items[id]
	if !exists {
		return nil
	}

	// Return a copy
	copy := *item
	return &copy
}

// List returns items matching the given filter
func (q *DeadLetterQueue) List(filter DLQFilter) []*DeadLetterItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var results []*DeadLetterItem

	for _, item := range q.items {
		if filter.matches(item) {
			copy := *item
			results = append(results, &copy)
		}
	}

	return results
}

// DLQFilter defines criteria for filtering DLQ items
type DLQFilter struct {
	Status        *DLQItemStatus     `json:"status,omitempty"`
	Provider      string             `json:"provider,omitempty"`
	OperationType string             `json:"operationType,omitempty"`
	FailureType   *shared.FailureType `json:"failureType,omitempty"`
	PlanId        string             `json:"planId,omitempty"`
	MinAge        time.Duration      `json:"minAge,omitempty"`
	MaxAge        time.Duration      `json:"maxAge,omitempty"`
	Limit         int                `json:"limit,omitempty"`
}

func (f DLQFilter) matches(item *DeadLetterItem) bool {
	if f.Status != nil && item.Status != *f.Status {
		return false
	}
	if f.Provider != "" && item.Provider != f.Provider {
		return false
	}
	if f.OperationType != "" && item.OperationType != f.OperationType {
		return false
	}
	if f.FailureType != nil && item.FailureType != *f.FailureType {
		return false
	}
	if f.PlanId != "" && item.PlanId != f.PlanId {
		return false
	}
	if f.MinAge > 0 && time.Since(item.CreatedAt) < f.MinAge {
		return false
	}
	if f.MaxAge > 0 && time.Since(item.CreatedAt) > f.MaxAge {
		return false
	}
	return true
}

// =============================================================================
// RETRY OPERATIONS
// =============================================================================

// MarkForRetry schedules an item for retry
func (q *DeadLetterQueue) MarkForRetry(id string, delay time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[id]
	if !exists {
		return fmt.Errorf("DLQ item not found: %s", id)
	}

	if item.Status == DLQStatusResolved || item.Status == DLQStatusDiscarded {
		return fmt.Errorf("cannot retry resolved/discarded item")
	}

	now := time.Now()
	retryAt := now.Add(delay)
	item.NextRetryAt = &retryAt
	item.Status = DLQStatusScheduled
	item.UpdatedAt = now

	log.Printf("[DLQ] Scheduled retry: id=%s, retryAt=%v", id, retryAt)

	return nil
}

// StartRetry marks an item as being retried (call before attempting retry)
func (q *DeadLetterQueue) StartRetry(id string) (*DeadLetterItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[id]
	if !exists {
		return nil, fmt.Errorf("DLQ item not found: %s", id)
	}

	if item.Status == DLQStatusProcessing {
		return nil, fmt.Errorf("item is already being processed")
	}

	now := time.Now()
	item.Status = DLQStatusProcessing
	item.RetryCount++
	item.UpdatedAt = now

	q.stats.TotalAutoRetried++

	// Return a copy
	copy := *item
	return &copy, nil
}

// CompleteRetry marks retry as complete (call after retry attempt)
func (q *DeadLetterQueue) CompleteRetry(id string, success bool, newError string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[id]
	if !exists {
		return
	}

	now := time.Now()
	item.UpdatedAt = now

	if success {
		item.Status = DLQStatusResolved
		item.ResolvedAt = &now
		item.Resolution = "retried_success"
		item.ResolvedBy = "auto"
		q.stats.TotalResolved++

		log.Printf("[DLQ] Resolved via retry: id=%s, attempts=%d", id, item.RetryCount)
	} else {
		// Add to failure history
		item.FailureHistory = append(item.FailureHistory, FailureRecord{
			Timestamp:  now,
			Error:      newError,
			AttemptNum: item.TotalAttempts + item.RetryCount,
		})
		item.LastError = newError

		// Check if max retries exceeded
		if item.RetryCount >= q.config.AutoRetryMaxCount {
			item.Status = DLQStatusPending
			item.NextRetryAt = nil
			log.Printf("[DLQ] Max retries exceeded: id=%s, retries=%d", id, item.RetryCount)
		} else {
			// Schedule next retry
			retryAt := now.Add(q.config.AutoRetryDelay * time.Duration(item.RetryCount+1))
			item.NextRetryAt = &retryAt
			item.Status = DLQStatusScheduled
			log.Printf("[DLQ] Retry failed, scheduling next: id=%s, retryAt=%v", id, retryAt)
		}
	}

	// Invoke callback
	if q.onItemRetried != nil {
		copy := *item
		go q.onItemRetried(&copy, success)
	}
}

// =============================================================================
// RESOLUTION OPERATIONS
// =============================================================================

// Resolve manually resolves an item
func (q *DeadLetterQueue) Resolve(id string, resolution string, resolvedBy string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[id]
	if !exists {
		return fmt.Errorf("DLQ item not found: %s", id)
	}

	now := time.Now()
	item.Status = DLQStatusResolved
	item.ResolvedAt = &now
	item.Resolution = resolution
	item.ResolvedBy = resolvedBy
	item.UpdatedAt = now

	q.stats.TotalResolved++

	log.Printf("[DLQ] Manually resolved: id=%s, resolution=%s, by=%s", id, resolution, resolvedBy)

	return nil
}

// Discard marks an item as discarded (will not be retried)
func (q *DeadLetterQueue) Discard(id string, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[id]
	if !exists {
		return fmt.Errorf("DLQ item not found: %s", id)
	}

	now := time.Now()
	item.Status = DLQStatusDiscarded
	item.ResolvedAt = &now
	item.Resolution = "discarded: " + reason
	item.ResolvedBy = "manual"
	item.UpdatedAt = now

	q.stats.TotalDiscarded++

	log.Printf("[DLQ] Discarded: id=%s, reason=%s", id, reason)

	return nil
}

// =============================================================================
// QUERY OPERATIONS
// =============================================================================

// GetPendingItems returns items ready for retry
func (q *DeadLetterQueue) GetPendingItems() []*DeadLetterItem {
	status := DLQStatusPending
	return q.List(DLQFilter{Status: &status})
}

// GetScheduledItems returns items scheduled for auto-retry
func (q *DeadLetterQueue) GetScheduledItems() []*DeadLetterItem {
	status := DLQStatusScheduled
	return q.List(DLQFilter{Status: &status})
}

// GetItemsDueForRetry returns items whose retry time has passed
func (q *DeadLetterQueue) GetItemsDueForRetry() []*DeadLetterItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	now := time.Now()
	var results []*DeadLetterItem

	for _, item := range q.items {
		if item.Status == DLQStatusScheduled && item.NextRetryAt != nil && now.After(*item.NextRetryAt) {
			copy := *item
			results = append(results, &copy)
		}
	}

	return results
}

// GetByProvider returns all items for a provider
func (q *DeadLetterQueue) GetByProvider(provider string) []*DeadLetterItem {
	return q.List(DLQFilter{Provider: provider})
}

// GetByFailureType returns items by failure type
func (q *DeadLetterQueue) GetByFailureType(failureType shared.FailureType) []*DeadLetterItem {
	return q.List(DLQFilter{FailureType: &failureType})
}

// =============================================================================
// CLEANUP
// =============================================================================

func (q *DeadLetterQueue) cleanupLoop() {
	ticker := time.NewTicker(q.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		q.cleanup()
	}
}

func (q *DeadLetterQueue) cleanup() {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	resolvedCutoff := now.Add(-q.config.KeepResolved)

	toDelete := make([]string, 0)

	for id, item := range q.items {
		// Check expiration
		if item.ExpiresAt != nil && now.After(*item.ExpiresAt) && item.Status == DLQStatusPending {
			item.Status = DLQStatusExpired
			item.ResolvedAt = &now
			item.Resolution = "expired"
			q.stats.TotalExpired++
			log.Printf("[DLQ] Expired: id=%s", id)
		}

		// Remove old resolved/discarded/expired items
		if (item.Status == DLQStatusResolved || item.Status == DLQStatusDiscarded || item.Status == DLQStatusExpired) &&
			item.ResolvedAt != nil && item.ResolvedAt.Before(resolvedCutoff) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(q.items, id)
	}

	q.stats.CurrentSize = len(q.items)

	if len(toDelete) > 0 {
		log.Printf("[DLQ] Cleanup removed %d items", len(toDelete))
	}
}

func (q *DeadLetterQueue) evictOldestLocked() {
	var oldestId string
	var oldestTime time.Time

	for id, item := range q.items {
		// Skip items being processed
		if item.Status == DLQStatusProcessing {
			continue
		}

		if oldestId == "" || item.CreatedAt.Before(oldestTime) {
			oldestId = id
			oldestTime = item.CreatedAt
		}
	}

	if oldestId != "" {
		delete(q.items, oldestId)
		log.Printf("[DLQ] Evicted oldest item: id=%s", oldestId)
	}
}

// =============================================================================
// STATISTICS AND METRICS
// =============================================================================

// GetStats returns queue statistics
func (q *DeadLetterQueue) GetStats() DLQStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := q.stats
	stats.CurrentSize = len(q.items)

	// Find oldest item
	for _, item := range q.items {
		if stats.OldestItem.IsZero() || item.CreatedAt.Before(stats.OldestItem) {
			stats.OldestItem = item.CreatedAt
		}
	}

	return stats
}

// DLQMetrics provides detailed metrics
type DLQMetrics struct {
	Stats             DLQStats                    `json:"stats"`
	ByStatus          map[DLQItemStatus]int       `json:"byStatus"`
	ByProvider        map[string]int              `json:"byProvider"`
	ByFailureType     map[shared.FailureType]int  `json:"byFailureType"`
	ByOperationType   map[string]int              `json:"byOperationType"`
	PendingRetries    int                         `json:"pendingRetries"`
	ScheduledRetries  int                         `json:"scheduledRetries"`
}

// GetMetrics returns detailed queue metrics
func (q *DeadLetterQueue) GetMetrics() DLQMetrics {
	q.mu.RLock()
	defer q.mu.RUnlock()

	metrics := DLQMetrics{
		Stats:           q.stats,
		ByStatus:        make(map[DLQItemStatus]int),
		ByProvider:      make(map[string]int),
		ByFailureType:   make(map[shared.FailureType]int),
		ByOperationType: make(map[string]int),
	}

	metrics.Stats.CurrentSize = len(q.items)

	for _, item := range q.items {
		metrics.ByStatus[item.Status]++
		metrics.ByProvider[item.Provider]++
		metrics.ByFailureType[item.FailureType]++
		metrics.ByOperationType[item.OperationType]++

		if item.Status == DLQStatusPending {
			metrics.PendingRetries++
		}
		if item.Status == DLQStatusScheduled {
			metrics.ScheduledRetries++
		}
	}

	return metrics
}
