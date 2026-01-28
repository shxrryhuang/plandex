package shared

import "time"

// =============================================================================
// ERROR HANDLING EXAMPLES AND DOCUMENTATION
// =============================================================================
//
// This file documents the error handling behavior of Plandex under various
// flaky provider conditions. It includes concrete scenarios showing:
//
// 1. How errors are classified (retryable vs fail-fast vs user-intervention)
// 2. How retry policies are applied with exponential backoff
// 3. How circuit breakers prevent cascading failures
// 4. How fallbacks work when primary providers fail
// 5. How partial failures are handled in streaming responses
//
// These examples serve as both documentation and test cases for verifying
// correct error handling behavior.
//
// =============================================================================

// =============================================================================
// ERROR CATEGORIES
// =============================================================================

// ErrorCategoryExample demonstrates the three error categories
type ErrorCategoryExample struct {
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Examples    []string `json:"examples"`
	Behavior    string   `json:"behavior"`
}

// GetErrorCategoryExamples returns documented error categories
func GetErrorCategoryExamples() []ErrorCategoryExample {
	return []ErrorCategoryExample{
		{
			Category:    "retryable",
			Description: "Errors that may succeed on retry with appropriate backoff",
			Examples: []string{
				"HTTP 429 Rate Limited - Provider is temporarily limiting requests",
				"HTTP 503 Service Unavailable - Provider is overloaded",
				"HTTP 502 Bad Gateway - Transient network issue",
				"HTTP 504 Gateway Timeout - Request took too long",
				"Connection timeout - Network connectivity issue",
				"Stream interrupted - Streaming response was cut off",
			},
			Behavior: "Retry with exponential backoff respecting Retry-After header. " +
				"Track failures in circuit breaker. After max retries, try fallback.",
		},
		{
			Category:    "fail_fast",
			Description: "Errors that will never succeed on retry and require immediate action",
			Examples: []string{
				"HTTP 401 Unauthorized - Invalid API key",
				"HTTP 403 Forbidden - Insufficient permissions",
				"HTTP 400 Context Too Long - Input exceeds model's limit",
				"HTTP 400 Invalid Request - Malformed request format",
				"HTTP 400 Content Policy Violation - Content blocked by provider",
				"HTTP 404 Model Not Found - Invalid model ID",
			},
			Behavior: "Do not retry. For context_too_long, try large context fallback. " +
				"For other errors, return immediately with clear error message.",
		},
		{
			Category:    "user_intervention",
			Description: "Errors that require user action before the run can continue",
			Examples: []string{
				"Quota Exhausted - Account has reached usage limits",
				"Account Suspended - Provider has suspended the account",
				"Billing Error - Payment issue with provider",
				"Authentication Required - API key not configured",
			},
			Behavior: "Stop execution, notify user with specific instructions for resolution. " +
				"Provide links to provider dashboard and suggested actions.",
		},
	}
}

// =============================================================================
// SCENARIO: RATE LIMITED WITH RECOVERY
// =============================================================================

// ScenarioRateLimitedRecovery demonstrates handling of rate limiting
type ScenarioRateLimitedRecovery struct {
	Description string                `json:"description"`
	Timeline    []ScenarioTimelineEvent `json:"timeline"`
	Outcome     string                `json:"outcome"`
}

// ScenarioTimelineEvent represents a point in time during error handling
type ScenarioTimelineEvent struct {
	TimeOffset  string `json:"timeOffset"`
	Event       string `json:"event"`
	Action      string `json:"action"`
	Details     string `json:"details,omitempty"`
}

// GetScenarioRateLimitedRecovery returns the rate limit recovery scenario
func GetScenarioRateLimitedRecovery() ScenarioRateLimitedRecovery {
	return ScenarioRateLimitedRecovery{
		Description: "User makes a request to OpenAI gpt-4. Provider returns 429 with Retry-After: 30s. " +
			"System waits 30s, retries, and succeeds.",
		Timeline: []ScenarioTimelineEvent{
			{
				TimeOffset: "0s",
				Event:      "Request sent to OpenAI (gpt-4)",
				Action:     "Send HTTP POST to provider API",
			},
			{
				TimeOffset: "0.1s",
				Event:      "Response: HTTP 429 Rate Limited",
				Action:     "Classify error using ClassifyProviderFailure()",
				Details:    "FailureType: rate_limit, Retryable: true, RetryAfter: 30s",
			},
			{
				TimeOffset: "0.2s",
				Event:      "Apply retry policy",
				Action:     "Get PolicyRateLimit, calculate delay",
				Details:    "Policy respects Retry-After, delay = 30s + jitter (~30.5s)",
			},
			{
				TimeOffset: "0.3s",
				Event:      "Record in circuit breaker",
				Action:     "CircuitBreaker.RecordFailure(openai, rate_limit)",
				Details:    "ConsecutiveFailures: 1, State: closed",
			},
			{
				TimeOffset: "0.4s",
				Event:      "Journal retry attempt",
				Action:     "Journal.AppendRetryAttempt()",
				Details:    "AttemptNumber: 1, DelayMs: 30500, WillRetry: true",
			},
			{
				TimeOffset: "30.5s",
				Event:      "Retry attempt 2",
				Action:     "Send HTTP POST to provider API",
			},
			{
				TimeOffset: "30.7s",
				Event:      "Response: HTTP 200 OK, streaming starts",
				Action:     "CircuitBreaker.RecordSuccess(openai)",
				Details:    "ConsecutiveFailures reset to 0",
			},
			{
				TimeOffset: "35s",
				Event:      "Stream completes",
				Action:     "Return response to caller",
			},
		},
		Outcome: "SUCCESS - Request recovered after one retry respecting Retry-After header. " +
			"Total time: ~35s. Circuit breaker recorded the incident but remained closed.",
	}
}

// =============================================================================
// SCENARIO: PROVIDER DOWN WITH CIRCUIT BREAKER AND FALLBACK
// =============================================================================

// GetScenarioProviderDownWithFallback returns the provider failure scenario
func GetScenarioProviderDownWithFallback() ScenarioRateLimitedRecovery {
	return ScenarioRateLimitedRecovery{
		Description: "Anthropic (claude-3) is experiencing outage. After 5 consecutive failures, " +
			"circuit breaker opens and system falls back to OpenRouter.",
		Timeline: []ScenarioTimelineEvent{
			{
				TimeOffset: "0s",
				Event:      "Request to Anthropic (claude-3)",
				Action:     "Send request",
			},
			{
				TimeOffset: "0.1s",
				Event:      "Response: HTTP 503 Overloaded",
				Action:     "Classify as FailureTypeOverloaded",
				Details:    "Apply PolicyOverloaded (tryFallbackFirst: true)",
			},
			{
				TimeOffset: "5s",
				Event:      "Retry attempt 2",
				Action:     "CircuitBreaker.consecutiveFailures = 2",
			},
			{
				TimeOffset: "15s",
				Event:      "Retry attempt 3",
				Action:     "CircuitBreaker.consecutiveFailures = 3",
			},
			{
				TimeOffset: "35s",
				Event:      "Retry attempt 4",
				Action:     "CircuitBreaker.consecutiveFailures = 4",
			},
			{
				TimeOffset: "75s",
				Event:      "Retry attempt 5 - still failing",
				Action:     "CircuitBreaker.consecutiveFailures = 5 >= threshold",
				Details:    "Circuit OPENS for Anthropic",
			},
			{
				TimeOffset: "75.1s",
				Event:      "Circuit breaker triggered",
				Action:     "Journal.AppendCircuitEvent(anthropic, closed -> open)",
			},
			{
				TimeOffset: "75.2s",
				Event:      "Try provider fallback",
				Action:     "Journal.AppendFallbackEvent(anthropic -> openrouter)",
			},
			{
				TimeOffset: "75.5s",
				Event:      "Request to OpenRouter",
				Action:     "Send request to fallback provider",
			},
			{
				TimeOffset: "78s",
				Event:      "Response: HTTP 200 OK",
				Action:     "Success via fallback",
			},
		},
		Outcome: "SUCCESS via FALLBACK - Primary provider (Anthropic) failed repeatedly, " +
			"circuit breaker opened after 5 failures, request succeeded via OpenRouter fallback. " +
			"Total time: ~78s. Circuit will remain open for 30s before testing recovery.",
	}
}

// =============================================================================
// SCENARIO: CONTEXT TOO LONG WITH LARGE CONTEXT FALLBACK
// =============================================================================

// GetScenarioContextTooLong returns the context limit scenario
func GetScenarioContextTooLong() ScenarioRateLimitedRecovery {
	return ScenarioRateLimitedRecovery{
		Description: "User request exceeds gpt-4 (8K) context window. System automatically " +
			"falls back to gpt-4-turbo (128K) configured as LargeContextFallback.",
		Timeline: []ScenarioTimelineEvent{
			{
				TimeOffset: "0s",
				Event:      "Request to OpenAI (gpt-4, 8K context)",
				Action:     "Send request with ~20K tokens",
			},
			{
				TimeOffset: "0.2s",
				Event:      "Response: HTTP 400 'maximum context length exceeded'",
				Action:     "Classify as FailureTypeContextTooLong",
				Details:    "Non-retryable error, but check for LargeContextFallback",
			},
			{
				TimeOffset: "0.3s",
				Event:      "Check fallback configuration",
				Action:     "ModelRoleConfig.LargeContextFallback exists (gpt-4-turbo-128k)",
			},
			{
				TimeOffset: "0.4s",
				Event:      "Activate large context fallback",
				Action:     "Journal.AppendFallbackEvent(type=context)",
				Details:    "From gpt-4 (8K) to gpt-4-turbo (128K)",
			},
			{
				TimeOffset: "0.5s",
				Event:      "Request to OpenAI (gpt-4-turbo, 128K context)",
				Action:     "Send same request to larger model",
			},
			{
				TimeOffset: "3s",
				Event:      "Response: HTTP 200 OK, streaming",
				Action:     "Success with large context model",
			},
		},
		Outcome: "SUCCESS via CONTEXT FALLBACK - Request exceeded primary model's context limit, " +
			"automatically switched to large context fallback model. " +
			"Total time: ~3s. No retries needed, just model switch.",
	}
}

// =============================================================================
// SCENARIO: STREAM INTERRUPTED WITH PARTIAL RECOVERY
// =============================================================================

// GetScenarioStreamInterrupted returns the stream interruption scenario
func GetScenarioStreamInterrupted() ScenarioRateLimitedRecovery {
	return ScenarioRateLimitedRecovery{
		Description: "Streaming response is interrupted mid-generation. System records partial " +
			"content, retries from beginning, and eventually succeeds.",
		Timeline: []ScenarioTimelineEvent{
			{
				TimeOffset: "0s",
				Event:      "Request sent, streaming starts",
				Action:     "StreamRecoveryManager.StartSession()",
			},
			{
				TimeOffset: "5s",
				Event:      "Received 5000 tokens (50% of expected)",
				Action:     "StreamRecoveryManager.RecordChunk() - checkpoint created",
				Details:    "ContentHash: abc123..., TokenCount: 5000",
			},
			{
				TimeOffset: "7s",
				Event:      "Connection dropped unexpectedly",
				Action:     "Classify as FailureTypeStreamInterrupted",
				Details:    "Retryable error, partial content saved",
			},
			{
				TimeOffset: "7.1s",
				Event:      "Save partial response",
				Action:     "StreamRecoveryManager.EndSession(interrupted)",
				Details:    "PartialTokens: 5000, PartialContentHash: abc123...",
			},
			{
				TimeOffset: "7.2s",
				Event:      "Apply retry policy",
				Action:     "PolicyStreamInterrupted: delay = 1s",
			},
			{
				TimeOffset: "7.3s",
				Event:      "Journal retry attempt",
				Action:     "Journal.AppendRetryAttempt(hasPartialResponse: true)",
			},
			{
				TimeOffset: "8.3s",
				Event:      "Retry from beginning",
				Action:     "Send full request again (no checkpoint resume support)",
				Details:    "Note: Most LLM APIs don't support mid-stream resumption",
			},
			{
				TimeOffset: "15s",
				Event:      "Complete response received",
				Action:     "Success on retry",
			},
		},
		Outcome: "SUCCESS after STREAM RECOVERY - Stream was interrupted at 50% completion, " +
			"partial content was recorded for debugging, request was retried from start. " +
			"Total time: ~15s. Partial content available in recovery info for analysis.",
	}
}

// =============================================================================
// SCENARIO: AUTHENTICATION FAILURE (FAIL FAST)
// =============================================================================

// GetScenarioAuthFailure returns the authentication failure scenario
func GetScenarioAuthFailure() ScenarioRateLimitedRecovery {
	return ScenarioRateLimitedRecovery{
		Description: "User's API key is invalid. System immediately fails without retry " +
			"and provides clear instructions for resolution.",
		Timeline: []ScenarioTimelineEvent{
			{
				TimeOffset: "0s",
				Event:      "Request to OpenAI",
				Action:     "Send request with invalid API key",
			},
			{
				TimeOffset: "0.1s",
				Event:      "Response: HTTP 401 Unauthorized",
				Action:     "Classify as FailureTypeAuthInvalid",
				Details:    "Non-retryable, requires user action",
			},
			{
				TimeOffset: "0.2s",
				Event:      "Fail fast - no retry",
				Action:     "Skip circuit breaker (client error, not provider issue)",
			},
			{
				TimeOffset: "0.3s",
				Event:      "Generate error report",
				Action:     "Create UnrecoverableError with resolution steps",
				Details: "Category: authentication, " +
					"Action: Update OPENAI_API_KEY environment variable, " +
					"Link: https://platform.openai.com/api-keys",
			},
			{
				TimeOffset: "0.4s",
				Event:      "Return to user",
				Action:     "Display error with clear next steps",
			},
		},
		Outcome: "FAILED (user action required) - Invalid API key detected immediately. " +
			"No retries attempted (would all fail). User notified with specific instructions " +
			"to update their API key. Total time: ~0.4s.",
	}
}

// =============================================================================
// RETRY POLICY EXAMPLES
// =============================================================================

// RetryPolicyExample demonstrates a specific retry policy
type RetryPolicyExample struct {
	PolicyName  string          `json:"policyName"`
	FailureType string          `json:"failureType"`
	Scenario    string          `json:"scenario"`
	Delays      []time.Duration `json:"delays"`
	Notes       string          `json:"notes"`
}

// GetRetryPolicyExamples returns examples of retry delays for each policy
func GetRetryPolicyExamples() []RetryPolicyExample {
	return []RetryPolicyExample{
		{
			PolicyName:  "PolicyRateLimit",
			FailureType: "rate_limit (HTTP 429)",
			Scenario:    "OpenAI returns 429 without Retry-After header",
			Delays: []time.Duration{
				1 * time.Second,  // Attempt 2: 1s * 2^0 = 1s
				2 * time.Second,  // Attempt 3: 1s * 2^1 = 2s
				4 * time.Second,  // Attempt 4: 1s * 2^2 = 4s
				8 * time.Second,  // Attempt 5: 1s * 2^3 = 8s
			},
			Notes: "With Retry-After: 30s header, all delays would be ~30s instead. " +
				"Max 5 attempts, max 60s delay cap.",
		},
		{
			PolicyName:  "PolicyOverloaded",
			FailureType: "overloaded (HTTP 503)",
			Scenario:    "Anthropic returns 503 Service Unavailable",
			Delays: []time.Duration{
				5 * time.Second,   // Attempt 2: 5s * 2^0 = 5s
				10 * time.Second,  // Attempt 3: 5s * 2^1 = 10s
				20 * time.Second,  // Attempt 4: 5s * 2^2 = 20s
				40 * time.Second,  // Attempt 5: 5s * 2^3 = 40s
			},
			Notes: "Longer initial delay (5s) since overload typically needs time to clear. " +
				"Max 120s delay, tryFallbackFirst=true for early fallback.",
		},
		{
			PolicyName:  "PolicyServerError",
			FailureType: "server_error (HTTP 500/502/504)",
			Scenario:    "Provider returns 500 Internal Server Error",
			Delays: []time.Duration{
				1 * time.Second, // Attempt 2
				2 * time.Second, // Attempt 3
			},
			Notes: "Quick retries (max 3 attempts) since 5xx errors are often transient. " +
				"tryFallbackFirst=true to fail fast to fallback.",
		},
		{
			PolicyName:  "PolicyTimeout",
			FailureType: "timeout",
			Scenario:    "Request times out after 60s",
			Delays: []time.Duration{
				0, // Immediate retry
			},
			Notes: "Only 2 attempts total. Immediate retry since timeout likely " +
				"means request was lost, not that provider is overloaded.",
		},
		{
			PolicyName:  "PolicyStreamInterrupted",
			FailureType: "stream_interrupted",
			Scenario:    "Connection drops mid-stream",
			Delays: []time.Duration{
				1 * time.Second,   // Attempt 2
				1500 * time.Millisecond, // Attempt 3 (1.5x multiplier)
			},
			Notes: "Quick retries with slight backoff. Max 2 attempts since " +
				"if network is down, additional retries won't help.",
		},
	}
}

// =============================================================================
// CIRCUIT BREAKER EXAMPLES
// =============================================================================

// CircuitBreakerExample demonstrates circuit breaker behavior
type CircuitBreakerExample struct {
	Scenario    string   `json:"scenario"`
	Events      []string `json:"events"`
	FinalState  string   `json:"finalState"`
	NextAction  string   `json:"nextAction"`
}

// GetCircuitBreakerExamples returns examples of circuit breaker behavior
func GetCircuitBreakerExamples() []CircuitBreakerExample {
	return []CircuitBreakerExample{
		{
			Scenario: "Provider experiences brief outage",
			Events: []string{
				"Request 1: 503 (consecutive=1, state=closed)",
				"Request 2: 503 (consecutive=2, state=closed)",
				"Request 3: 200 OK (consecutive=0, state=closed)",
			},
			FinalState: "closed (normal operation)",
			NextAction: "Continue sending requests normally",
		},
		{
			Scenario: "Provider has prolonged outage",
			Events: []string{
				"Request 1: 503 (consecutive=1, state=closed)",
				"Request 2: 503 (consecutive=2, state=closed)",
				"Request 3: 503 (consecutive=3, state=closed)",
				"Request 4: 503 (consecutive=4, state=closed)",
				"Request 5: 503 (consecutive=5, state=OPEN)",
			},
			FinalState: "OPEN (blocking requests)",
			NextAction: "Immediate fallback to alternative provider. " +
				"After 30s, circuit transitions to half-open for testing.",
		},
		{
			Scenario: "Provider recovers after outage",
			Events: []string{
				"Circuit was OPEN for 30s",
				"Circuit transitions to HALF-OPEN",
				"Test request 1: 200 OK (halfOpenSuccesses=1)",
				"Test request 2: 200 OK (halfOpenSuccesses=2)",
				"Circuit transitions to CLOSED",
			},
			FinalState: "closed (normal operation restored)",
			NextAction: "Resume normal operation with provider",
		},
		{
			Scenario: "Provider recovery fails",
			Events: []string{
				"Circuit was OPEN for 30s",
				"Circuit transitions to HALF-OPEN",
				"Test request 1: 503 (failure during testing)",
				"Circuit immediately transitions back to OPEN",
			},
			FinalState: "OPEN (still blocking)",
			NextAction: "Continue using fallback. Wait another 30s before retesting.",
		},
	}
}

// =============================================================================
// JOURNAL ENTRY EXAMPLES
// =============================================================================

// JournalEntryExample shows how errors appear in the run journal
type JournalEntryExample struct {
	EntryType   string                 `json:"entryType"`
	Description string                 `json:"description"`
	ExampleData map[string]interface{} `json:"exampleData"`
}

// GetJournalEntryExamples returns examples of journal entries for errors
func GetJournalEntryExamples() []JournalEntryExample {
	return []JournalEntryExample{
		{
			EntryType:   "retry_attempt",
			Description: "Recorded when a retry is about to be attempted",
			ExampleData: map[string]interface{}{
				"attemptNumber":   2,
				"totalAttempts":   1,
				"failureType":     "rate_limit",
				"errorMessage":    "Rate limit exceeded",
				"provider":        "openai",
				"model":           "gpt-4",
				"policyUsed":      "rate_limit",
				"delayMs":         30500,
				"willRetry":       true,
				"retryable":       true,
				"idempotencyKey":  "abc123...",
			},
		},
		{
			EntryType:   "retry_exhaust",
			Description: "Recorded when all retry attempts have been exhausted",
			ExampleData: map[string]interface{}{
				"totalAttempts":   5,
				"totalDurationMs": 75000,
				"failureTypes":    []string{"overloaded", "overloaded", "overloaded", "overloaded", "overloaded"},
				"finalError":      "Service temporarily unavailable",
				"provider":        "anthropic",
				"model":           "claude-3-sonnet",
				"fallbackUsed":    true,
				"fallbackType":    "provider",
				"resolution":      "fallback_success",
			},
		},
		{
			EntryType:   "circuit_event",
			Description: "Recorded when circuit breaker state changes",
			ExampleData: map[string]interface{}{
				"provider":       "anthropic",
				"oldState":       "closed",
				"newState":       "open",
				"triggerReason":  "consecutive failures threshold exceeded",
				"consecFailures": 5,
				"recentFailures": 7,
			},
		},
		{
			EntryType:   "fallback_event",
			Description: "Recorded when fallback is activated",
			ExampleData: map[string]interface{}{
				"fromProvider": "anthropic",
				"toProvider":   "openrouter",
				"fromModel":    "claude-3-sonnet",
				"toModel":      "claude-3-sonnet",
				"fallbackType": "provider",
				"reason":       "Circuit breaker open for anthropic",
				"failureType":  "overloaded",
				"success":      true,
			},
		},
	}
}
