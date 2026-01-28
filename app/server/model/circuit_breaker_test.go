package model

import (
	shared "plandex-shared"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	// Unknown provider should have closed circuit
	if cb.IsOpen("openai") {
		t.Error("Circuit should be closed for unknown provider")
	}

	// No state should exist yet
	state := cb.GetState("openai")
	if state != nil {
		t.Error("State should be nil for unknown provider")
	}
}

func TestCircuitBreaker_RecordSuccess(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	cb.RecordSuccess("openai")

	state := cb.GetState("openai")
	if state == nil {
		t.Fatal("State should exist after recording")
	}
	if state.TotalSuccesses != 1 {
		t.Errorf("TotalSuccesses = %d, want 1", state.TotalSuccesses)
	}
	if state.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", state.TotalRequests)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", state.ConsecutiveFailures)
	}
}

func TestCircuitBreaker_RecordFailure(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
		Message:  "Rate limited",
	}

	cb.RecordFailure("openai", failure)

	state := cb.GetState("openai")
	if state == nil {
		t.Fatal("State should exist after recording")
	}
	if state.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d, want 1", state.TotalFailures)
	}
	if state.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", state.ConsecutiveFailures)
	}
	if len(state.RecentFailures) != 1 {
		t.Errorf("RecentFailures = %d, want 1", len(state.RecentFailures))
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:      3,
		SuccessThreshold:      2,
		OpenDuration:          30 * time.Second,
		HalfOpenMaxRequests:   2,
		FailureWindowDuration: 60 * time.Second,
		FailureWindowMax:      10,
	}
	cb := NewCircuitBreaker(config)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure("openai", failure)
	}

	// Circuit should now be open
	if !cb.IsOpen("openai") {
		t.Error("Circuit should be open after threshold failures")
	}

	state := cb.GetState("openai")
	if state.State != CircuitOpen {
		t.Errorf("State = %s, want open", state.State)
	}
}

func TestCircuitBreaker_ExcludedFailureTypes(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	// Context too long is excluded by default
	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeContextTooLong,
		HTTPCode: 400,
	}

	// Record many "failures" that should be excluded
	for i := 0; i < 10; i++ {
		cb.RecordFailure("openai", failure)
	}

	// Circuit should still be closed (excluded failures don't count)
	if cb.IsOpen("openai") {
		t.Error("Circuit should remain closed for excluded failure types")
	}

	// State should not even exist (excluded failures aren't tracked)
	state := cb.GetState("openai")
	if state != nil {
		t.Error("State should be nil for excluded-only failures")
	}
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:      2,
		SuccessThreshold:      1,
		OpenDuration:          1 * time.Millisecond, // Very short for testing
		HalfOpenMaxRequests:   2,
		FailureWindowDuration: 60 * time.Second,
		FailureWindowMax:      10,
	}
	cb := NewCircuitBreaker(config)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Open the circuit
	cb.RecordFailure("openai", failure)
	cb.RecordFailure("openai", failure)

	// Wait for open duration to pass
	time.Sleep(5 * time.Millisecond)

	// Circuit should allow a request (transitioning to half-open)
	if cb.IsOpen("openai") {
		t.Error("Circuit should allow request after open duration")
	}
}

func TestCircuitBreaker_ClosesAfterRecovery(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:      2,
		SuccessThreshold:      2,
		OpenDuration:          1 * time.Millisecond,
		HalfOpenMaxRequests:   3,
		FailureWindowDuration: 60 * time.Second,
		FailureWindowMax:      10,
	}
	cb := NewCircuitBreaker(config)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Open the circuit
	cb.RecordFailure("openai", failure)
	cb.RecordFailure("openai", failure)

	// Wait for transition to half-open
	time.Sleep(5 * time.Millisecond)

	// Record successes in half-open
	cb.RecordSuccess("openai")
	cb.RecordSuccess("openai")

	// Circuit should be closed
	state := cb.GetState("openai")
	if state.State != CircuitClosed {
		t.Errorf("State = %s, want closed", state.State)
	}
}

func TestCircuitBreaker_ReopensOnHalfOpenFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold:      2,
		SuccessThreshold:      2,
		OpenDuration:          1 * time.Millisecond,
		HalfOpenMaxRequests:   3,
		FailureWindowDuration: 60 * time.Second,
		FailureWindowMax:      10,
	}
	cb := NewCircuitBreaker(config)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Open the circuit
	cb.RecordFailure("openai", failure)
	cb.RecordFailure("openai", failure)

	// Wait for transition to half-open
	time.Sleep(5 * time.Millisecond)

	// Record a success (enters half-open)
	cb.RecordSuccess("openai")

	state := cb.GetState("openai")
	if state.State != CircuitHalfOpen {
		t.Errorf("State = %s, want half_open", state.State)
	}

	// Record a failure in half-open
	cb.RecordFailure("openai", failure)

	// Circuit should reopen
	state = cb.GetState("openai")
	if state.State != CircuitOpen {
		t.Errorf("State = %s, want open", state.State)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Record failures to open circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure("openai", failure)
	}

	// Reset the circuit
	cb.Reset("openai")

	// Circuit should be closed
	if cb.IsOpen("openai") {
		t.Error("Circuit should be closed after reset")
	}

	state := cb.GetState("openai")
	if state.State != CircuitClosed {
		t.Errorf("State = %s, want closed", state.State)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", state.ConsecutiveFailures)
	}
}

func TestCircuitBreaker_ResetAll(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Record failures for multiple providers
	for i := 0; i < 5; i++ {
		cb.RecordFailure("openai", failure)
		cb.RecordFailure("anthropic", failure)
	}

	// Reset all
	cb.ResetAll()

	// All states should be cleared
	if cb.GetState("openai") != nil {
		t.Error("openai state should be nil after ResetAll")
	}
	if cb.GetState("anthropic") != nil {
		t.Error("anthropic state should be nil after ResetAll")
	}
}

func TestCircuitBreaker_GetMetrics(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Record some activity
	cb.RecordSuccess("openai")
	cb.RecordSuccess("openai")
	cb.RecordFailure("openai", failure)

	cb.RecordFailure("anthropic", failure)
	cb.RecordFailure("anthropic", failure)
	cb.RecordFailure("anthropic", failure)
	cb.RecordFailure("anthropic", failure)
	cb.RecordFailure("anthropic", failure) // Should open

	metrics := cb.GetMetrics()

	if metrics.TotalProviders != 2 {
		t.Errorf("TotalProviders = %d, want 2", metrics.TotalProviders)
	}
	if metrics.OpenCircuits != 1 {
		t.Errorf("OpenCircuits = %d, want 1", metrics.OpenCircuits)
	}
	if metrics.ClosedCircuits != 1 {
		t.Errorf("ClosedCircuits = %d, want 1", metrics.ClosedCircuits)
	}

	openaiMetrics := metrics.Providers["openai"]
	if openaiMetrics.TotalRequests != 3 {
		t.Errorf("openai TotalRequests = %d, want 3", openaiMetrics.TotalRequests)
	}
	if openaiMetrics.FailureRate < 0.3 || openaiMetrics.FailureRate > 0.4 {
		t.Errorf("openai FailureRate = %f, want ~0.33", openaiMetrics.FailureRate)
	}
}

func TestCircuitBreaker_SuccessResetsConsecutiveFailures(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Record some failures (not enough to open)
	cb.RecordFailure("openai", failure)
	cb.RecordFailure("openai", failure)

	state := cb.GetState("openai")
	if state.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", state.ConsecutiveFailures)
	}

	// Record a success
	cb.RecordSuccess("openai")

	state = cb.GetState("openai")
	if state.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures after success = %d, want 0", state.ConsecutiveFailures)
	}
}

func TestCircuitBreaker_MultipleProviders(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	// Record failures for openai (open it)
	for i := 0; i < 5; i++ {
		cb.RecordFailure("openai", failure)
	}

	// anthropic should still be closed
	if cb.IsOpen("anthropic") {
		t.Error("anthropic circuit should be closed")
	}
	if !cb.IsOpen("openai") {
		t.Error("openai circuit should be open")
	}

	// Record success for anthropic
	cb.RecordSuccess("anthropic")

	// States should be independent
	openaiState := cb.GetState("openai")
	anthropicState := cb.GetState("anthropic")

	if openaiState.State != CircuitOpen {
		t.Errorf("openai state = %s, want open", openaiState.State)
	}
	if anthropicState.State != CircuitClosed {
		t.Errorf("anthropic state = %s, want closed", anthropicState.State)
	}
}
