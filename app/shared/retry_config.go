package shared

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// RETRY CONFIGURATION
// =============================================================================
//
// RetryConfig is the single source of truth for retry behaviour across the
// system.  GetRetryStrategy() in provider_failures.go provides per-FailureType
// defaults; RetryConfig lets callers override those defaults or impose global
// caps without touching the strategy table.
//
// Thread-safety: RetryConfig is designed to be initialised once and then
// treated as read-only.  The Overrides map is never mutated after creation.
//
// =============================================================================

// RetryConfig holds the configurable retry policy for the system.
type RetryConfig struct {
	// MaxTotalAttempts caps the total number of attempts (initial + retries)
	// for any single operation, regardless of per-type strategy.
	// 0 means "no global cap; use per-type MaxAttempts".
	MaxTotalAttempts int `json:"maxTotalAttempts"`

	// MaxRetryDelayMs caps the delay between retries in milliseconds.
	// Any computed backoff exceeding this is clamped.
	// 0 means "no global cap; use per-type MaxDelayMs".
	MaxRetryDelayMs int `json:"maxRetryDelayMs"`

	// MaxProviderRetryAfterMs caps the provider-declared Retry-After value.
	// If a provider says "retry after 300 s" but this is set to 30000 ms,
	// the error is treated as non-retryable rather than waiting too long.
	MaxProviderRetryAfterMs int `json:"maxProviderRetryAfterMs"`

	// Overrides maps specific FailureTypes to custom RetryStrategy values.
	// A nil entry means "use GetRetryStrategy() default".
	Overrides map[FailureType]*RetryStrategy `json:"overrides,omitempty"`

	// RetryIrreversible controls whether irreversible operations may be
	// retried.  Default is false — the system will never retry an operation
	// classified as OperationIrreversible unless this is explicitly true.
	RetryIrreversible bool `json:"retryIrreversible"`
}

// DefaultRetryConfig returns a RetryConfig whose behaviour matches the
// pre-existing hard-coded defaults in the system (3 retries, ~1 s jitter,
// 10 s max provider retry-after).
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxTotalAttempts:        0, // no global cap — defer to per-type
		MaxRetryDelayMs:         0, // no global cap
		MaxProviderRetryAfterMs: 10000,
		Overrides:               nil,
		RetryIrreversible:       false,
	}
}

// LoadRetryConfigFromEnv builds a RetryConfig from environment variables.
//
// Supported variables:
//
//	PLANDEX_MAX_RETRY_ATTEMPTS       — global attempt cap
//	PLANDEX_MAX_RETRY_DELAY_MS       — global delay cap in ms
//	PLANDEX_MAX_PROVIDER_RETRY_AFTER_MS — cap on provider Retry-After in ms
//	PLANDEX_RETRY_IRREVERSIBLE       — "true" to allow retrying irreversible ops
func LoadRetryConfigFromEnv() *RetryConfig {
	cfg := DefaultRetryConfig()

	if v := os.Getenv("PLANDEX_MAX_RETRY_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			cfg.MaxTotalAttempts = n
		}
	}
	if v := os.Getenv("PLANDEX_MAX_RETRY_DELAY_MS"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			cfg.MaxRetryDelayMs = n
		}
	}
	if v := os.Getenv("PLANDEX_MAX_PROVIDER_RETRY_AFTER_MS"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			cfg.MaxProviderRetryAfterMs = n
		}
	}
	if v := os.Getenv("PLANDEX_RETRY_IRREVERSIBLE"); strings.EqualFold(strings.TrimSpace(v), "true") {
		cfg.RetryIrreversible = true
	}

	return cfg
}

// GetStrategy resolves the effective RetryStrategy for a given FailureType,
// applying any per-type override before falling back to the default table.
func (c *RetryConfig) GetStrategy(failureType FailureType) RetryStrategy {
	if c != nil && c.Overrides != nil {
		if override, ok := c.Overrides[failureType]; ok && override != nil {
			return *override
		}
	}
	return GetRetryStrategy(failureType)
}

// EffectiveMaxAttempts returns the maximum number of total attempts allowed
// for the given failure type, applying global caps if set.
func (c *RetryConfig) EffectiveMaxAttempts(failureType FailureType) int {
	strategy := c.GetStrategy(failureType)
	if c != nil && c.MaxTotalAttempts > 0 {
		if strategy.MaxAttempts > c.MaxTotalAttempts {
			return c.MaxTotalAttempts
		}
	}
	return strategy.MaxAttempts
}

// ComputeBackoffDelay calculates the delay before the next retry attempt.
//
// If the provider supplied a Retry-After value (retryAfterSeconds > 0), that
// is used as the base (with 10% padding).  Otherwise the strategy's
// InitialDelayMs is scaled by BackoffMultiplier^attempt with optional jitter
// and clamped to [0, effectiveMaxMs].
func (c *RetryConfig) ComputeBackoffDelay(strategy RetryStrategy, attempt int, retryAfterSeconds int) time.Duration {
	if retryAfterSeconds > 0 {
		// Provider told us how long to wait.  Add 10% padding.
		delayMs := retryAfterSeconds * 1000
		delayMs = int(float64(delayMs) * 1.1)

		// Clamp to global cap if set.
		effectiveMax := c.effectiveMaxDelayMs(strategy)
		if effectiveMax > 0 && delayMs > effectiveMax {
			// Provider wants longer than our cap — honour the cap but still
			// respect it (don't treat as non-retryable here; that decision
			// lives in the retry loop via MaxProviderRetryAfterMs).
			delayMs = effectiveMax
		}
		return time.Duration(delayMs) * time.Millisecond
	}

	if !strategy.ShouldRetry {
		return 0
	}

	// Exponential backoff: initialDelay * multiplier^attempt
	baseMs := float64(strategy.InitialDelayMs)
	if baseMs <= 0 {
		baseMs = 1000 // fallback: 1 second
	}

	delayMs := baseMs * math.Pow(strategy.BackoffMultiplier, float64(attempt))

	if strategy.UseJitter {
		// Full jitter: uniform in [0, computed delay]
		delayMs = rand.Float64() * delayMs
	}

	// Clamp to effective max
	effectiveMax := c.effectiveMaxDelayMs(strategy)
	if effectiveMax > 0 && delayMs > float64(effectiveMax) {
		delayMs = float64(effectiveMax)
	}

	return time.Duration(int(delayMs)) * time.Millisecond
}

func (c *RetryConfig) effectiveMaxDelayMs(strategy RetryStrategy) int {
	strategyMax := strategy.MaxDelayMs
	globalMax := 0
	if c != nil {
		globalMax = c.MaxRetryDelayMs
	}

	switch {
	case strategyMax > 0 && globalMax > 0:
		if globalMax < strategyMax {
			return globalMax
		}
		return strategyMax
	case strategyMax > 0:
		return strategyMax
	case globalMax > 0:
		return globalMax
	default:
		return 0 // no cap
	}
}

// IsProviderRetryAfterAcceptable checks whether a provider-declared
// Retry-After value is within the configured ceiling.  If the provider
// says "retry after X seconds" and X exceeds MaxProviderRetryAfterMs, the
// caller should treat the error as non-retryable rather than waiting.
func (c *RetryConfig) IsProviderRetryAfterAcceptable(retryAfterSeconds int) bool {
	if c == nil || c.MaxProviderRetryAfterMs <= 0 {
		return true // no cap configured
	}
	return retryAfterSeconds*1000 <= c.MaxProviderRetryAfterMs
}
