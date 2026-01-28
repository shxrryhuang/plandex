package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// =============================================================================
// ERROR REGISTRY - Persistent storage for error reports
// =============================================================================

// DefaultMaxErrors is the default maximum number of errors to store
const DefaultMaxErrors = 100

// DefaultErrorsFileName is the default file name for persisted errors
const DefaultErrorsFileName = "errors.json"

// StoredError wraps an ErrorReport with storage metadata
type StoredError struct {
	*ErrorReport

	// StoredAt is when the error was stored
	StoredAt time.Time `json:"storedAt"`

	// SessionId identifies the CLI session that recorded this error
	SessionId string `json:"sessionId"`

	// Resolved indicates if this error was resolved (e.g., after successful retry)
	Resolved bool `json:"resolved"`

	// ResolvedAt is when the error was resolved
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`

	// ResolvedBy describes how the error was resolved
	ResolvedBy string `json:"resolvedBy,omitempty"`

	// RetryCount is how many times retry was attempted
	RetryCount int `json:"retryCount"`
}

// ErrorFilter specifies criteria for filtering stored errors
type ErrorFilter struct {
	// Category filters by error category
	Category ErrorCategory `json:"category,omitempty"`

	// PlanId filters by plan ID
	PlanId string `json:"planId,omitempty"`

	// Since filters to errors after this time
	Since *time.Time `json:"since,omitempty"`

	// Until filters to errors before this time
	Until *time.Time `json:"until,omitempty"`

	// Resolved filters by resolved status (nil = all, true = resolved only, false = unresolved only)
	Resolved *bool `json:"resolved,omitempty"`

	// Limit limits the number of results
	Limit int `json:"limit,omitempty"`

	// Types filters by specific error types
	Types []string `json:"types,omitempty"`
}

// ErrorRegistry stores and manages error reports
type ErrorRegistry struct {
	errors      []*StoredError
	maxErrors   int
	mu          sync.RWMutex
	persistPath string
	sessionId   string
	dirty       bool // Tracks if changes need to be persisted
}

// NewErrorRegistry creates a new error registry
func NewErrorRegistry(persistDir string, maxErrors int, sessionId string) *ErrorRegistry {
	if maxErrors <= 0 {
		maxErrors = DefaultMaxErrors
	}

	persistPath := ""
	if persistDir != "" {
		persistPath = filepath.Join(persistDir, DefaultErrorsFileName)
	}

	return &ErrorRegistry{
		errors:      make([]*StoredError, 0, maxErrors),
		maxErrors:   maxErrors,
		persistPath: persistPath,
		sessionId:   sessionId,
		dirty:       false,
	}
}

// Store adds an error report to the registry
// Returns the error ID for later reference
func (r *ErrorRegistry) Store(report *ErrorReport) string {
	if report == nil {
		return ""
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stored := &StoredError{
		ErrorReport: report,
		StoredAt:    time.Now(),
		SessionId:   r.sessionId,
		Resolved:    false,
		RetryCount:  0,
	}

	// Ring buffer: remove oldest if at capacity
	if len(r.errors) >= r.maxErrors {
		r.errors = r.errors[1:]
	}

	r.errors = append(r.errors, stored)
	r.dirty = true

	return report.Id
}

// Get retrieves a stored error by ID
func (r *ErrorRegistry) Get(id string) *StoredError {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, err := range r.errors {
		if err.Id == id {
			return err
		}
	}
	return nil
}

// List returns errors matching the filter criteria
func (r *ErrorRegistry) List(filter ErrorFilter) []*StoredError {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*StoredError

	for _, err := range r.errors {
		if !matchesFilter(err, filter) {
			continue
		}
		results = append(results, err)
	}

	// Sort by StoredAt descending (most recent first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].StoredAt.After(results[j].StoredAt)
	})

	// Apply limit
	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results
}

// matchesFilter checks if an error matches the filter criteria
func matchesFilter(err *StoredError, filter ErrorFilter) bool {
	// Category filter
	if filter.Category != "" && err.RootCause.Category != filter.Category {
		return false
	}

	// Plan ID filter
	if filter.PlanId != "" && err.StepContext != nil && err.StepContext.PlanId != filter.PlanId {
		return false
	}

	// Time range filters
	if filter.Since != nil && err.StoredAt.Before(*filter.Since) {
		return false
	}
	if filter.Until != nil && err.StoredAt.After(*filter.Until) {
		return false
	}

	// Resolved filter
	if filter.Resolved != nil && err.Resolved != *filter.Resolved {
		return false
	}

	// Types filter
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if err.RootCause.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// MarkResolved marks an error as resolved
func (r *ErrorRegistry) MarkResolved(id string, resolvedBy string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, err := range r.errors {
		if err.Id == id {
			err.Resolved = true
			now := time.Now()
			err.ResolvedAt = &now
			err.ResolvedBy = resolvedBy
			r.dirty = true
			return true
		}
	}
	return false
}

// IncrementRetry increments the retry count for an error
func (r *ErrorRegistry) IncrementRetry(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, err := range r.errors {
		if err.Id == id {
			err.RetryCount++
			r.dirty = true
			return err.RetryCount
		}
	}
	return 0
}

// Clear removes all errors from the registry
func (r *ErrorRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.errors = make([]*StoredError, 0, r.maxErrors)
	r.dirty = true
}

// Count returns the number of stored errors
func (r *ErrorRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.errors)
}

// UnresolvedCount returns the number of unresolved errors
func (r *ErrorRegistry) UnresolvedCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, err := range r.errors {
		if !err.Resolved {
			count++
		}
	}
	return count
}

// =============================================================================
// PERSISTENCE
// =============================================================================

// Persist saves the registry to disk
func (r *ErrorRegistry) Persist() error {
	if r.persistPath == "" {
		return nil // Persistence not configured
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.dirty {
		return nil // No changes to persist
	}

	data, err := json.MarshalIndent(r.errors, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal errors: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(r.persistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write atomically using temp file
	tmpPath := r.persistPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, r.persistPath); err != nil {
		os.Remove(tmpPath) // Clean up
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	r.dirty = false
	return nil
}

// Load loads the registry from disk
func (r *ErrorRegistry) Load() error {
	if r.persistPath == "" {
		return nil // Persistence not configured
	}

	data, err := os.ReadFile(r.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet, that's OK
		}
		return fmt.Errorf("failed to read errors file: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var errors []*StoredError
	if err := json.Unmarshal(data, &errors); err != nil {
		return fmt.Errorf("failed to unmarshal errors: %w", err)
	}

	// Trim to max size if file had more
	if len(errors) > r.maxErrors {
		errors = errors[len(errors)-r.maxErrors:]
	}

	r.errors = errors
	r.dirty = false
	return nil
}

// =============================================================================
// EXPORT
// =============================================================================

// ExportFormat specifies the export format
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatText ExportFormat = "text"
)

// Export exports the registry in the specified format
func (r *ErrorRegistry) Export(format ExportFormat, sanitizeLevel SanitizeLevel) ([]byte, error) {
	r.mu.RLock()
	errors := make([]*StoredError, len(r.errors))
	copy(errors, r.errors)
	r.mu.RUnlock()

	// Sanitize errors if requested
	if sanitizeLevel != SanitizeLevelNone {
		for i, err := range errors {
			sanitized := SanitizeError(err.ErrorReport, sanitizeLevel)
			errors[i] = &StoredError{
				ErrorReport: sanitized,
				StoredAt:    err.StoredAt,
				SessionId:   err.SessionId,
				Resolved:    err.Resolved,
				ResolvedAt:  err.ResolvedAt,
				ResolvedBy:  err.ResolvedBy,
				RetryCount:  err.RetryCount,
			}
		}
	}

	switch format {
	case ExportFormatJSON:
		return json.MarshalIndent(errors, "", "  ")
	case ExportFormatText:
		return r.exportText(errors), nil
	default:
		return nil, fmt.Errorf("unknown export format: %s", format)
	}
}

func (r *ErrorRegistry) exportText(errors []*StoredError) []byte {
	var result string

	result += "═══════════════════════════════════════════════════════════════════\n"
	result += fmt.Sprintf("                    ERROR EXPORT (%d errors)\n", len(errors))
	result += "═══════════════════════════════════════════════════════════════════\n\n"

	for i, err := range errors {
		result += fmt.Sprintf("[%d] %s | %s\n", i+1, err.StoredAt.Format("2006-01-02 15:04:05"), err.Id)
		result += fmt.Sprintf("    Category: %s\n", err.RootCause.Category)
		result += fmt.Sprintf("    Type:     %s\n", err.RootCause.Type)
		result += fmt.Sprintf("    Message:  %s\n", err.RootCause.Message)

		if err.StepContext != nil && err.StepContext.PlanId != "" {
			result += fmt.Sprintf("    Plan:     %s", err.StepContext.PlanId)
			if err.StepContext.Branch != "" {
				result += fmt.Sprintf(" (branch: %s)", err.StepContext.Branch)
			}
			result += "\n"
		}

		if err.Resolved {
			result += fmt.Sprintf("    Resolved: Yes (%s)", err.ResolvedBy)
			if err.ResolvedAt != nil {
				result += fmt.Sprintf(" at %s", err.ResolvedAt.Format("2006-01-02 15:04:05"))
			}
			result += "\n"
		} else {
			result += "    Resolved: No\n"
		}

		if err.RetryCount > 0 {
			result += fmt.Sprintf("    Retries:  %d\n", err.RetryCount)
		}

		result += "\n"
	}

	return []byte(result)
}

// =============================================================================
// SUMMARY STATISTICS
// =============================================================================

// RegistryStats provides summary statistics
type RegistryStats struct {
	TotalErrors      int            `json:"totalErrors"`
	UnresolvedErrors int            `json:"unresolvedErrors"`
	ResolvedErrors   int            `json:"resolvedErrors"`
	ErrorsByCategory map[string]int `json:"errorsByCategory"`
	ErrorsByType     map[string]int `json:"errorsByType"`
	OldestError      *time.Time     `json:"oldestError,omitempty"`
	NewestError      *time.Time     `json:"newestError,omitempty"`
}

// Stats returns summary statistics about the registry
func (r *ErrorRegistry) Stats() RegistryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := RegistryStats{
		ErrorsByCategory: make(map[string]int),
		ErrorsByType:     make(map[string]int),
	}

	for _, err := range r.errors {
		stats.TotalErrors++

		if err.Resolved {
			stats.ResolvedErrors++
		} else {
			stats.UnresolvedErrors++
		}

		cat := string(err.RootCause.Category)
		stats.ErrorsByCategory[cat]++

		stats.ErrorsByType[err.RootCause.Type]++

		if stats.OldestError == nil || err.StoredAt.Before(*stats.OldestError) {
			t := err.StoredAt
			stats.OldestError = &t
		}
		if stats.NewestError == nil || err.StoredAt.After(*stats.NewestError) {
			t := err.StoredAt
			stats.NewestError = &t
		}
	}

	return stats
}

// =============================================================================
// GLOBAL REGISTRY
// =============================================================================

// GlobalErrorRegistry is the default error registry instance
var GlobalErrorRegistry *ErrorRegistry

// InitGlobalRegistry initializes the global error registry
func InitGlobalRegistry(persistDir string, sessionId string) error {
	GlobalErrorRegistry = NewErrorRegistry(persistDir, DefaultMaxErrors, sessionId)
	return GlobalErrorRegistry.Load()
}

// StoreError is a convenience function to store an error in the global registry
func StoreError(report *ErrorReport) string {
	if GlobalErrorRegistry == nil {
		return ""
	}
	return GlobalErrorRegistry.Store(report)
}

// StoreWithContext stores an error report enriched with retry context data.
// It records total attempts, whether the error is unrecoverable, and the
// operation type as tags on the ErrorReport before persisting.
func StoreWithContext(report *ErrorReport, retryCtx *RetryContext) string {
	if report == nil {
		return ""
	}

	// Enrich with retry context metadata
	if retryCtx != nil {
		report.Tags = append(report.Tags,
			fmt.Sprintf("retry_attempts:%d", retryCtx.TotalAttempts()),
			fmt.Sprintf("operation:%s", retryCtx.OperationType),
			fmt.Sprintf("safety:%s", retryCtx.Safety),
		)
		if retryCtx.Unrecoverable != nil {
			report.Tags = append(report.Tags, "unrecoverable", string(retryCtx.Unrecoverable.Reason))
		}
	}

	return StoreError(report)
}

// GetError is a convenience function to get an error from the global registry
func GetError(id string) *StoredError {
	if GlobalErrorRegistry == nil {
		return nil
	}
	return GlobalErrorRegistry.Get(id)
}

// ListErrors is a convenience function to list errors from the global registry
func ListErrors(filter ErrorFilter) []*StoredError {
	if GlobalErrorRegistry == nil {
		return nil
	}
	return GlobalErrorRegistry.List(filter)
}
