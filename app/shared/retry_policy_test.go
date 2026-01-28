package shared

import (
	"testing"
	"time"
)

func TestCalculateDelay_ExponentialBackoff(t *testing.T) {
	policy := &RetryPolicy{
		Name:          "test",
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		Multiplier:    2.0,
		JitterEnabled: false,
	}

	// Attempt 1: should be InitialDelay
	delay1 := policy.CalculateDelay(1, 0)
	if delay1 != 1*time.Second {
		t.Errorf("Attempt 1: delay = %v, want 1s", delay1)
	}

	// Attempt 2: should be InitialDelay * Multiplier = 2s
	delay2 := policy.CalculateDelay(2, 0)
	if delay2 != 2*time.Second {
		t.Errorf("Attempt 2: delay = %v, want 2s", delay2)
	}

	// Attempt 3: should be 4s
	delay3 := policy.CalculateDelay(3, 0)
	if delay3 != 4*time.Second {
		t.Errorf("Attempt 3: delay = %v, want 4s", delay3)
	}

	// Attempt 6: would be 32s but capped at MaxDelay (30s)
	delay6 := policy.CalculateDelay(6, 0)
	if delay6 != 30*time.Second {
		t.Errorf("Attempt 6: delay = %v, want 30s (capped)", delay6)
	}
}

func TestCalculateDelay_WithJitter(t *testing.T) {
	policy := &RetryPolicy{
		Name:          "test",
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		Multiplier:    2.0,
		JitterEnabled: true,
		JitterFactor:  0.2,
	}

	// Run multiple times and check that jitter adds variation
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = policy.CalculateDelay(2, 0)
	}

	// Base delay for attempt 2 is 2s, jitter +/- 20% = 1.6s to 2.4s
	for _, d := range delays {
		if d < 1600*time.Millisecond || d > 2400*time.Millisecond {
			t.Errorf("Delay %v outside expected jitter range [1.6s, 2.4s]", d)
		}
	}

	// Check that not all delays are identical (jitter is working)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("All delays are identical - jitter may not be working")
	}
}

func TestCalculateDelay_RetryAfterHint(t *testing.T) {
	policy := &RetryPolicy{
		Name:              "test",
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		Multiplier:        2.0,
		JitterEnabled:     false,
		RespectRetryAfter: true,
	}

	// When RetryAfter hint is provided, it should be used
	delay := policy.CalculateDelay(1, 10*time.Second)
	if delay != 10*time.Second {
		t.Errorf("With RetryAfter hint: delay = %v, want 10s", delay)
	}

	// RetryAfter should be capped at MaxDelay
	delay = policy.CalculateDelay(1, 60*time.Second)
	if delay != 30*time.Second {
		t.Errorf("With RetryAfter > MaxDelay: delay = %v, want 30s", delay)
	}
}

func TestCalculateDelay_RetryAfterNotRespected(t *testing.T) {
	policy := &RetryPolicy{
		Name:              "test",
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		Multiplier:        2.0,
		JitterEnabled:     false,
		RespectRetryAfter: false, // Disabled
	}

	// When RespectRetryAfter is false, hint should be ignored
	delay := policy.CalculateDelay(1, 10*time.Second)
	if delay != 1*time.Second {
		t.Errorf("With RespectRetryAfter=false: delay = %v, want 1s (ignoring hint)", delay)
	}
}

func TestGetPolicyForFailure(t *testing.T) {
	tests := []struct {
		failureType  FailureType
		expectedName string
		expectNil    bool
	}{
		{FailureTypeRateLimit, "rate_limit", false},
		{FailureTypeOverloaded, "overloaded", false},
		{FailureTypeServerError, "server_error", false},
		{FailureTypeTimeout, "timeout", false},
		{FailureTypeConnectionError, "connection_error", false},
		{FailureTypeStreamInterrupted, "stream_interrupted", false},
		{FailureTypeCacheError, "cache_error", false},
		// Non-retryable types should return nil
		{FailureTypeAuthInvalid, "", true},
		{FailureTypePermissionDenied, "", true},
		{FailureTypeQuotaExhausted, "", true},
		{FailureTypeContextTooLong, "", true},
	}

	for _, tc := range tests {
		policy := GetPolicyForFailure(tc.failureType)
		if tc.expectNil {
			if policy != nil {
				t.Errorf("GetPolicyForFailure(%s): expected nil, got %s", tc.failureType, policy.Name)
			}
		} else {
			if policy == nil {
				t.Errorf("GetPolicyForFailure(%s): expected policy, got nil", tc.failureType)
			} else if policy.Name != tc.expectedName {
				t.Errorf("GetPolicyForFailure(%s): name = %s, want %s", tc.failureType, policy.Name, tc.expectedName)
			}
		}
	}
}

func TestGetDefaultPolicy(t *testing.T) {
	policy := GetDefaultPolicy()

	if policy == nil {
		t.Fatal("GetDefaultPolicy returned nil")
	}
	if policy.Name != "default" {
		t.Errorf("Name = %s, want default", policy.Name)
	}
	if policy.MaxAttempts < 1 {
		t.Errorf("MaxAttempts = %d, want >= 1", policy.MaxAttempts)
	}
	if policy.InitialDelay <= 0 {
		t.Errorf("InitialDelay = %v, want > 0", policy.InitialDelay)
	}
}

func TestRetryState_ShouldRetry(t *testing.T) {
	policy := &RetryPolicy{
		Name:         "test",
		MaxAttempts:  3,
		MaxTotalTime: 30 * time.Second,
	}

	state := NewRetryState("req-1", "idem-1", "openai")

	// Initially should retry (0 attempts)
	if !state.ShouldRetry(policy) {
		t.Error("ShouldRetry should be true with 0 attempts")
	}

	// After 1 attempt, should still retry
	state.TotalAttempts = 1
	if !state.ShouldRetry(policy) {
		t.Error("ShouldRetry should be true with 1 attempt")
	}

	// After 2 attempts, should still retry
	state.TotalAttempts = 2
	if !state.ShouldRetry(policy) {
		t.Error("ShouldRetry should be true with 2 attempts")
	}

	// After 3 attempts (at max), should not retry
	state.TotalAttempts = 3
	if state.ShouldRetry(policy) {
		t.Error("ShouldRetry should be false at max attempts")
	}
}

func TestRetryState_ShouldRetry_TimeLimit(t *testing.T) {
	policy := &RetryPolicy{
		Name:         "test",
		MaxAttempts:  10,
		MaxTotalTime: 1 * time.Millisecond, // Very short for testing
	}

	state := NewRetryState("req-1", "idem-1", "openai")
	state.TotalAttempts = 1

	// Wait for time limit to pass
	time.Sleep(5 * time.Millisecond)

	if state.ShouldRetry(policy) {
		t.Error("ShouldRetry should be false after time limit exceeded")
	}
}

func TestNewRetryResult(t *testing.T) {
	state := NewRetryState("req-1", "idem-1", "openai")
	state.TotalAttempts = 3
	state.UsedFallback = true
	state.Policy = &PolicyRateLimit

	result := NewRetryResult(state, true, nil)

	if !result.Success {
		t.Error("Success should be true")
	}
	if result.TotalAttempts != 3 {
		t.Errorf("TotalAttempts = %d, want 3", result.TotalAttempts)
	}
	if !result.UsedFallback {
		t.Error("UsedFallback should be true")
	}
	if result.PolicyUsed != "rate_limit" {
		t.Errorf("PolicyUsed = %s, want rate_limit", result.PolicyUsed)
	}
}

func TestPolicyRateLimit_Config(t *testing.T) {
	// Verify rate limit policy has expected configuration
	if PolicyRateLimit.MaxAttempts < 3 {
		t.Errorf("RateLimit MaxAttempts = %d, want >= 3", PolicyRateLimit.MaxAttempts)
	}
	if !PolicyRateLimit.RespectRetryAfter {
		t.Error("RateLimit should respect Retry-After header")
	}
	if !PolicyRateLimit.JitterEnabled {
		t.Error("RateLimit should have jitter enabled")
	}
}

func TestPolicyOverloaded_Config(t *testing.T) {
	// Verify overloaded policy has expected configuration
	if !PolicyOverloaded.TryFallbackFirst {
		t.Error("Overloaded should try fallback first")
	}
	if PolicyOverloaded.InitialDelay < 2*time.Second {
		t.Errorf("Overloaded InitialDelay = %v, should be >= 2s for aggressive backoff", PolicyOverloaded.InitialDelay)
	}
}

func TestPolicyServerError_Config(t *testing.T) {
	// Verify server error policy has expected configuration
	if !PolicyServerError.TryFallbackFirst {
		t.Error("ServerError should try fallback first")
	}
	if !PolicyServerError.FallbackOnExhaust {
		t.Error("ServerError should fallback on exhaust")
	}
}

func TestPolicyTimeout_Config(t *testing.T) {
	// Verify timeout policy has minimal delay
	if PolicyTimeout.InitialDelay != 0 {
		t.Errorf("Timeout InitialDelay = %v, want 0 (immediate retry)", PolicyTimeout.InitialDelay)
	}
	if PolicyTimeout.MaxAttempts < 2 {
		t.Errorf("Timeout MaxAttempts = %d, want >= 2", PolicyTimeout.MaxAttempts)
	}
}
