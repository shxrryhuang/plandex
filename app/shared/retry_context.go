package shared

import (
	"fmt"
	"time"
)

// =============================================================================
// RETRY CONTEXT — structured state carried through the retry loop
// =============================================================================
//
// RetryContext is the single object that flows through every iteration of
// withStreamingRetries.  It accumulates attempt history, links back to the
// run journal and error registry, and exposes helpers so the retry loop never
// has to reach for global mutable state.
//
// =============================================================================

// RetryAttempt records what happened on a single execution attempt.
type RetryAttempt struct {
	// AttemptNumber is 1-based (first try = 1).
	AttemptNumber int `json:"attemptNumber"`

	// StartedAt is when the attempt began.
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when the attempt finished (success or failure).
	CompletedAt time.Time `json:"completedAt"`

	// DurationMs is CompletedAt - StartedAt in milliseconds.
	DurationMs int64 `json:"durationMs"`

	// Succeeded indicates whether this attempt produced a usable result.
	Succeeded bool `json:"succeeded"`

	// Error is the classified error, if any.
	Error *ModelError `json:"error,omitempty"`

	// FailureType is the resolved failure category for this attempt.
	FailureType FailureType `json:"failureType,omitempty"`

	// Strategy is the RetryStrategy that was applied after this failure.
	Strategy *RetryStrategy `json:"strategy,omitempty"`

	// ComputedDelayMs is the actual delay applied before the next attempt.
	ComputedDelayMs int64 `json:"computedDelayMs,omitempty"`

	// UsedFallback indicates this attempt used a fallback provider/model.
	UsedFallback bool `json:"usedFallback,omitempty"`

	// FallbackType describes which kind of fallback was used.
	FallbackType FallbackType `json:"fallbackType,omitempty"`
}

// RetryContext carries all retry state through the loop.
type RetryContext struct {
	// Config is the active retry policy.
	Config *RetryConfig `json:"config"`

	// OperationSafety classifies whether this operation may be safely retried.
	Safety OperationSafety `json:"safety"`

	// OperationType is a string label used for safety classification.
	OperationType string `json:"operationType"`

	// Attempts is the ordered history of every execution attempt.
	Attempts []RetryAttempt `json:"attempts"`

	// JournalEntryId links this retry context to a RunJournal entry (optional).
	JournalEntryId string `json:"journalEntryId,omitempty"`

	// ErrorReportId links to the ErrorRegistry entry created on final failure
	// (populated only after all retries are exhausted).
	ErrorReportId string `json:"errorReportId,omitempty"`

	// FinalError holds the ErrorReport generated when all retries fail or an
	// unrecoverable condition is detected.
	FinalError *ErrorReport `json:"finalError,omitempty"`

	// Unrecoverable is set if DetectUnrecoverableCondition fired.
	Unrecoverable *UnrecoverableError `json:"unrecoverable,omitempty"`
}

// NewRetryContext creates a RetryContext with sensible defaults.
func NewRetryContext(operationType string, config *RetryConfig) *RetryContext {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &RetryContext{
		Config:        config,
		Safety:        ClassifyOperation(operationType),
		OperationType: operationType,
		Attempts:      make([]RetryAttempt, 0, 4),
	}
}

// RecordAttemptStart begins a new attempt record.
func (rc *RetryContext) RecordAttemptStart() int {
	attempt := RetryAttempt{
		AttemptNumber: len(rc.Attempts) + 1,
		StartedAt:     time.Now(),
	}
	rc.Attempts = append(rc.Attempts, attempt)
	return len(rc.Attempts) - 1 // index of new attempt
}

// RecordAttemptSuccess marks the most recent attempt as successful.
func (rc *RetryContext) RecordAttemptSuccess(idx int) {
	if idx < 0 || idx >= len(rc.Attempts) {
		return
	}
	now := time.Now()
	rc.Attempts[idx].Succeeded = true
	rc.Attempts[idx].CompletedAt = now
	rc.Attempts[idx].DurationMs = now.Sub(rc.Attempts[idx].StartedAt).Milliseconds()
}

// RecordAttemptFailure marks the most recent attempt as failed with the
// given error details and the strategy that will be applied next.
func (rc *RetryContext) RecordAttemptFailure(idx int, modelErr *ModelError, strategy *RetryStrategy, delayMs int64, usedFallback bool, fallbackType FallbackType) {
	if idx < 0 || idx >= len(rc.Attempts) {
		return
	}
	now := time.Now()
	rc.Attempts[idx].Succeeded = false
	rc.Attempts[idx].CompletedAt = now
	rc.Attempts[idx].DurationMs = now.Sub(rc.Attempts[idx].StartedAt).Milliseconds()
	rc.Attempts[idx].Error = modelErr
	if modelErr != nil {
		rc.Attempts[idx].FailureType = modelErr.ProviderFailureType
	}
	rc.Attempts[idx].Strategy = strategy
	rc.Attempts[idx].ComputedDelayMs = delayMs
	rc.Attempts[idx].UsedFallback = usedFallback
	rc.Attempts[idx].FallbackType = fallbackType
}

// TotalAttempts returns the number of attempts recorded so far.
func (rc *RetryContext) TotalAttempts() int {
	return len(rc.Attempts)
}

// LastAttempt returns the most recent attempt, or nil if none.
func (rc *RetryContext) LastAttempt() *RetryAttempt {
	if len(rc.Attempts) == 0 {
		return nil
	}
	return &rc.Attempts[len(rc.Attempts)-1]
}

// CanRetry checks whether another retry is allowed given the config and
// current attempt count.  It considers both global caps and per-type limits.
func (rc *RetryContext) CanRetry(failureType FailureType) bool {
	if !IsOperationSafe(rc.Safety, rc.Config) {
		return false
	}

	strategy := rc.Config.GetStrategy(failureType)
	if !strategy.ShouldRetry {
		return false
	}

	effectiveMax := rc.Config.EffectiveMaxAttempts(failureType)
	// MaxAttempts means total attempts including the initial one.
	// If we've already attempted MaxAttempts times, no more retries.
	return rc.TotalAttempts() < effectiveMax
}

// FinalizeWithError creates an ErrorReport, optionally detects unrecoverable
// conditions, and stores the result in the global ErrorRegistry.
// Returns the ErrorReport for logging or surfacing to the user.
func (rc *RetryContext) FinalizeWithError(failure *ProviderFailure, stepCtx *StepContext) *ErrorReport {
	report := ErrorReportFromProviderFailure(failure, stepCtx)

	// Attach retry history metadata as tags
	report.Tags = append(report.Tags,
		fmt.Sprintf("attempts:%d", rc.TotalAttempts()),
		fmt.Sprintf("operation:%s", rc.OperationType),
		fmt.Sprintf("safety:%s", rc.Safety),
	)

	// Check for unrecoverable condition
	unrecov := DetectUnrecoverableCondition(report)
	if unrecov != nil {
		rc.Unrecoverable = unrecov
		report.Tags = append(report.Tags, "unrecoverable")
	}

	rc.FinalError = report

	// Persist to global registry
	rc.ErrorReportId = StoreError(report)

	return report
}

// Summary returns a compact human-readable summary of retry activity.
func (rc *RetryContext) Summary() string {
	total := rc.TotalAttempts()
	succeeded := 0
	for _, a := range rc.Attempts {
		if a.Succeeded {
			succeeded++
		}
	}

	if succeeded > 0 {
		return fmt.Sprintf("succeeded after %d attempt(s)", total)
	}

	if rc.Unrecoverable != nil {
		return fmt.Sprintf("failed after %d attempt(s) — unrecoverable: %s", total, rc.Unrecoverable.Reason)
	}

	return fmt.Sprintf("failed after %d attempt(s) — retries exhausted", total)
}
