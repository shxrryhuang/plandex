package shared

import (
	"math"
	"math/rand"
	"time"
)

// =============================================================================
// RETRY POLICY SYSTEM
// =============================================================================
//
// This file defines configurable retry policies with exponential backoff
// and jitter for handling provider failures gracefully.
//
// =============================================================================

// RetryPolicy defines how retries should be executed for a specific failure type
type RetryPolicy struct {
	// Name identifies this policy
	Name string `json:"name"`

	// Description explains when this policy should be used
	Description string `json:"description,omitempty"`

	// MaxAttempts is the maximum number of retry attempts (including the first)
	MaxAttempts int `json:"maxAttempts"`

	// MaxTotalTime is the maximum total duration for all retry attempts
	MaxTotalTime time.Duration `json:"maxTotalTime"`

	// InitialDelay is the base delay before the first retry
	InitialDelay time.Duration `json:"initialDelay"`

	// MaxDelay caps the delay regardless of backoff calculation
	MaxDelay time.Duration `json:"maxDelay"`

	// Multiplier is the exponential backoff multiplier (e.g., 2.0 for doubling)
	Multiplier float64 `json:"multiplier"`

	// JitterEnabled adds randomization to prevent thundering herd
	JitterEnabled bool `json:"jitterEnabled"`

	// JitterFactor is the percentage of delay to randomize (0.0-1.0)
	JitterFactor float64 `json:"jitterFactor"`

	// RespectRetryAfter uses the Retry-After header value when available
	RespectRetryAfter bool `json:"respectRetryAfter"`

	// TryFallbackFirst attempts fallback before exhausting retries
	TryFallbackFirst bool `json:"tryFallbackFirst"`

	// AbortOnNonRetryable immediately returns on non-retryable errors
	AbortOnNonRetryable bool `json:"abortOnNonRetryable"`

	// FallbackOnExhaust tries fallback when retries are exhausted
	FallbackOnExhaust bool `json:"fallbackOnExhaust"`
}

// =============================================================================
// PRE-DEFINED POLICIES
// =============================================================================

// PolicyRateLimit handles rate limiting (HTTP 429)
// Strategy: Respect Retry-After header, use exponential backoff with generous max
var PolicyRateLimit = RetryPolicy{
	Name:              "rate_limit",
	Description:       "Rate limiting - backs off based on Retry-After header",
	MaxAttempts:       5,
	MaxTotalTime:      5 * time.Minute,
	InitialDelay:      1 * time.Second,
	MaxDelay:          60 * time.Second,
	Multiplier:        2.0,
	JitterEnabled:     true,
	JitterFactor:      0.2,
	RespectRetryAfter: true,
	TryFallbackFirst:  false,
	FallbackOnExhaust: true,
}

// PolicyOverloaded handles server overload (HTTP 503, 529)
// Strategy: Aggressive backoff, try fallback early since overload may persist
var PolicyOverloaded = RetryPolicy{
	Name:              "overloaded",
	Description:       "Server overloaded - aggressive backoff, early fallback",
	MaxAttempts:       5,
	MaxTotalTime:      3 * time.Minute,
	InitialDelay:      5 * time.Second,
	MaxDelay:          120 * time.Second,
	Multiplier:        2.0,
	JitterEnabled:     true,
	JitterFactor:      0.3,
	RespectRetryAfter: true,
	TryFallbackFirst:  true,
	FallbackOnExhaust: true,
}

// PolicyServerError handles general server errors (HTTP 500, 502, 504)
// Strategy: Quick retries with fallback, server errors are often transient
var PolicyServerError = RetryPolicy{
	Name:              "server_error",
	Description:       "Server error - quick retries, fallback on persist",
	MaxAttempts:       3,
	MaxTotalTime:      2 * time.Minute,
	InitialDelay:      1 * time.Second,
	MaxDelay:          30 * time.Second,
	Multiplier:        2.0,
	JitterEnabled:     true,
	JitterFactor:      0.2,
	RespectRetryAfter: false,
	TryFallbackFirst:  true,
	FallbackOnExhaust: true,
}

// PolicyTimeout handles request timeouts
// Strategy: Immediate retry with possibly extended timeout
var PolicyTimeout = RetryPolicy{
	Name:              "timeout",
	Description:       "Request timeout - immediate retry",
	MaxAttempts:       2,
	MaxTotalTime:      1 * time.Minute,
	InitialDelay:      0,
	MaxDelay:          0,
	Multiplier:        1.0,
	JitterEnabled:     false,
	RespectRetryAfter: false,
	TryFallbackFirst:  false,
	FallbackOnExhaust: true,
}

// PolicyConnectionError handles network connectivity issues
// Strategy: Brief delay to allow network recovery
var PolicyConnectionError = RetryPolicy{
	Name:              "connection_error",
	Description:       "Network connectivity - brief delay for recovery",
	MaxAttempts:       3,
	MaxTotalTime:      30 * time.Second,
	InitialDelay:      500 * time.Millisecond,
	MaxDelay:          5 * time.Second,
	Multiplier:        2.0,
	JitterEnabled:     true,
	JitterFactor:      0.1,
	RespectRetryAfter: false,
	TryFallbackFirst:  false,
	FallbackOnExhaust: true,
}

// PolicyStreamInterrupted handles interrupted streaming responses
// Strategy: Quick retry to resume streaming
var PolicyStreamInterrupted = RetryPolicy{
	Name:              "stream_interrupted",
	Description:       "Stream interrupted - quick retry to resume",
	MaxAttempts:       2,
	MaxTotalTime:      30 * time.Second,
	InitialDelay:      1 * time.Second,
	MaxDelay:          5 * time.Second,
	Multiplier:        1.5,
	JitterEnabled:     false,
	RespectRetryAfter: false,
	TryFallbackFirst:  false,
	FallbackOnExhaust: true,
}

// PolicyCacheError handles cache-related errors
// Strategy: Immediate retry without cache parameters
var PolicyCacheError = RetryPolicy{
	Name:              "cache_error",
	Description:       "Cache error - retry without cache",
	MaxAttempts:       2,
	MaxTotalTime:      30 * time.Second,
	InitialDelay:      0,
	MaxDelay:          0,
	Multiplier:        1.0,
	JitterEnabled:     false,
	RespectRetryAfter: false,
	TryFallbackFirst:  false,
	FallbackOnExhaust: false,
}

// =============================================================================
// POLICY LOOKUP
// =============================================================================

// policyMap maps failure types to their corresponding retry policies
var policyMap = map[FailureType]*RetryPolicy{
	FailureTypeRateLimit:         &PolicyRateLimit,
	FailureTypeOverloaded:        &PolicyOverloaded,
	FailureTypeServerError:       &PolicyServerError,
	FailureTypeTimeout:           &PolicyTimeout,
	FailureTypeConnectionError:   &PolicyConnectionError,
	FailureTypeStreamInterrupted: &PolicyStreamInterrupted,
	FailureTypeCacheError:        &PolicyCacheError,
}

// GetPolicyForFailure returns the appropriate retry policy for a failure type
// Returns nil for non-retryable failure types
func GetPolicyForFailure(failureType FailureType) *RetryPolicy {
	policy, exists := policyMap[failureType]
	if !exists {
		return nil
	}
	// Return a copy to prevent mutation
	policyCopy := *policy
	return &policyCopy
}

// GetDefaultPolicy returns the default retry policy for unknown retryable errors
func GetDefaultPolicy() *RetryPolicy {
	return &RetryPolicy{
		Name:              "default",
		Description:       "Default retry policy for unknown errors",
		MaxAttempts:       3,
		MaxTotalTime:      2 * time.Minute,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		Multiplier:        2.0,
		JitterEnabled:     true,
		JitterFactor:      0.2,
		RespectRetryAfter: true,
		TryFallbackFirst:  false,
		FallbackOnExhaust: true,
	}
}

// =============================================================================
// DELAY CALCULATION
// =============================================================================

// CalculateDelay computes the delay for a given retry attempt
// Parameters:
//   - attempt: the current attempt number (1-indexed, so first retry is attempt 2)
//   - retryAfterHint: optional Retry-After value from the server
//
// Returns the duration to wait before the next attempt
func (p *RetryPolicy) CalculateDelay(attempt int, retryAfterHint time.Duration) time.Duration {
	// Respect Retry-After if configured and provided
	if p.RespectRetryAfter && retryAfterHint > 0 {
		delay := retryAfterHint
		// Cap at MaxDelay if configured
		if p.MaxDelay > 0 && delay > p.MaxDelay {
			delay = p.MaxDelay
		}
		// Add small jitter to avoid thundering herd
		if p.JitterEnabled {
			jitter := time.Duration(float64(delay) * p.JitterFactor * rand.Float64())
			delay += jitter
		}
		return delay
	}

	// Calculate exponential backoff: initialDelay * multiplier^(attempt-1)
	// For attempt 1 (first retry), this gives initialDelay
	// For attempt 2 (second retry), this gives initialDelay * multiplier
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt-1))

	// Apply max delay cap
	if p.MaxDelay > 0 && time.Duration(delay) > p.MaxDelay {
		delay = float64(p.MaxDelay)
	}

	// Apply jitter: randomize by +/- jitterFactor
	if p.JitterEnabled && delay > 0 {
		jitterRange := delay * p.JitterFactor
		// Random value between -jitterRange and +jitterRange
		jitter := (rand.Float64()*2 - 1) * jitterRange
		delay += jitter
		// Ensure delay doesn't go negative
		if delay < 0 {
			delay = 0
		}
	}

	return time.Duration(delay)
}

// =============================================================================
// RETRY STATE TRACKING
// =============================================================================

// RetryState tracks the state of a retry sequence
type RetryState struct {
	// RequestId uniquely identifies this request sequence
	RequestId string `json:"requestId"`

	// IdempotencyKey for preventing duplicate side effects
	IdempotencyKey string `json:"idempotencyKey"`

	// Tracking
	TotalAttempts    int           `json:"totalAttempts"`
	FallbackAttempts int           `json:"fallbackAttempts"`
	TotalDuration    time.Duration `json:"totalDuration"`
	StartTime        time.Time     `json:"startTime"`

	// Current state
	LastError       error            `json:"-"`
	LastFailure     *ProviderFailure `json:"lastFailure,omitempty"`
	CurrentProvider string           `json:"currentProvider"`
	UsedFallback    bool             `json:"usedFallback"`

	// Policy being used
	Policy *RetryPolicy `json:"policy,omitempty"`

	// History of attempts for debugging
	AttemptHistory []RetryStateAttempt `json:"attemptHistory,omitempty"`
}

// RetryStateAttempt records details of a single retry attempt within a RetryState sequence.
// This is distinct from RetryAttempt (in retry_context.go) which is used by RetryContext.
type RetryStateAttempt struct {
	AttemptNumber int          `json:"attemptNumber"`
	Timestamp     time.Time    `json:"timestamp"`
	Provider      string       `json:"provider"`
	DelayUsed     time.Duration `json:"delayUsed"`
	Error         string       `json:"error,omitempty"`
	FailureType   FailureType  `json:"failureType,omitempty"`
	PolicyUsed    string       `json:"policyUsed,omitempty"`
	WillRetry     bool         `json:"willRetry"`
	FallbackUsed  bool         `json:"fallbackUsed"`
	FallbackType  FallbackType `json:"fallbackType,omitempty"`
}

// NewRetryState creates a new retry state for tracking a request sequence
func NewRetryState(requestId, idempotencyKey, provider string) *RetryState {
	return &RetryState{
		RequestId:       requestId,
		IdempotencyKey:  idempotencyKey,
		StartTime:       time.Now(),
		CurrentProvider: provider,
		AttemptHistory:  make([]RetryStateAttempt, 0),
	}
}

// RecordAttempt records a retry attempt for debugging and auditing
func (s *RetryState) RecordAttempt(delay time.Duration, failure *ProviderFailure, willRetry bool, fallbackType FallbackType) {
	attempt := RetryStateAttempt{
		AttemptNumber: s.TotalAttempts,
		Timestamp:     time.Now(),
		Provider:      s.CurrentProvider,
		DelayUsed:     delay,
		WillRetry:     willRetry,
		FallbackUsed:  fallbackType != "",
		FallbackType:  fallbackType,
	}

	if failure != nil {
		attempt.FailureType = failure.Type
		attempt.Error = failure.Message
	}

	if s.Policy != nil {
		attempt.PolicyUsed = s.Policy.Name
	}

	s.AttemptHistory = append(s.AttemptHistory, attempt)
}

// ShouldRetry determines if another retry should be attempted
func (s *RetryState) ShouldRetry(policy *RetryPolicy) bool {
	if policy == nil {
		return false
	}

	// Check max attempts
	if s.TotalAttempts >= policy.MaxAttempts {
		return false
	}

	// Check max total time
	if policy.MaxTotalTime > 0 && time.Since(s.StartTime) > policy.MaxTotalTime {
		return false
	}

	return true
}

// =============================================================================
// RETRY RESULT
// =============================================================================

// RetryResult captures the final outcome of a retry sequence
type RetryResult struct {
	// Success indicates if the operation eventually succeeded
	Success bool `json:"success"`

	// FinalError is the last error encountered (if any)
	FinalError error `json:"-"`

	// FinalErrorMessage is the string representation for serialization
	FinalErrorMessage string `json:"finalError,omitempty"`

	// TotalAttempts is how many attempts were made
	TotalAttempts int `json:"totalAttempts"`

	// UsedFallback indicates if a fallback was used
	UsedFallback bool `json:"usedFallback"`

	// FallbackType indicates which type of fallback was used
	FallbackType FallbackType `json:"fallbackType,omitempty"`

	// Duration is the total time spent in retry logic
	Duration time.Duration `json:"duration"`

	// ProviderUsed is the provider that handled the final attempt
	ProviderUsed string `json:"providerUsed"`

	// PolicyUsed is the retry policy that was applied
	PolicyUsed string `json:"policyUsed,omitempty"`

	// AttemptHistory contains details of each attempt
	AttemptHistory []RetryStateAttempt `json:"attemptHistory,omitempty"`
}

// NewRetryResult creates a RetryResult from a RetryState
func NewRetryResult(state *RetryState, success bool, finalError error) *RetryResult {
	result := &RetryResult{
		Success:        success,
		FinalError:     finalError,
		TotalAttempts:  state.TotalAttempts,
		UsedFallback:   state.UsedFallback,
		Duration:       time.Since(state.StartTime),
		ProviderUsed:   state.CurrentProvider,
		AttemptHistory: state.AttemptHistory,
	}

	if finalError != nil {
		result.FinalErrorMessage = finalError.Error()
	}

	if state.Policy != nil {
		result.PolicyUsed = state.Policy.Name
	}

	// Determine fallback type from history
	for _, attempt := range state.AttemptHistory {
		if attempt.FallbackUsed {
			result.FallbackType = attempt.FallbackType
			break
		}
	}

	return result
}
