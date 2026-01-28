package shared

import (
	"testing"
	"time"
)

func TestNewRetryContext(t *testing.T) {
	rc := NewRetryContext("model_request", nil)
	if rc == nil {
		t.Fatal("NewRetryContext returned nil")
	}
	if rc.Safety != OperationSafe {
		t.Errorf("model_request should be OperationSafe, got %s", rc.Safety)
	}
	if rc.Config == nil {
		t.Error("Config should default to DefaultRetryConfig when nil passed")
	}
	if rc.TotalAttempts() != 0 {
		t.Errorf("expected 0 initial attempts, got %d", rc.TotalAttempts())
	}
}

func TestNewRetryContext_IrreversibleOp(t *testing.T) {
	rc := NewRetryContext("shell_exec", DefaultRetryConfig())
	if rc.Safety != OperationIrreversible {
		t.Errorf("shell_exec should be OperationIrreversible, got %s", rc.Safety)
	}
}

func TestRecordAttemptStart_Success_Failure(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())

	// First attempt
	idx := rc.RecordAttemptStart()
	if idx != 0 {
		t.Errorf("first attempt index should be 0, got %d", idx)
	}
	if rc.TotalAttempts() != 1 {
		t.Errorf("expected 1 attempt after start, got %d", rc.TotalAttempts())
	}

	// Record success
	rc.RecordAttemptSuccess(idx)
	attempt := rc.LastAttempt()
	if !attempt.Succeeded {
		t.Error("attempt should be marked succeeded")
	}
	if attempt.DurationMs < 0 {
		t.Error("duration should be non-negative")
	}

	// Second attempt — failure
	idx2 := rc.RecordAttemptStart()
	if idx2 != 1 {
		t.Errorf("second attempt index should be 1, got %d", idx2)
	}

	modelErr := &ModelError{
		Kind:                ErrRateLimited,
		Retriable:           true,
		RetryAfterSeconds:   5,
		ProviderFailureType: FailureTypeRateLimit,
	}
	strategy := GetRetryStrategy(FailureTypeRateLimit)

	rc.RecordAttemptFailure(idx2, modelErr, &strategy, 5500, false, "")
	last := rc.LastAttempt()
	if last.Succeeded {
		t.Error("second attempt should not be succeeded")
	}
	if last.FailureType != FailureTypeRateLimit {
		t.Errorf("expected FailureType=rate_limit, got %s", last.FailureType)
	}
	if last.ComputedDelayMs != 5500 {
		t.Errorf("expected delay 5500ms, got %d", last.ComputedDelayMs)
	}
}

func TestCanRetry(t *testing.T) {
	cfg := &RetryConfig{
		MaxTotalAttempts: 3,
	}
	rc := NewRetryContext("model_request", cfg)

	// No attempts yet — can retry
	if !rc.CanRetry(FailureTypeRateLimit) {
		t.Error("should be able to retry with 0 attempts and cap of 3")
	}

	// Add 2 attempts
	rc.RecordAttemptStart()
	rc.RecordAttemptStart()
	if !rc.CanRetry(FailureTypeRateLimit) {
		t.Error("should be able to retry with 2 attempts and cap of 3")
	}

	// Add 3rd — now at cap
	rc.RecordAttemptStart()
	if rc.CanRetry(FailureTypeRateLimit) {
		t.Error("should NOT be able to retry with 3 attempts and cap of 3")
	}
}

func TestCanRetry_IrreversibleBlocked(t *testing.T) {
	cfg := &RetryConfig{
		RetryIrreversible: false,
	}
	rc := NewRetryContext("shell_exec", cfg)

	if rc.CanRetry(FailureTypeServerError) {
		t.Error("irreversible operation should not be retryable when config blocks it")
	}
}

func TestCanRetry_NonRetryableType(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())

	if rc.CanRetry(FailureTypeAuthInvalid) {
		t.Error("auth_invalid is non-retryable; CanRetry should return false")
	}
}

func TestSummary(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())

	// No attempts
	if s := rc.Summary(); s != "failed after 0 attempt(s) — retries exhausted" {
		t.Errorf("unexpected summary with 0 attempts: %s", s)
	}

	// One successful attempt
	idx := rc.RecordAttemptStart()
	rc.RecordAttemptSuccess(idx)
	if s := rc.Summary(); s != "succeeded after 1 attempt(s)" {
		t.Errorf("unexpected summary after success: %s", s)
	}
}

func TestSummary_Unrecoverable(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())
	rc.RecordAttemptStart()
	rc.Unrecoverable = &UnrecoverableError{Reason: UnrecoverableAuthInvalid}

	s := rc.Summary()
	if s != "failed after 1 attempt(s) — unrecoverable: auth_invalid" {
		t.Errorf("unexpected unrecoverable summary: %s", s)
	}
}

func TestRecordAttemptStart_OutOfBounds(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())

	// Out-of-bounds index should be a no-op (not panic)
	rc.RecordAttemptSuccess(99)
	rc.RecordAttemptFailure(-1, nil, nil, 0, false, "")

	if rc.TotalAttempts() != 0 {
		t.Errorf("out-of-bounds operations should not add attempts, got %d", rc.TotalAttempts())
	}
}

func TestLastAttempt_Empty(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())
	if rc.LastAttempt() != nil {
		t.Error("LastAttempt on empty context should return nil")
	}
}

func TestAttemptNumbering(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())

	for i := 0; i < 3; i++ {
		idx := rc.RecordAttemptStart()
		if rc.Attempts[idx].AttemptNumber != i+1 {
			t.Errorf("attempt %d should be numbered %d, got %d", idx, i+1, rc.Attempts[idx].AttemptNumber)
		}
	}
}

func TestRecordAttemptFailure_WithFallback(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())
	idx := rc.RecordAttemptStart()

	modelErr := &ModelError{
		Kind:                ErrOther,
		Retriable:           true,
		ProviderFailureType: FailureTypeProviderUnavailable,
	}
	strategy := GetRetryStrategy(FailureTypeProviderUnavailable)

	rc.RecordAttemptFailure(idx, modelErr, &strategy, 2000, true, FallbackTypeProvider)

	a := rc.Attempts[idx]
	if !a.UsedFallback {
		t.Error("should record UsedFallback=true")
	}
	if a.FallbackType != FallbackTypeProvider {
		t.Errorf("expected FallbackTypeProvider, got %s", a.FallbackType)
	}
}

// Verify that timing fields are populated.
func TestAttemptTiming(t *testing.T) {
	rc := NewRetryContext("model_request", DefaultRetryConfig())
	idx := rc.RecordAttemptStart()

	time.Sleep(5 * time.Millisecond) // brief pause for measurable duration

	rc.RecordAttemptSuccess(idx)

	a := rc.Attempts[idx]
	if a.DurationMs < 1 {
		t.Errorf("expected positive duration, got %dms", a.DurationMs)
	}
	if a.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
}
