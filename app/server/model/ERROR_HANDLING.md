# Error Handling and Retry Strategy

This document describes the comprehensive error handling system implemented for Plandex server.

## Overview

The error handling system provides resilient operation across all external providers (LLMs, tools, shell commands) through:

- **Unified error classification** - Consistent categorization of failures
- **Exponential backoff retry** - Smart retry policies per failure type
- **Circuit breaker** - Prevents cascading failures
- **Health monitoring** - Proactive provider health tracking
- **Graceful degradation** - Automatic quality reduction under load
- **Dead letter queue** - Failed operation storage for recovery
- **Idempotency tracking** - Safe retries without duplicate side effects
- **Stream recovery** - Partial response preservation

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    UNIFIED ERROR HANDLING SYSTEM                         │
├─────────────────────────────────────────────────────────────────────────┤
│  HTTP Response → ClassifyProviderFailure() → ProviderFailure            │
│                         ↓                                                │
│              ┌─────────────────────────────────────────┐                │
│              │       Error Classification              │                │
│              │   (rate_limit, overloaded, etc.)        │                │
│              └─────────────────────────────────────────┘                │
│                         ↓                                                │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │
│   │RetryPolicy  │  │CircuitBreak │  │HealthCheck  │  │Degradation  │   │
│   │(exp backoff)│  │(provider    │  │Manager      │  │Manager      │   │
│   │             │  │health)      │  │(monitoring) │  │(quality)    │   │
│   └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘   │
│                         ↓                                                │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                    │
│   │Idempotency  │  │StreamRecov  │  │DeadLetter   │                    │
│   │Manager      │  │Manager      │  │Queue        │                    │
│   │(safe retry) │  │(partial)    │  │(failed ops) │                    │
│   └─────────────┘  └─────────────┘  └─────────────┘                    │
└─────────────────────────────────────────────────────────────────────────┘
```

## Components

### 1. Retry Policy System

**File:** `/app/shared/retry_policy.go`

Configurable retry policies with exponential backoff.

```go
type RetryPolicy struct {
    MaxAttempts       int
    InitialDelay      time.Duration
    MaxDelay          time.Duration
    Multiplier        float64
    JitterFactor      float64
    RespectRetryAfter bool
}
```

**Pre-defined Policies:**

| Policy | Max Attempts | Initial Delay | Max Delay | Use Case |
|--------|-------------|---------------|-----------|----------|
| `PolicyRateLimit` | 5 | 30s | 5min | 429 errors, respects Retry-After |
| `PolicyOverloaded` | 4 | 15s | 2min | 503/529 errors |
| `PolicyServerError` | 3 | 5s | 1min | 500/502/504 errors |
| `PolicyTimeout` | 3 | 1s | 30s | Request timeouts |
| `PolicyStreamInterrupted` | 4 | 2s | 30s | Stream disconnections |

**Usage:**
```go
policy := shared.GetPolicyForFailure(failure.Type)
delay := policy.CalculateDelay(attemptNumber, retryAfterHint)
```

---

### 2. Circuit Breaker

**File:** `/app/server/model/circuit_breaker.go`

Prevents cascading failures by tracking provider health.

**States:**
- `CircuitClosed` - Normal operation, requests allowed
- `CircuitOpen` - Provider failing, requests blocked
- `CircuitHalfOpen` - Testing recovery, limited requests

**Configuration:**
```go
type CircuitBreakerConfig struct {
    FailureThreshold   int           // Failures before opening (default: 5)
    OpenDuration       time.Duration // Time before half-open (default: 30s)
    HalfOpenMaxAttempts int          // Test requests in half-open (default: 3)
    SuccessThreshold   int           // Successes to close (default: 2)
}
```

**Usage:**
```go
// Check before making request
if GlobalCircuitBreaker.IsOpen("openai") {
    return ErrCircuitOpen
}

// Record result
if success {
    GlobalCircuitBreaker.RecordSuccess("openai")
} else {
    GlobalCircuitBreaker.RecordFailure("openai", failure)
}
```

**Excluded Failure Types** (don't trigger circuit):
- `auth_invalid` - User configuration issue
- `permission_denied` - Access control issue
- `invalid_request` - Bad request format
- `content_policy` - Content violation

---

### 3. Health Check Manager

**File:** `/app/server/model/health_check.go`

Proactive provider health monitoring with latency tracking.

**Health Statuses:**
- `HealthStatusHealthy` - Score >= 80, low latency, high success rate
- `HealthStatusDegraded` - Score 50-79, elevated latency or errors
- `HealthStatusUnhealthy` - Score < 50, high failure rate
- `HealthStatusUnknown` - No data yet

**Tracked Metrics:**
```go
type ProviderHealth struct {
    Score                int           // 0-100 health score
    Status               HealthStatus
    ConsecutiveSuccesses int
    ConsecutiveFailures  int
    AvgLatencyMs         int64
    P95LatencyMs         int64
    P99LatencyMs         int64
    TotalRequests        int64
    FailedRequests       int64
}
```

**Usage:**
```go
// Record request outcome
GlobalHealthCheckManager.RecordRequest("openai", success, latencyMs, failure)

// Get best provider from list
best := GlobalHealthCheckManager.GetBestProvider([]string{"openai", "anthropic"})

// Get all healthy providers
healthy := GlobalHealthCheckManager.GetHealthyProviders()
```

---

### 4. Graceful Degradation Manager

**File:** `/app/server/model/graceful_degradation.go`

Automatic quality reduction when providers are under stress.

**Degradation Levels:**

| Level | Context Reduction | Timeout Multiplier | Features Disabled |
|-------|------------------|-------------------|-------------------|
| `None` | 0% | 1.0x | None |
| `Light` | 10% | 1.2x | None |
| `Moderate` | 25% | 1.5x | Caching |
| `Heavy` | 50% | 2.0x | Caching, streaming |
| `Critical` | 75% | 3.0x | Caching, streaming, parallel |

**Request Modifications:**
```go
type RequestModifications struct {
    MaxTokens        int
    ContextTokens    int
    TimeoutMs        int
    ShouldQueue      bool    // Queue non-urgent requests
    PreferredModel   string  // Faster model suggestion
    DisabledFeatures []string
}

// Get modifications for a request
mods := GlobalDegradationManager.GetRequestModifications("openai", maxTokens, timeout, isUrgent)
```

**Triggers:**
```go
// Manual trigger
GlobalDegradationManager.TriggerDegradation(DegradationModerate, "high error rate", "openai", 10*time.Minute)

// Automatic from failure rate
GlobalDegradationManager.TriggerFromFailure(failure, errorRatePercent)

// Recovery
GlobalDegradationManager.RecoverDegradation(degradationId)
GlobalDegradationManager.RecoverProvider("openai")
GlobalDegradationManager.RecoverAll()
```

---

### 5. Dead Letter Queue

**File:** `/app/server/model/dead_letter_queue.go`

Stores failed operations for later retry or manual intervention.

**Item Statuses:**
- `pending` - Waiting, no auto-retry scheduled
- `scheduled` - Auto-retry scheduled
- `processing` - Currently being retried
- `resolved` - Successfully completed
- `discarded` - Manually discarded
- `expired` - TTL exceeded

**Configuration:**
```go
type DLQConfig struct {
    MaxItems          int           // Max queue size (default: 1000)
    DefaultTTL        time.Duration // Item expiration (default: 7 days)
    AutoRetryEnabled  bool          // Enable auto-retry (default: true)
    AutoRetryDelay    time.Duration // Delay between retries (default: 1 hour)
    AutoRetryMaxCount int           // Max auto-retries (default: 3)
}
```

**Usage:**
```go
// Add failed operation
item := GlobalDeadLetterQueue.Add(
    "model_request",           // operation type
    "openai",                  // provider
    "gpt-4",                   // model
    planId,                    // plan ID
    requestData,               // serialized request
    failure,                   // failure details
    totalAttempts,             // attempts made
)

// Manual retry
GlobalDeadLetterQueue.MarkForRetry(itemId, 1*time.Hour)

// Process retry
item, _ := GlobalDeadLetterQueue.StartRetry(itemId)
// ... attempt operation ...
GlobalDeadLetterQueue.CompleteRetry(itemId, success, errorMsg)

// Manual resolution
GlobalDeadLetterQueue.Resolve(itemId, "fixed manually", "admin")
GlobalDeadLetterQueue.Discard(itemId, "no longer needed")

// Query
pending := GlobalDeadLetterQueue.GetPendingItems()
due := GlobalDeadLetterQueue.GetItemsDueForRetry()
byProvider := GlobalDeadLetterQueue.GetByProvider("openai")
```

---

### 6. Idempotency Manager

**File:** `/app/shared/idempotency.go`

Prevents duplicate side effects during retries.

**Record Statuses:**
- `pending` - Registered but not started
- `in_progress` - Currently executing
- `completed` - Finished successfully
- `failed` - Finished with error
- `rolled_back` - Changes were undone

**Usage:**
```go
manager := shared.NewIdempotencyManager(24 * time.Hour)

// Generate key
key := shared.GenerateIdempotencyKey(planId, branch, "apply_changes", params)

// Check before operation
result := manager.Check(key, requestData)
if !result.ShouldProceed {
    // Already completed or in progress
    return result.Record
}

// Start operation
record := manager.Start(key, requestData)

// Track file changes
manager.RecordFileChange(key, shared.FileChangeRecord{
    Path:      "/path/to/file",
    Operation: shared.IdempotentFileOpModify,
    BeforeHash: beforeHash,
    AfterHash:  afterHash,
})

// Complete
manager.Complete(key, success, result, err)
```

---

### 7. Stream Recovery Manager

**File:** `/app/server/model/stream_recovery.go`

Preserves partial streaming responses for recovery.

**Features:**
- Buffers content as it streams
- Creates checkpoints every ~1000 tokens
- Enables resume from last checkpoint on interruption

**Usage:**
```go
// Start session
session := GlobalStreamRecoveryManager.StartSession("openai", "gpt-4", requestHash)

// Record chunks as they arrive
GlobalStreamRecoveryManager.RecordChunk(session.Id, chunkContent, tokenCount)

// On interruption, get partial content
partial := GlobalStreamRecoveryManager.GetPartialContent(session.Id)

// End session
GlobalStreamRecoveryManager.EndSession(session.Id, success)
```

---

## Error Categories

### Retryable (with backoff)

| Type | HTTP Codes | Behavior |
|------|------------|----------|
| `rate_limit` | 429 | Respect Retry-After header |
| `overloaded` | 503, 529 | Exponential backoff |
| `server_error` | 500, 502, 504 | Exponential backoff |
| `timeout` | - | Quick retry |
| `connection_error` | - | Brief delay |
| `stream_interrupted` | - | Quick retry with partial content |

### Fail-Fast (no retry)

| Type | HTTP Codes | Required Action |
|------|------------|-----------------|
| `auth_invalid` | 401 | Fix API key |
| `permission_denied` | 403 | Fix permissions |
| `context_too_long` | 400, 413 | Reduce context or use fallback |
| `invalid_request` | 400 | Fix request format |
| `content_policy` | - | Modify content |
| `quota_exhausted` | - | Upgrade plan |
| `model_not_found` | 404 | Use valid model |
| `account_suspended` | - | Contact provider |

---

## Initialization

All components are initialized in `main.go`:

```go
// Initialize error handling infrastructure
model.InitGlobalCircuitBreaker()
model.InitGlobalStreamRecoveryManager()
model.InitGlobalHealthCheckManager()
model.InitGlobalDegradationManager()
model.InitGlobalDeadLetterQueue()
```

Shutdown hooks log final metrics:

```go
setup.RegisterShutdownHook(func() {
    log.Println("Circuit breaker final metrics:", model.GlobalCircuitBreaker.GetMetrics())
    log.Println("Stream recovery final stats:", model.GlobalStreamRecoveryManager.GetStats())
    log.Println("Health check final metrics:", model.GlobalHealthCheckManager.GetMetrics())
    log.Println("Degradation final metrics:", model.GlobalDegradationManager.GetMetrics())
    log.Println("Dead letter queue final stats:", model.GlobalDeadLetterQueue.GetStats())
})
```

---

## Testing

All components have comprehensive test coverage:

| Component | Tests | File |
|-----------|-------|------|
| Circuit Breaker | 13 | `circuit_breaker_test.go` |
| Dead Letter Queue | 15 | `dead_letter_queue_test.go` |
| Degradation Manager | 14 | `graceful_degradation_test.go` |
| Health Check Manager | 10 | `health_check_test.go` |
| Stream Recovery | 6 | `stream_recovery_test.go` |
| Retry Policy | - | `retry_policy_test.go` (in shared) |
| Idempotency | - | `idempotency_test.go` (in shared) |

Run tests:
```bash
go test ./model/... -run "Test(CircuitBreaker|DeadLetterQueue|Degradation|HealthCheck|StreamRecovery)"
```

---

## Files Reference

### New Files Created

| File | Description |
|------|-------------|
| `/app/shared/retry_policy.go` | Retry policies with exponential backoff |
| `/app/shared/idempotency.go` | Idempotency tracking for safe retries |
| `/app/server/model/circuit_breaker.go` | Circuit breaker for provider health |
| `/app/server/model/stream_recovery.go` | Partial streaming response recovery |
| `/app/server/model/health_check.go` | Proactive provider health monitoring |
| `/app/server/model/graceful_degradation.go` | Automatic quality reduction |
| `/app/server/model/dead_letter_queue.go` | Failed operation storage |

### Modified Files

| File | Changes |
|------|---------|
| `/app/shared/ai_models_errors.go` | Added `ProviderFailure` field to `ModelError` |
| `/app/server/model/model_error.go` | Refactored to use unified classification |
| `/app/server/main.go` | Added initialization for all components |

---

## Changelog

### 2026-01-28

**Bug Fixes:**
- Fixed `shared.NewUserError` undefined error in `dead_letter_queue.go` - replaced with `fmt.Errorf`
- Fixed unused variable `capturedOldStatus` in `health_check_test.go`
- Fixed DLQ item status: items with auto-retry enabled now correctly use `DLQStatusScheduled` instead of `DLQStatusPending`

**Test Fixes:**
- Updated `TestDeadLetterQueue_Add` to expect `scheduled` status (auto-retry enabled by default)
- Updated `TestDeadLetterQueue_GetPendingItems` to use config with `AutoRetryEnabled: false`

**Verification:**
- All 52 error handling tests pass
- Full build succeeds
- All server package tests pass
