# Provider Failure Classification

This document classifies AI provider failures into retryable vs non-retryable categories with concrete examples.

## Quick Reference

### Retryable Failures

| Failure Type | HTTP Code | Retry Strategy | Max Attempts |
|--------------|-----------|----------------|--------------|
| **Rate Limit** | 429 | Exponential backoff, respect Retry-After | 5 |
| **Overloaded** | 503, 529 | Exponential backoff, consider fallback | 5 |
| **Server Error** | 500, 502 | Exponential backoff | 3 |
| **Timeout** | 504 | Immediate retry | 2 |
| **Connection Error** | - | Short backoff | 3 |
| **Stream Interrupted** | - | Retry from start | 2 |

### Non-Retryable Failures

| Failure Type | HTTP Code | Required Action |
|--------------|-----------|-----------------|
| **Auth Invalid** | 401 | Fix API credentials |
| **Permission Denied** | 403 | Request access to resource |
| **Context Too Long** | 400, 413 | Reduce input size |
| **Invalid Request** | 400 | Fix request format |
| **Content Policy** | 400 | Modify content |
| **Quota Exhausted** | 402, 429* | Add credits/upgrade plan |
| **Model Not Found** | 404 | Use valid model ID |
| **Unsupported Feature** | 501 | Change approach |

*Note: 429 can be rate limit (retryable) OR quota exhausted (non-retryable). Check message content.

---

## Detailed Examples by Provider

### OpenAI

#### Retryable

```json
// Rate Limit (429)
{
  "error": {
    "message": "Rate limit reached for gpt-4 in organization org-xxx on requests per min (RPM): Limit 10, Used 10, Requested 1.",
    "type": "requests",
    "code": "rate_limit_exceeded"
  }
}
// Action: Wait and retry with exponential backoff
```

```json
// Server Error (500)
{
  "error": {
    "message": "The server had an error while processing your request. Sorry about that!",
    "type": "server_error",
    "code": "server_error"
  }
}
// Action: Retry with backoff
```

```json
// Overloaded (503)
{
  "error": {
    "message": "The engine is currently overloaded, please try again later.",
    "type": "server_error",
    "code": "overloaded"
  }
}
// Action: Wait 5-30 seconds and retry
```

#### Non-Retryable

```json
// Invalid API Key (401)
{
  "error": {
    "message": "Incorrect API key provided: sk-xxx. You can find your API key at https://platform.openai.com/account/api-keys.",
    "type": "invalid_request_error",
    "code": "invalid_api_key"
  }
}
// Action: User must provide valid API key
```

```json
// Context Too Long (400)
{
  "error": {
    "message": "This model's maximum context length is 8192 tokens. However, your messages resulted in 12847 tokens. Please reduce the length of the messages.",
    "type": "invalid_request_error",
    "code": "context_length_exceeded"
  }
}
// Action: Reduce input size or use larger context model
```

```json
// Quota Exhausted (429 - NON-RETRYABLE!)
{
  "error": {
    "message": "You exceeded your current quota, please check your plan and billing details.",
    "type": "insufficient_quota",
    "code": "insufficient_quota"
  }
}
// Action: Add credits or upgrade plan - DO NOT RETRY
```

```json
// Content Policy (400)
{
  "error": {
    "message": "Your request was rejected as a result of our safety system.",
    "type": "invalid_request_error",
    "code": "content_policy_violation"
  }
}
// Action: Modify the prompt
```

---

### Anthropic

#### Retryable

```json
// Rate Limit (429)
{
  "type": "error",
  "error": {
    "type": "rate_limit_error",
    "message": "Number of request tokens has exceeded your per-minute rate limit"
  }
}
// Action: Check retry-after header, usually 60 seconds or less
```

```json
// Overloaded (529) - Anthropic-specific code
{
  "type": "error",
  "error": {
    "type": "overloaded_error",
    "message": "Anthropic's API is temporarily overloaded"
  }
}
// Action: Retry after 10-60 seconds
```

```json
// Internal Error (500)
{
  "type": "error",
  "error": {
    "type": "api_error",
    "message": "An unexpected error has occurred internal to Anthropic's systems"
  }
}
// Action: Retry with exponential backoff
```

#### Non-Retryable

```json
// Invalid API Key (401)
{
  "type": "error",
  "error": {
    "type": "authentication_error",
    "message": "Invalid API Key"
  }
}
// Action: Provide valid API key
```

```json
// Permission Denied (403)
{
  "type": "error",
  "error": {
    "type": "permission_error",
    "message": "Your API key does not have permission to use the specified resource"
  }
}
// Action: Request access or use allowed models
```

```json
// Context Too Long (400)
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "prompt is too long: 234567 tokens > 200000 maximum"
  }
}
// Action: Reduce input or use model with larger context
```

---

### Google (Vertex AI / Gemini)

#### Retryable

```json
// Per-Minute Rate Limit (429)
{
  "error": {
    "code": 429,
    "message": "Quota exceeded for aiplatform.googleapis.com/generate_content_requests_per_minute",
    "status": "RESOURCE_EXHAUSTED"
  }
}
// Action: Wait and retry - this is a rate limit, not quota exhaustion
```

```json
// Service Unavailable (503)
{
  "error": {
    "code": 503,
    "message": "The model is temporarily unavailable. Please try again later.",
    "status": "UNAVAILABLE"
  }
}
// Action: Retry with backoff
```

#### Non-Retryable

```json
// Authentication Failed (401)
{
  "error": {
    "code": 401,
    "message": "Request had invalid authentication credentials",
    "status": "UNAUTHENTICATED"
  }
}
// Action: Fix service account or API key
```

```json
// Daily Quota Exhausted (429 - NON-RETRYABLE!)
{
  "error": {
    "code": 429,
    "message": "Quota exceeded for aiplatform.googleapis.com/base_model_generate_content_requests_per_day",
    "status": "RESOURCE_EXHAUSTED"
  }
}
// Action: Wait for daily reset or increase quota - DO NOT RETRY
```

```json
// Content Blocked (400)
{
  "error": {
    "code": 400,
    "message": "User input or prompt contains blocked content",
    "status": "INVALID_ARGUMENT"
  }
}
// Action: Modify content
```

---

### Azure OpenAI

#### Retryable

```
// Rate Limit (429)
HTTP 429
"Requests to the ChatCompletions_Create Operation have exceeded rate limit. Try again in 59 seconds."
// Action: Parse retry time from message and wait
```

```
// Service Unavailable (503)
HTTP 503
"The service is temporarily unable to process your request. Please try again later."
// Action: Retry with backoff
```

#### Non-Retryable

```
// Invalid Subscription (401)
HTTP 401
"Access denied due to invalid subscription key or wrong API endpoint."
// Action: Check subscription key and endpoint URL
```

```
// Deployment Not Found (404)
HTTP 404
"The API deployment for this resource does not exist."
// Action: Fix deployment name or deploy the model
```

```
// Content Filter (400)
HTTP 400
"The response was filtered due to the prompt triggering Azure OpenAI's content management policy."
// Action: Modify prompt - Azure filters are stricter than OpenAI
```

---

### OpenRouter

#### Retryable

```json
// Rate Limit (429)
{
  "error": {
    "code": "rate_limit",
    "message": "Rate limit exceeded. Please slow down your requests."
  }
}
// Action: Retry - OpenRouter may automatically failover to different provider
```

```json
// Provider Error (502)
{
  "error": {
    "code": "provider_returned_error",
    "message": "The upstream provider returned an error. OpenRouter may automatically retry with a different provider."
  }
}
// Action: Retry - request may be routed to different provider
```

#### Non-Retryable

```json
// Insufficient Credits (402)
{
  "error": {
    "code": "insufficient_credits",
    "message": "Insufficient credits. Please add credits at openrouter.ai/account."
  }
}
// Action: Add credits to account
```

```json
// Model Not Found (404)
{
  "error": {
    "code": "model_not_found",
    "message": "Model 'nonexistent/model' not found"
  }
}
// Action: Use valid model ID from OpenRouter catalog
```

---

## Distinguishing 429 Errors

The HTTP 429 status code is especially tricky because it can mean two different things:

### Rate Limit (Retryable)
- Temporary throttling
- Will succeed after waiting
- Keywords: "rate limit", "too many requests", "per minute", "per second", "RPM", "TPM"

### Quota Exhausted (Non-Retryable)
- Account limit reached
- Requires action to fix
- Keywords: "quota", "exceeded your current quota", "insufficient credits", "billing", "per day"

### Decision Logic

```go
if httpCode == 429 {
    if contains(message, "per_minute") || contains(message, "per_second") {
        return RETRYABLE  // Rate limit
    }
    if contains(message, "exceeded your current quota") ||
       contains(message, "insufficient") {
        return NON_RETRYABLE  // Quota exhausted
    }
    return RETRYABLE  // Default to rate limit
}
```

---

## Retry Strategy Recommendations

### For Rate Limits

```
Initial delay: 1 second
Max delay: 60 seconds
Multiplier: 2.0
Max attempts: 5
Jitter: Yes (add randomness)
Respect Retry-After: Yes
```

### For Server Errors

```
Initial delay: 1 second
Max delay: 30 seconds
Multiplier: 2.0
Max attempts: 3
Jitter: Yes
Consider fallback: Yes
```

### For Overloaded Errors

```
Initial delay: 5 seconds
Max delay: 120 seconds
Multiplier: 2.0
Max attempts: 5
Jitter: Yes
Respect Retry-After: Yes
Consider fallback: Yes
```

### For Timeouts

```
Initial delay: 0 (immediate)
Max attempts: 2
Consider: Increasing timeout, using streaming, reducing input
```

---

## Fallback Strategy

When errors persist, consider provider fallback:

1. **Rate limits**: Wait first, fallback if still failing
2. **Overloaded**: Try fallback immediately (different provider may not be overloaded)
3. **Server errors**: Try fallback after 1-2 failures
4. **Context too long**: Fallback to model with larger context (e.g., GPT-4 → GPT-4-Turbo-128k)

### Fallback Order Example

```
Primary: OpenAI GPT-4
├─ Error Fallback: Anthropic Claude 3.5 Sonnet
├─ Context Fallback: OpenAI GPT-4-Turbo-128k
└─ Provider Fallback: OpenRouter (automatic routing)
```

---

## Configurable Retry Policy

All retry behaviour is governed by `RetryConfig` (`app/shared/retry_config.go`).
Sensible defaults match the strategies documented above; every value is
overridable at runtime.

### Environment Variables

| Variable | Default | Effect |
|----------|---------|--------|
| `PLANDEX_MAX_RETRY_ATTEMPTS` | 0 (no cap) | Hard cap on total attempts for any operation |
| `PLANDEX_MAX_RETRY_DELAY_MS` | 0 (no cap) | Maximum delay between retries (ms) |
| `PLANDEX_MAX_PROVIDER_RETRY_AFTER_MS` | 10000 | If a provider's Retry-After exceeds this, treat as non-retryable |
| `PLANDEX_RETRY_IRREVERSIBLE` | false | Allow retrying destructive / irreversible operations |

### Per-Type Overrides (programmatic)

```go
cfg := shared.DefaultRetryConfig()
cfg.Overrides = map[shared.FailureType]*shared.RetryStrategy{
    shared.FailureTypeRateLimit: {
        ShouldRetry:       true,
        MaxAttempts:       2,
        InitialDelayMs:    2000,
        MaxDelayMs:        10000,
        BackoffMultiplier: 1.5,
        UseJitter:         true,
        RespectRetryAfter: true,
    },
}
```

### Backoff Formula

```
delay = initialDelayMs × backoffMultiplier ^ attemptNumber
```

With **full jitter** enabled: `delay = random(0, delay)`.
Result is clamped to `[0, effectiveMaxDelayMs]`.

If the provider returns a `Retry-After` header, that value (+ 10 % padding)
is used instead, subject to `MaxProviderRetryAfterMs`.

---

## Operation Safety & Idempotency Guard

Before retrying, `withStreamingRetries` checks whether the operation is safe
to re-execute via `OperationSafety` (`app/shared/operation_safety.go`):

| Safety Level | Examples | Retry Allowed? |
|-------------|----------|----------------|
| **Safe** | Model requests, file reads, context loads | Always |
| **Conditional** | File writes/edits (checkpoint-backed rollback available) | Yes, after rollback |
| **Irreversible** | Shell exec, external API writes, deploys | Only if `PLANDEX_RETRY_IRREVERSIBLE=true` |

`ClassifyOperation()` maps well-known operation-type strings to safety levels
automatically.  Unknown types default to **Safe** so that model requests
(the most common case) work without explicit configuration.

---

## Structured Retry Tracking (RetryContext)

Every retry loop creates a `RetryContext` (`app/shared/retry_context.go`)
that accumulates a `[]RetryAttempt` record for each execution attempt.
Each attempt captures:

- Attempt number and wall-clock timing
- Classified `ModelError` and resolved `FailureType`
- The `RetryStrategy` that was applied
- Computed delay before the next attempt
- Whether a fallback provider/model was used

On final failure the context calls `DetectUnrecoverableCondition()` and,
if triggered, persists an `ErrorReport` (with full attempt history) into
the `ErrorRegistry`.  The run journal's `RecordRetryAttempt` / `RecordRetryOutcome`
helpers mirror the trace into the `JournalEntry` for post-mortem auditing.

---

## Implementation Reference

See `app/shared/provider_failures.go` for:
- `ClassifyProviderFailure()` - Main classification function
- `GetRetryStrategy()` - Get retry parameters for failure type
- `GetProviderFailureExamples()` - All documented examples

See `app/shared/retry_config.go` for:
- `RetryConfig` struct and `DefaultRetryConfig()`
- `LoadRetryConfigFromEnv()` - environment-driven configuration
- `ComputeBackoffDelay()` - exponential backoff with jitter and clamping

See `app/shared/operation_safety.go` for:
- `OperationSafety` enum (Safe / Conditional / Irreversible)
- `IsOperationSafe()` - retry permission check
- `ClassifyOperation()` - automatic operation-type classification

See `app/shared/retry_context.go` for:
- `RetryContext` and `RetryAttempt` structs
- `NewRetryContext()`, `CanRetry()`, `FinalizeWithError()`
- Bridge to `ErrorRegistry` via `StoreWithContext()`

See `app/shared/error_report.go` for:
- `FromProviderFailure()` - Create error report with recovery guidance
- `ErrorReport.Format()` - Human-readable error output
- Root cause, step context, and recovery action structure

See `app/shared/unrecoverable_errors.go` for:
- `GetUnrecoverableEdgeCases()` - Documented unrecoverable scenarios
- `DetectUnrecoverableCondition()` - Identify non-recoverable states (now called from retry loop)
- User communication templates with manual actions

See `app/shared/run_journal.go` for:
- `RecordRetryAttempt()` - log each retry attempt with structured metadata
- `RecordRetryOutcome()` - finalise journal entry as resolved or failed

See `app/shared/error_registry.go` for:
- `StoreWithContext()` - persist errors enriched with retry-context tags

See `app/shared/file_transaction.go` for:
- `FileTransaction` — wraps every patch in a WAL-backed transaction with persisted snapshots
- Sequential apply with reverse-order rollback on any write failure
- `RecoverTransaction()` — replays orphaned WAL on startup to restore pre-crash state
- Three-phase rollback: restore (pre-apply content) → remove (newly created files) → sweep (side-effect stragglers)

See `app/server/model/model_error.go` for:
- HTTP header parsing for Retry-After
- `ClassifyModelError()` and `ClassifyErrMsg()` now populate `ProviderFailureType`

See `app/server/model/client_stream.go` for:
- `withStreamingRetries()` — the live retry loop, now wired to `RetryConfig`,
  `RetryContext`, and `DetectUnrecoverableCondition()`

---

## Related Documentation

- [Error Handling & Retry Strategy](./error-handling.md) - Full walkthrough scenarios and configuration reference
- [System Design - Recovery Section](./SYSTEM_DESIGN.md#11-recovery--resilience-system) - Full architecture
- [Replay Safety](./replay-safety.md) - Safe replay and recovery modes
