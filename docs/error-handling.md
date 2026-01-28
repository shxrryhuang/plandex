# Error Handling & Retry Strategy

This document describes how Plandex classifies, retries, and reports errors
from external providers (LLMs, shell commands, file operations) during plan
execution.

---

## Failure Categories

Every error that reaches the retry engine is classified into one of three
categories:

| Category | Meaning | Default Behaviour |
|----------|---------|-------------------|
| **Retryable** | Transient; the same request may succeed on the next attempt | Retry with exponential backoff |
| **Non-retryable** | The request will fail identically if repeated | Fail fast; surface to user |
| **Conditional** | May be retryable depending on context (e.g. provider fallback available) | Try fallback first; then retry or fail |

---

## Failure Type Reference

### Retryable Failures

| Type | HTTP | Strategy | Notes |
|------|------|----------|-------|
| `rate_limit` | 429 | 5 attempts, 1–60 s backoff ×2, respects Retry-After | Most common transient error |
| `overloaded` | 503, 529 | 5 attempts, 5–120 s backoff ×2, tries fallback | Server under load |
| `server_error` | 5xx | 3 attempts, 1–30 s backoff ×2, tries fallback | Infrastructure issue |
| `timeout` | 504 | 2 attempts, immediate retry | Request took too long |
| `connection_error` | — | 3 attempts, 500 ms–5 s backoff ×1.5 | Network blip |
| `stream_interrupted` | — | 2 attempts, 1–5 s backoff ×1.5 | Mid-stream disconnect |
| `cache_error` | — | 1 retry, no delay | Retry without cache params |
| `provider_unavailable` | 502 | 3 attempts, 1–10 s backoff ×2, tries fallback | Provider routing failure |

### Non-Retryable Failures (Fail Fast)

| Type | HTTP | Required User Action |
|------|------|---------------------|
| `auth_invalid` | 401 | Fix API credentials (`plandex api-keys`) |
| `permission_denied` | 403 | Request access in provider console |
| `context_too_long` | 400, 413 | Reduce context (`plandex context rm`) or switch model |
| `invalid_request` | 400 | Fix request format |
| `content_policy` | 400 | Modify prompt |
| `quota_exhausted` | 429, 402 | Add credits or upgrade plan |
| `model_not_found` | 404 | Use a valid model ID |
| `model_deprecated` | — | Migrate to a newer model |
| `unsupported_feature` | 501 | Change approach |
| `account_suspended` | 403 | Contact provider |

### Conditional Failures

| Type | Behaviour |
|------|-----------|
| `billing_error` | Retry if payment may have processed; otherwise fail |
| `provider_unavailable` | Switch to fallback provider if configured |

---

## Retry Policy Configuration

The retry policy is controlled by `RetryConfig` (defined in
`app/shared/retry_config.go`). Sensible defaults are loaded automatically; all
values can be overridden via environment variables.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PLANDEX_MAX_RETRY_ATTEMPTS` | 0 (no global cap) | Hard cap on total attempts for any single operation |
| `PLANDEX_MAX_RETRY_DELAY_MS` | 0 (no global cap) | Maximum delay between retries in milliseconds |
| `PLANDEX_MAX_PROVIDER_RETRY_AFTER_MS` | 10000 | If a provider's Retry-After exceeds this, treat as non-retryable |
| `PLANDEX_RETRY_IRREVERSIBLE` | false | Allow retrying irreversible operations (destructive commands) |

### Programmatic Overrides

```go
cfg := shared.DefaultRetryConfig()
cfg.Overrides = map[shared.FailureType]*shared.RetryStrategy{
    shared.FailureTypeRateLimit: {
        ShouldRetry:       true,
        MaxAttempts:       2,       // Only 2 attempts for rate limits
        InitialDelayMs:    2000,    // Start at 2 s
        MaxDelayMs:        10000,   // Cap at 10 s
        BackoffMultiplier: 1.5,
        UseJitter:         true,
        RespectRetryAfter: true,
    },
}
```

---

## Operation Safety & Idempotency Guard

Before retrying, the system checks whether the operation is safe to
re-execute:

| Safety Level | Examples | Retry Allowed? |
|-------------|----------|----------------|
| **Safe** | Model requests, file reads, context loads | Always |
| **Conditional** | File writes/edits (rollback-able via checkpoint) | Yes, after rollback |
| **Irreversible** | Shell commands, external API writes, deploys | Only if `PLANDEX_RETRY_IRREVERSIBLE=true` |

This prevents the retry engine from accidentally duplicating destructive side
effects.  The classification is automatic — `ClassifyOperation()` maps
well-known operation type strings to safety levels.

---

## Backoff Algorithm

Delay between retries is computed as:

```
delay = initialDelayMs × backoffMultiplier^attemptNumber
```

With **full jitter** (when enabled):

```
delay = random(0, delay)
```

The result is clamped to `[0, effectiveMaxDelayMs]`.

If the provider supplies a `Retry-After` header, that value (plus 10% padding)
is used instead, subject to the `MaxProviderRetryAfterMs` ceiling.

---

## Fallback Chain

When a retryable error persists past `MAX_RETRIES_WITHOUT_FALLBACK` (3), or a
non-retryable error occurs with a fallback configured, the system cascades:

1. **LargeContextFallback** — for `context_too_long` errors, switch to a model
   with a larger context window.
2. **ErrorFallback** — for other non-retryable errors, switch to the
   configured error-fallback model.
3. **ProviderFallback** — if no model-level fallback exists, switch to a
   different provider (OpenRouter preferred for its internal routing).

After a fallback switch, the retry counter resets and the system gets
`MAX_ADDITIONAL_RETRIES_WITH_FALLBACK` (1) additional attempts on the new
target.

---

## Structured Error Reporting

Every failure that exhausts retries or triggers an unrecoverable condition is
recorded as an `ErrorReport` containing:

- **Root Cause** — category, type, HTTP code, provider, original message
- **Step Context** — plan ID, journal entry, execution phase, model context
- **Recovery Action** — auto-recovery plan or manual actions with CLI commands
- **Retry History** — all attempts with timing, delays, and fallback usage

Reports are persisted in the `ErrorRegistry` (ring buffer, max 100 entries,
atomic file writes) and linked to the `RunJournal` entry where the failure
occurred.

### Querying Errors

```go
// List unresolved provider errors from the last hour
errors := shared.ListErrors(shared.ErrorFilter{
    Category: shared.ErrorCategoryProvider,
    Resolved: boolPtr(false),
    Since:    timePtr(time.Now().Add(-time.Hour)),
})
```

---

## Walkthrough Scenarios

### Scenario 1: Rate Limit → Backoff → Success

```
Attempt 1: POST /v1/chat/completions → HTTP 429 "Rate limit reached"
  ├─ Classified as: rate_limit (retryable)
  ├─ Strategy: 5 attempts, 1–60 s backoff ×2, respect Retry-After
  ├─ Provider Retry-After: 12 s → delay = 12 × 1.1 = 13.2 s
  └─ Sleeping 13.2 s...

Attempt 2: POST /v1/chat/completions → HTTP 200 OK
  └─ Succeeded after 2 attempts
  └─ RetryContext.Summary(): "succeeded after 2 attempt(s)"
```

### Scenario 2: Quota Exhausted → Unrecoverable

```
Attempt 1: POST /v1/chat/completions → HTTP 429 "exceeded your current quota"
  ├─ Classified as: quota_exhausted (non-retryable)
  ├─ DetectUnrecoverableCondition → UnrecoverableQuotaExhausted
  ├─ ErrorReport stored in registry (tag: unrecoverable)
  └─ Error surfaced to user:

      ╔═══════════════════════════════════════════════════════════╗
      ║                UNRECOVERABLE ERROR                        ║
      ╚═══════════════════════════════════════════════════════════╝

      ▌ WHAT HAPPENED
      │ You exceeded your current quota...

      ▌ REQUIRED ACTIONS
      │ 1. [CRITICAL] Add credits to your account
      │ 2. [HIGH]     Switch to a different provider
      │    └─ Run: plandex providers switch <provider>
```

### Scenario 3: Stream Interrupted → Retry → Fallback → Success

```
Attempt 1: Streaming gpt-4 → connection dropped mid-stream (42% received)
  ├─ Classified as: stream_interrupted (retryable)
  ├─ Strategy: 2 attempts, 1–5 s backoff ×1.5
  └─ Sleeping 1.4 s...

Attempt 2: Streaming gpt-4 → HTTP 503 "overloaded"
  ├─ Classified as: overloaded (retryable)
  ├─ numTotalRetry (2) > MAX_RETRIES_WITHOUT_FALLBACK (3)? No
  ├─ Strategy: 5 attempts, 5–120 s backoff ×2, tries fallback
  └─ Sleeping 7.2 s...

Attempt 3: Streaming gpt-4 → HTTP 503 again
  ├─ numTotalRetry (3) >= MAX_RETRIES_WITHOUT_FALLBACK (3)
  ├─ Fallback engaged: switching to ErrorFallback (claude-3-haiku)
  ├─ numFallbackRetry reset to 0
  └─ Retrying on fallback model...

Attempt 4: Streaming claude-3-haiku → HTTP 200 OK (full response)
  └─ Succeeded after 4 attempts
  └─ RetryContext.Summary(): "succeeded after 4 attempt(s)"
```

### Scenario 4: Irreversible Operation Blocked

```
Operation: shell_exec "rm -rf build/"
  ├─ ClassifyOperation("shell_exec") → OperationIrreversible
  ├─ IsOperationSafe(Irreversible, config) → false
  └─ Error: "operation is irreversible; retry not permitted"
  └─ User must set PLANDEX_RETRY_IRREVERSIBLE=true to allow
```

---

## Run Journal Integration

Each retry attempt is recorded in the `RunJournal` via:

- `RecordRetryAttempt(entryIndex, RetryRecord)` — logs each attempt with
  failure type, delay, and fallback info
- `RecordRetryOutcome(entryIndex, succeeded, totalAttempts)` — finalises the
  entry as completed (if retry succeeded) or failed (if exhausted)

The retry trace is stored in the journal entry's `StackTrace` field as
newline-delimited JSON records, making it machine-parseable for post-mortem
analysis.

---

## File Locations

| Component | File |
|-----------|------|
| Failure taxonomy & examples | `app/shared/provider_failures.go` |
| Retry policy config | `app/shared/retry_config.go` |
| Operation safety guard | `app/shared/operation_safety.go` |
| Retry context & attempt tracking | `app/shared/retry_context.go` |
| Error report model | `app/shared/error_report.go` |
| Error registry (persistence) | `app/shared/error_registry.go` |
| Unrecoverable error definitions | `app/shared/unrecoverable_errors.go` |
| Run journal (execution log) | `app/shared/run_journal.go` |
| Model error classifier | `app/server/model/model_error.go` |
| Streaming retry loop | `app/server/model/client_stream.go` |
| Retry constants & config init | `app/server/model/client.go` |
