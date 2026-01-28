package shared

import (
	"os"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg == nil {
		t.Fatal("DefaultRetryConfig returned nil")
	}
	if cfg.MaxTotalAttempts != 0 {
		t.Errorf("expected MaxTotalAttempts=0 (no cap), got %d", cfg.MaxTotalAttempts)
	}
	if cfg.MaxProviderRetryAfterMs != 10000 {
		t.Errorf("expected MaxProviderRetryAfterMs=10000, got %d", cfg.MaxProviderRetryAfterMs)
	}
	if cfg.RetryIrreversible {
		t.Error("expected RetryIrreversible=false by default")
	}
}

func TestLoadRetryConfigFromEnv(t *testing.T) {
	os.Setenv("PLANDEX_MAX_RETRY_ATTEMPTS", "7")
	os.Setenv("PLANDEX_MAX_RETRY_DELAY_MS", "5000")
	os.Setenv("PLANDEX_RETRY_IRREVERSIBLE", "true")
	defer func() {
		os.Unsetenv("PLANDEX_MAX_RETRY_ATTEMPTS")
		os.Unsetenv("PLANDEX_MAX_RETRY_DELAY_MS")
		os.Unsetenv("PLANDEX_RETRY_IRREVERSIBLE")
	}()

	cfg := LoadRetryConfigFromEnv()
	if cfg.MaxTotalAttempts != 7 {
		t.Errorf("expected MaxTotalAttempts=7, got %d", cfg.MaxTotalAttempts)
	}
	if cfg.MaxRetryDelayMs != 5000 {
		t.Errorf("expected MaxRetryDelayMs=5000, got %d", cfg.MaxRetryDelayMs)
	}
	if !cfg.RetryIrreversible {
		t.Error("expected RetryIrreversible=true from env")
	}
}

func TestGetStrategy_DefaultFallback(t *testing.T) {
	cfg := DefaultRetryConfig()

	// Should fall through to GetRetryStrategy default
	s := cfg.GetStrategy(FailureTypeRateLimit)
	if !s.ShouldRetry {
		t.Error("rate_limit strategy should be retryable")
	}
	if s.MaxAttempts != 5 {
		t.Errorf("rate_limit MaxAttempts expected 5, got %d", s.MaxAttempts)
	}
}

func TestGetStrategy_Override(t *testing.T) {
	cfg := &RetryConfig{
		Overrides: map[FailureType]*RetryStrategy{
			FailureTypeRateLimit: {
				ShouldRetry:       true,
				MaxAttempts:       2,
				InitialDelayMs:    500,
				MaxDelayMs:        3000,
				BackoffMultiplier: 1.5,
				UseJitter:         false,
			},
		},
	}

	s := cfg.GetStrategy(FailureTypeRateLimit)
	if s.MaxAttempts != 2 {
		t.Errorf("expected override MaxAttempts=2, got %d", s.MaxAttempts)
	}
	if s.InitialDelayMs != 500 {
		t.Errorf("expected override InitialDelayMs=500, got %d", s.InitialDelayMs)
	}
}

func TestEffectiveMaxAttempts_GlobalCap(t *testing.T) {
	cfg := &RetryConfig{
		MaxTotalAttempts: 3, // Cap at 3
	}

	// rate_limit default is 5 attempts; global cap should bring it to 3
	effective := cfg.EffectiveMaxAttempts(FailureTypeRateLimit)
	if effective != 3 {
		t.Errorf("expected global cap of 3, got %d", effective)
	}

	// timeout default is 2 attempts; below global cap, so unchanged
	effective = cfg.EffectiveMaxAttempts(FailureTypeTimeout)
	if effective != 2 {
		t.Errorf("expected timeout at 2 (below cap), got %d", effective)
	}
}

func TestComputeBackoffDelay_WithRetryAfter(t *testing.T) {
	cfg := DefaultRetryConfig()
	strategy := GetRetryStrategy(FailureTypeRateLimit)

	delay := cfg.ComputeBackoffDelay(strategy, 0, 5) // provider says "retry after 5s"
	// Should be 5s * 1.1 = 5.5s = 5500ms
	if delay < 5000*time.Millisecond || delay > 6000*time.Millisecond {
		t.Errorf("expected ~5500ms with 10%% padding, got %v", delay)
	}
}

func TestComputeBackoffDelay_ExponentialGrowth(t *testing.T) {
	cfg := &RetryConfig{}
	strategy := RetryStrategy{
		ShouldRetry:       true,
		MaxAttempts:       5,
		InitialDelayMs:    1000,
		MaxDelayMs:        30000,
		BackoffMultiplier: 2.0,
		UseJitter:         false,
	}

	// attempt 0: 1000 * 2^0 = 1000ms
	d0 := cfg.ComputeBackoffDelay(strategy, 0, 0)
	if d0 != 1000*time.Millisecond {
		t.Errorf("attempt 0: expected 1000ms, got %v", d0)
	}

	// attempt 1: 1000 * 2^1 = 2000ms
	d1 := cfg.ComputeBackoffDelay(strategy, 1, 0)
	if d1 != 2000*time.Millisecond {
		t.Errorf("attempt 1: expected 2000ms, got %v", d1)
	}

	// attempt 2: 1000 * 2^2 = 4000ms
	d2 := cfg.ComputeBackoffDelay(strategy, 2, 0)
	if d2 != 4000*time.Millisecond {
		t.Errorf("attempt 2: expected 4000ms, got %v", d2)
	}
}

func TestComputeBackoffDelay_MaxClamping(t *testing.T) {
	cfg := &RetryConfig{
		MaxRetryDelayMs: 2500, // global cap at 2.5s
	}
	strategy := RetryStrategy{
		ShouldRetry:       true,
		InitialDelayMs:    1000,
		MaxDelayMs:        30000,
		BackoffMultiplier: 2.0,
		UseJitter:         false,
	}

	// attempt 2 would be 4000ms but should be clamped to 2500ms
	d := cfg.ComputeBackoffDelay(strategy, 2, 0)
	if d > 2500*time.Millisecond {
		t.Errorf("expected clamping to 2500ms, got %v", d)
	}
}

func TestComputeBackoffDelay_JitterBounds(t *testing.T) {
	cfg := &RetryConfig{}
	strategy := RetryStrategy{
		ShouldRetry:       true,
		InitialDelayMs:    1000,
		MaxDelayMs:        10000,
		BackoffMultiplier: 2.0,
		UseJitter:         true,
	}

	// Run 100 iterations; jitter should keep all values in [0, computed]
	for i := 0; i < 100; i++ {
		d := cfg.ComputeBackoffDelay(strategy, 1, 0) // base would be 2000ms
		if d < 0 {
			t.Errorf("jitter produced negative delay: %v", d)
		}
		if d > 2000*time.Millisecond {
			t.Errorf("jitter exceeded base delay: %v > 2000ms", d)
		}
	}
}

func TestIsProviderRetryAfterAcceptable(t *testing.T) {
	cfg := &RetryConfig{
		MaxProviderRetryAfterMs: 10000, // 10 seconds
	}

	if !cfg.IsProviderRetryAfterAcceptable(5) {
		t.Error("5s should be acceptable with 10s cap")
	}
	if cfg.IsProviderRetryAfterAcceptable(15) {
		t.Error("15s should NOT be acceptable with 10s cap")
	}
	if !cfg.IsProviderRetryAfterAcceptable(10) {
		t.Error("10s (exactly at cap) should be acceptable")
	}
}

func TestComputeBackoffDelay_NonRetryable(t *testing.T) {
	cfg := &RetryConfig{}
	strategy := RetryStrategy{
		ShouldRetry: false,
	}

	d := cfg.ComputeBackoffDelay(strategy, 0, 0)
	if d != 0 {
		t.Errorf("non-retryable strategy should return 0 delay, got %v", d)
	}
}
