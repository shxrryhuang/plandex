package model

import (
	shared "plandex-shared"
	"testing"
	"time"
)

func TestHealthCheckManager_InitialState(t *testing.T) {
	m := NewHealthCheckManager(nil)

	// Unknown provider should return unknown status
	status := m.GetStatus("openai")
	if status != HealthStatusUnknown {
		t.Errorf("Status = %s, want unknown", status)
	}

	health := m.GetHealth("openai")
	if health.Score != 50 {
		t.Errorf("Score = %d, want 50 (default for unknown)", health.Score)
	}
}

func TestHealthCheckManager_RecordSuccess(t *testing.T) {
	m := NewHealthCheckManager(nil)

	m.RecordRequest("openai", true, 500, nil)
	m.RecordRequest("openai", true, 600, nil)
	m.RecordRequest("openai", true, 400, nil)

	health := m.GetHealth("openai")

	if health.ConsecutiveSuccesses != 3 {
		t.Errorf("ConsecutiveSuccesses = %d, want 3", health.ConsecutiveSuccesses)
	}
	if health.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", health.ConsecutiveFailures)
	}
	if health.Score < 70 {
		t.Errorf("Score = %d, should be >= 70 after successes", health.Score)
	}
}

func TestHealthCheckManager_RecordFailure(t *testing.T) {
	m := NewHealthCheckManager(nil)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		HTTPCode: 429,
	}

	m.RecordRequest("openai", false, 0, failure)
	m.RecordRequest("openai", false, 0, failure)
	m.RecordRequest("openai", false, 0, failure)

	health := m.GetHealth("openai")

	if health.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", health.ConsecutiveFailures)
	}
	if health.ConsecutiveSuccesses != 0 {
		t.Errorf("ConsecutiveSuccesses = %d, want 0", health.ConsecutiveSuccesses)
	}
}

func TestHealthCheckManager_HealthStatusTransitions(t *testing.T) {
	config := &HealthCheckConfig{
		HealthyThreshold:    80,
		DegradedThreshold:   50,
		HealthySuccessRate:  0.95,
		DegradedSuccessRate: 0.80,
		HealthyLatencyMs:    1000,
		DegradedLatencyMs:   3000,
		MaxLatencySamples:   100,
	}
	m := NewHealthCheckManager(config)

	// Record many successes with good latency
	for i := 0; i < 20; i++ {
		m.RecordRequest("openai", true, 500, nil)
	}

	health := m.GetHealth("openai")
	if health.Status != HealthStatusHealthy {
		t.Errorf("Status = %s, want healthy after many successes", health.Status)
	}

	// Record failures to degrade
	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	for i := 0; i < 5; i++ {
		m.RecordRequest("openai", false, 0, failure)
	}

	health = m.GetHealth("openai")
	if health.Status == HealthStatusHealthy {
		t.Error("Status should not be healthy after failures")
	}
}

func TestHealthCheckManager_LatencyTracking(t *testing.T) {
	m := NewHealthCheckManager(nil)

	// Record requests with varying latency
	latencies := []int64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	for _, lat := range latencies {
		m.RecordRequest("openai", true, lat, nil)
	}

	health := m.GetHealth("openai")

	if health.AvgLatencyMs == 0 {
		t.Error("AvgLatencyMs should be calculated")
	}
	if health.P95LatencyMs == 0 {
		t.Error("P95LatencyMs should be calculated")
	}
	if health.P99LatencyMs == 0 {
		t.Error("P99LatencyMs should be calculated")
	}

	// Average should be around 550ms
	if health.AvgLatencyMs < 400 || health.AvgLatencyMs > 700 {
		t.Errorf("AvgLatencyMs = %d, expected around 550", health.AvgLatencyMs)
	}
}

func TestHealthCheckManager_GetBestProvider(t *testing.T) {
	m := NewHealthCheckManager(nil)

	// Setup different health levels
	for i := 0; i < 10; i++ {
		m.RecordRequest("openai", true, 500, nil)
	}

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	for i := 0; i < 5; i++ {
		m.RecordRequest("anthropic", false, 0, failure)
	}

	providers := []string{"openai", "anthropic"}
	best := m.GetBestProvider(providers)

	if best != "openai" {
		t.Errorf("Best provider = %s, want openai (healthier)", best)
	}
}

func TestHealthCheckManager_GetHealthyProviders(t *testing.T) {
	m := NewHealthCheckManager(nil)

	// Make openai healthy
	for i := 0; i < 10; i++ {
		m.RecordRequest("openai", true, 500, nil)
	}

	// Make anthropic unhealthy
	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}
	for i := 0; i < 10; i++ {
		m.RecordRequest("anthropic", false, 0, failure)
	}

	healthy := m.GetHealthyProviders()

	found := false
	for _, p := range healthy {
		if p == "openai" {
			found = true
		}
		if p == "anthropic" {
			t.Error("anthropic should not be in healthy providers")
		}
	}

	if !found {
		t.Error("openai should be in healthy providers")
	}
}

func TestHealthCheckManager_SuccessResetsFailures(t *testing.T) {
	m := NewHealthCheckManager(nil)

	failure := &shared.ProviderFailure{Type: shared.FailureTypeRateLimit}

	// Record failures
	m.RecordRequest("openai", false, 0, failure)
	m.RecordRequest("openai", false, 0, failure)

	health := m.GetHealth("openai")
	if health.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", health.ConsecutiveFailures)
	}

	// Record success
	m.RecordRequest("openai", true, 500, nil)

	health = m.GetHealth("openai")
	if health.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures after success = %d, want 0", health.ConsecutiveFailures)
	}
	if health.ConsecutiveSuccesses != 1 {
		t.Errorf("ConsecutiveSuccesses = %d, want 1", health.ConsecutiveSuccesses)
	}
}

func TestHealthCheckManager_Metrics(t *testing.T) {
	m := NewHealthCheckManager(nil)

	// Setup multiple providers
	for i := 0; i < 5; i++ {
		m.RecordRequest("openai", true, 500, nil)
		m.RecordRequest("anthropic", true, 600, nil)
	}

	metrics := m.GetMetrics()

	if metrics.TotalProviders != 2 {
		t.Errorf("TotalProviders = %d, want 2", metrics.TotalProviders)
	}
	if len(metrics.ProviderDetails) != 2 {
		t.Errorf("ProviderDetails count = %d, want 2", len(metrics.ProviderDetails))
	}
}

func TestHealthCheckManager_Callback(t *testing.T) {
	m := NewHealthCheckManager(nil)

	callbackCalled := false
	var capturedOldStatus, capturedNewStatus HealthStatus

	m.SetHealthChangeCallback(func(provider string, oldStatus, newStatus HealthStatus) {
		callbackCalled = true
		capturedOldStatus = oldStatus
		capturedNewStatus = newStatus
	})

	// Record enough successes to change from unknown to healthy
	for i := 0; i < 15; i++ {
		m.RecordRequest("openai", true, 500, nil)
	}

	// Give callback time to run
	time.Sleep(10 * time.Millisecond)

	if !callbackCalled {
		t.Error("Callback should have been called on status change")
	}
	if capturedOldStatus != HealthStatusUnknown {
		t.Errorf("Old status = %s, want unknown", capturedOldStatus)
	}
	if capturedNewStatus != HealthStatusHealthy {
		t.Errorf("New status = %s, want healthy", capturedNewStatus)
	}
}
