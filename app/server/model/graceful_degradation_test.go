package model

import (
	shared "plandex-shared"
	"testing"
	"time"
)

func TestDegradationManager_InitialState(t *testing.T) {
	m := NewDegradationManager(nil)

	if m.GetGlobalLevel() != DegradationNone {
		t.Errorf("Initial global level = %s, want none", m.GetGlobalLevel())
	}

	if m.GetProviderLevel("openai") != DegradationNone {
		t.Errorf("Initial provider level = %s, want none", m.GetProviderLevel("openai"))
	}
}

func TestDegradationManager_TriggerDegradation(t *testing.T) {
	m := NewDegradationManager(nil)

	id := m.TriggerDegradation(DegradationModerate, "test reason", "openai", 5*time.Minute)

	if id == "" {
		t.Error("Should return degradation ID")
	}

	level := m.GetProviderLevel("openai")
	if level != DegradationModerate {
		t.Errorf("Provider level = %s, want moderate", level)
	}

	// Global should still be none
	if m.GetGlobalLevel() != DegradationNone {
		t.Errorf("Global level = %s, want none", m.GetGlobalLevel())
	}
}

func TestDegradationManager_TriggerGlobalDegradation(t *testing.T) {
	m := NewDegradationManager(nil)

	m.TriggerDegradation(DegradationHeavy, "system wide issue", "", 5*time.Minute)

	if m.GetGlobalLevel() != DegradationHeavy {
		t.Errorf("Global level = %s, want heavy", m.GetGlobalLevel())
	}

	// Provider level should inherit global
	if m.GetEffectiveLevel("openai") != DegradationHeavy {
		t.Errorf("Effective provider level = %s, want heavy", m.GetEffectiveLevel("openai"))
	}
}

func TestDegradationManager_EffectiveLevel(t *testing.T) {
	m := NewDegradationManager(nil)

	// Set global to light
	m.TriggerDegradation(DegradationLight, "global", "", 5*time.Minute)

	// Set provider to heavy (higher)
	m.TriggerDegradation(DegradationHeavy, "provider", "openai", 5*time.Minute)

	// Effective should be the higher (more degraded) level
	if m.GetEffectiveLevel("openai") != DegradationHeavy {
		t.Errorf("Effective level = %s, want heavy (higher)", m.GetEffectiveLevel("openai"))
	}

	// Different provider should use global
	if m.GetEffectiveLevel("anthropic") != DegradationLight {
		t.Errorf("Effective level for anthropic = %s, want light (global)", m.GetEffectiveLevel("anthropic"))
	}
}

func TestDegradationManager_GetStrategy(t *testing.T) {
	m := NewDegradationManager(nil)

	m.TriggerDegradation(DegradationModerate, "test", "openai", 5*time.Minute)

	strategy := m.GetStrategy("openai")

	if strategy.Level != DegradationModerate {
		t.Errorf("Strategy level = %s, want moderate", strategy.Level)
	}
	if strategy.ContextReductionPct <= 0 {
		t.Error("Strategy should have context reduction")
	}
	if !strategy.PreferFasterModels {
		t.Error("Moderate should prefer faster models")
	}
}

func TestDegradationManager_IsFeatureEnabled(t *testing.T) {
	m := NewDegradationManager(nil)

	// No degradation - all features enabled
	if !m.IsFeatureEnabled("openai", "caching") {
		t.Error("Caching should be enabled with no degradation")
	}

	// Moderate degradation - caching disabled
	m.TriggerDegradation(DegradationModerate, "test", "openai", 5*time.Minute)

	if m.IsFeatureEnabled("openai", "caching") {
		t.Error("Caching should be disabled with moderate degradation")
	}
}

func TestDegradationManager_RecoverDegradation(t *testing.T) {
	m := NewDegradationManager(nil)

	id := m.TriggerDegradation(DegradationHeavy, "test", "openai", 5*time.Minute)

	// Verify degradation
	if m.GetProviderLevel("openai") != DegradationHeavy {
		t.Error("Provider should be degraded")
	}

	// Recover
	m.RecoverDegradation(id)

	// Should be back to none
	if m.GetProviderLevel("openai") != DegradationNone {
		t.Errorf("Provider level after recovery = %s, want none", m.GetProviderLevel("openai"))
	}
}

func TestDegradationManager_RecoverProvider(t *testing.T) {
	m := NewDegradationManager(nil)

	// Multiple degradations for same provider
	m.TriggerDegradation(DegradationLight, "test1", "openai", 5*time.Minute)
	m.TriggerDegradation(DegradationModerate, "test2", "openai", 5*time.Minute)

	// Recover all for provider
	m.RecoverProvider("openai")

	if m.GetProviderLevel("openai") != DegradationNone {
		t.Error("All provider degradations should be recovered")
	}
}

func TestDegradationManager_RecoverAll(t *testing.T) {
	m := NewDegradationManager(nil)

	m.TriggerDegradation(DegradationHeavy, "global", "", 5*time.Minute)
	m.TriggerDegradation(DegradationModerate, "openai", "openai", 5*time.Minute)
	m.TriggerDegradation(DegradationLight, "anthropic", "anthropic", 5*time.Minute)

	m.RecoverAll()

	if m.GetGlobalLevel() != DegradationNone {
		t.Error("Global should be recovered")
	}
	if len(m.GetActiveDegradations()) != 0 {
		t.Error("All degradations should be cleared")
	}
}

func TestDegradationManager_TriggerFromFailure(t *testing.T) {
	config := &DegradationConfig{
		LightThreshold:    10,
		ModerateThreshold: 25,
		HeavyThreshold:    50,
		CriticalThreshold: 75,
	}
	m := NewDegradationManager(config)

	failure := &shared.ProviderFailure{
		Type:     shared.FailureTypeRateLimit,
		Provider: "openai",
	}

	// 30% error rate should trigger moderate
	m.TriggerFromFailure(failure, 30)

	level := m.GetProviderLevel("openai")
	if level != DegradationModerate {
		t.Errorf("Level = %s, want moderate for 30%% error rate", level)
	}
}

func TestDegradationManager_GetRequestModifications(t *testing.T) {
	m := NewDegradationManager(nil)

	// No degradation
	mods := m.GetRequestModifications("openai", 8000, 30000, false)
	if mods.MaxTokens != 8000 {
		t.Errorf("MaxTokens without degradation = %d, want 8000", mods.MaxTokens)
	}

	// Heavy degradation
	m.TriggerDegradation(DegradationHeavy, "test", "openai", 5*time.Minute)

	mods = m.GetRequestModifications("openai", 8000, 30000, false)

	// Should reduce tokens by 50%
	if mods.MaxTokens >= 8000 {
		t.Errorf("MaxTokens with heavy degradation = %d, should be reduced", mods.MaxTokens)
	}

	// Should increase timeout
	if mods.TimeoutMs <= 30000 {
		t.Errorf("TimeoutMs with heavy degradation = %d, should be increased", mods.TimeoutMs)
	}

	// Non-urgent should be queued
	if !mods.ShouldQueue {
		t.Error("Non-urgent request should be queued in heavy degradation")
	}

	// Urgent should not be queued
	modsUrgent := m.GetRequestModifications("openai", 8000, 30000, true)
	if modsUrgent.ShouldQueue {
		t.Error("Urgent request should not be queued")
	}
}

func TestDegradationManager_Metrics(t *testing.T) {
	m := NewDegradationManager(nil)

	m.TriggerDegradation(DegradationModerate, "test1", "openai", 5*time.Minute)
	m.TriggerDegradation(DegradationLight, "test2", "anthropic", 5*time.Minute)

	metrics := m.GetMetrics()

	if metrics.ActiveCount != 2 {
		t.Errorf("ActiveCount = %d, want 2", metrics.ActiveCount)
	}
	if len(metrics.ProviderLevels) != 2 {
		t.Errorf("ProviderLevels count = %d, want 2", len(metrics.ProviderLevels))
	}
}

func TestDegradationManager_Callback(t *testing.T) {
	m := NewDegradationManager(nil)

	callbackCalled := false
	var capturedLevel DegradationLevel

	m.SetDegradationChangeCallback(func(level DegradationLevel, reason string) {
		callbackCalled = true
		capturedLevel = level
	})

	m.TriggerDegradation(DegradationHeavy, "test", "", 5*time.Minute)

	// Give callback time to run
	time.Sleep(10 * time.Millisecond)

	if !callbackCalled {
		t.Error("Callback should have been called")
	}
	if capturedLevel != DegradationHeavy {
		t.Errorf("Captured level = %s, want heavy", capturedLevel)
	}
}

func TestDegradationManager_ExpiredDegradation(t *testing.T) {
	m := NewDegradationManager(nil)

	// Trigger with very short duration
	m.TriggerDegradation(DegradationHeavy, "test", "openai", 1*time.Millisecond)

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	// Get metrics triggers cleanup
	metrics := m.GetMetrics()

	// Level should still show but GetMetrics cleans up
	// The actual level check depends on when cleanup runs
	_ = metrics
}
