# Concurrency Safety in Plandex

**Version:** 1.1
**Last Updated:** January 2026
**Status:** Implemented

---

## Implementation Status

| Component | Status | Location |
|-----------|--------|----------|
| Doctor Command | Implemented | `app/cli/cmd/doctor.go` |
| Doctor Handler | Implemented | `app/server/handlers/doctor.go` |
| Doctor Types | Implemented | `app/shared/doctor.go` |
| Concurrency Tests | Implemented | `app/server/db/concurrency_test.go` |
| Atomic File Locking | Implemented | `.claude/hooks/capture_session_event.py` |
| API Endpoint | Implemented | `POST /doctor` |
| CI Pipeline | Implemented | `.github/workflows/concurrency-tests.yml` |

---

## Table of Contents

1. [Overview](#1-overview)
2. [Single-Run Execution Assumptions](#2-single-run-execution-assumptions)
3. [Shared Mutable State](#3-shared-mutable-state)
4. [Failure Modes](#4-failure-modes)
5. [Concurrency Design](#5-concurrency-design)
6. [Debugging Concurrency Issues](#6-debugging-concurrency-issues)
7. [Testing Concurrency](#7-testing-concurrency)
8. [CLI Communication](#8-cli-communication)
9. [Implementation Checklist](#9-implementation-checklist)

---

## 1. Overview

### 1.1 Purpose

This document describes how Plandex handles concurrent operations, identifies potential failure modes, and provides guidance for debugging, testing, and communicating concurrency-related issues to users.

### 1.2 Concurrency Scope

Plandex handles concurrency at multiple levels:

| Level | Description | Mechanism |
|-------|-------------|-----------|
| **Process** | Multiple CLI instances | Database locks |
| **Thread** | Goroutines within server | Mutexes, channels |
| **Request** | Concurrent API requests | Queue system |
| **File** | Concurrent file access | fcntl locks |

### 1.3 Design Principles

1. **Explicit over implicit** - Concurrency behavior is documented and predictable
2. **Fail-safe** - On conflict, operations fail clearly rather than corrupt data
3. **Informative** - Users receive actionable feedback about concurrent operations
4. **Testable** - Concurrency scenarios are covered by automated tests

---

## 2. Single-Run Execution Assumptions

### 2.1 Components Assuming Single Execution

The following components were originally designed for single-run execution and require special handling for concurrent access:

#### 2.1.1 Plan Execution Engine

**Location:** `/app/server/model/plan/`

**Assumption:** Only one tell/build operation runs per plan at a time.

**Risk:** Multiple concurrent tells could interleave AI responses, corrupting conversation history.

**Mitigation:** Database locks with LockScopeWrite ensure exclusive access during plan execution.

```go
// Plan execution requires exclusive write lock
LockRepoParams{
    Scope: LockScopeWrite,  // Blocks all other operations
    Reason: "tell execution",
}
```

#### 2.1.2 Git Repository Operations

**Location:** `/app/server/db/git_repo.go`

**Assumption:** Git operations (checkout, commit, reset) are atomic within a single operation.

**Risk:** Concurrent branch checkouts could leave repository in inconsistent state.

**Mitigation:**
- Per-plan operation queue serializes git operations
- Lock file cleanup on lock acquisition
- Branch checkout happens after lock is acquired

#### 2.1.3 Context Loading

**Location:** `/app/cli/lib/context_load.go`

**Assumption:** Files being loaded don't change during load operation.

**Risk:** File contents could change between read and tokenization.

**Mitigation:**
- Read locks allow concurrent context reads on same branch
- File snapshots captured at load time

#### 2.1.4 Build Application

**Location:** `/app/cli/lib/apply.go`

**Assumption:** Target files don't change during apply operation.

**Risk:** Changes made by other processes could be overwritten.

**Mitigation:**
- CLI warns if file modification time changed
- Optional dry-run mode to preview changes
- File transaction system with rollback support

### 2.2 Session Capture Hooks

**Location:** `.claude/hooks/capture_session_event.py`

**Assumption:** Each session has a unique session ID.

**Risk:** Sessions with missing IDs could overwrite each other's logs.

**Mitigation:**
- Fallback session ID generation: `f"fallback_{uuid.uuid4().hex[:8]}"`
- Atomic writes with file locking should be used for shared log files

---

## 3. Shared Mutable State

### 3.1 Database State

| Table | Concurrent Access Pattern | Protection |
|-------|---------------------------|------------|
| `plans` | Read: Multiple, Write: Exclusive | Row-level locks |
| `branches` | Read: Multiple, Write: Exclusive | Row-level locks |
| `repo_locks` | Coordinated access | Transaction isolation |
| `convo_messages` | Append-only during tell | Plan-level write lock |
| `contexts` | Read during tell | Plan-level read lock |
| `plan_builds` | Write during build | Plan-level write lock |

### 3.2 In-Memory State

#### 3.2.1 Active Lock Registry

```go
// Location: /app/server/db/locks.go
var activeLockIds = make(map[string]bool)
var activeLockIdsMu sync.Mutex
```

**Purpose:** Tracks which locks this server instance holds.

**Access Pattern:**
- Write: On lock acquire/release
- Read: On cleanup/shutdown

**Protection:** `sync.Mutex`

#### 3.2.2 Repository Queue Map

```go
// Location: /app/server/db/queue.go
var repoQueues = make(repoQueueMap)
var queuesMu sync.Mutex
```

**Purpose:** Per-plan operation queues.

**Access Pattern:**
- Write: Adding operations, creating queues
- Read: Queue processing, status checks

**Protection:**
- `queuesMu` for map access
- Per-queue `mu` for queue operations

#### 3.2.3 Stream Accumulators

```go
// Location: /app/server/model/client_stream.go
accumulator := types.NewStreamCompletionAccumulator()
```

**Purpose:** Accumulates streaming AI responses.

**Access Pattern:** Single goroutine per stream (no sharing).

**Protection:** Not shared between goroutines.

### 3.3 File System State

| Resource | Concurrent Access Risk | Protection |
|----------|------------------------|------------|
| Plan directories | Git operations conflict | Database locks |
| Session logs | Multiple sessions write | File locking (fcntl) |
| Git index lock | Concurrent git commands | Lock file cleanup |
| Context cache | Read during plan execution | Read-only access |

---

## 4. Failure Modes

### 4.1 Race Conditions

#### 4.1.1 Lock Acquisition Race

**Scenario:** Two operations try to acquire write lock simultaneously.

**Symptom:** Both may briefly succeed before conflict detection.

**Detection:** PostgreSQL REPEATABLE_READ isolation detects conflict.

**Recovery:** Automatic retry with exponential backoff.

**User Message:**
```
Another operation is in progress on this plan. Waiting...
Retry 1/6: Will retry in 450ms
```

#### 4.1.2 Timer Drain Race

**Scenario:** Timer fires and is consumed by select, then code tries to drain it.

**Symptom:** Goroutine blocks forever.

**Prevention:** Non-blocking drain pattern:

```go
// SAFE: Non-blocking drain
if !timer.Stop() {
    select {
    case <-timer.C:
        // Timer value was pending
    default:
        // Already consumed
    }
}
```

#### 4.1.3 Session ID Collision

**Scenario:** Two sessions without IDs use same fallback ID.

**Symptom:** Log entries interleaved or overwritten.

**Prevention:** UUID-based fallback IDs ensure uniqueness.

### 4.2 Lost Updates

#### 4.2.1 Optimistic Update Failure

**Scenario:** Context update conflicts with ongoing tell operation.

**Symptom:** Context changes not reflected in AI response.

**Detection:** Lock acquisition failure indicates conflict.

**User Message:**
```
Cannot update context while plan is executing.
Wait for the current operation to complete, then try again.
```

#### 4.2.2 File System Conflict

**Scenario:** User edits file while apply operation runs.

**Symptom:** User changes overwritten.

**Detection:** Modification time check before write.

**User Message:**
```
Warning: src/auth.go was modified since last read.
Options:
  1. Overwrite with planned changes
  2. Skip this file
  3. Abort apply operation
```

### 4.3 Inconsistent User Feedback

#### 4.3.1 Stale Progress Display

**Scenario:** Connection drops during streaming operation.

**Symptom:** CLI shows operation in progress but it completed/failed.

**Detection:** Reconnect and query current plan status.

**User Message:**
```
Connection lost. Reconnecting...
Plan status: ready (operation completed while disconnected)
```

#### 4.3.2 Partial Error Reporting

**Scenario:** Operation fails mid-stream, error not fully displayed.

**Symptom:** User sees incomplete output without clear error.

**Prevention:** Error accumulator captures all errors for final report.

**User Message:**
```
Operation failed after partial completion.

Error: Context limit exceeded at 85% completion
Completed: 12/15 file changes
Remaining: 3 files skipped

Run 'plandex status' for details.
```

### 4.4 Deadlocks

#### 4.4.1 Database Deadlock

**Scenario:** Two transactions wait for each other's locks.

**Detection:** PostgreSQL error code 40P01.

**Recovery:** Automatic transaction rollback and retry.

**Logging:**
```
[Lock][Retry] Lock/transaction conflict (attempt #2).
Retrying in 600ms... (cause: deadlock detected)
```

#### 4.4.2 Heartbeat Deadlock

**Scenario:** Heartbeat update conflicts with lock deletion.

**Detection:** Heartbeat error counter exceeds threshold.

**Recovery:** Context cancellation stops the operation.

**Logging:**
```
[Lock][Heartbeat] Too many errors updating heartbeat, canceling operation
```

---

## 5. Concurrency Design

### 5.1 Lock Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Concurrency Control Stack                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Application Layer                      │  │
│  │  ExecRepoOperation() - Entry point for all repo ops      │  │
│  └────────────────────────────┬─────────────────────────────┘  │
│                               │                                 │
│  ┌────────────────────────────▼─────────────────────────────┐  │
│  │                     Queue Layer                           │  │
│  │  Per-plan queues batch compatible operations              │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                   │  │
│  │  │ Plan A  │  │ Plan B  │  │ Plan C  │                   │  │
│  │  │ Queue   │  │ Queue   │  │ Queue   │                   │  │
│  │  └────┬────┘  └────┬────┘  └────┬────┘                   │  │
│  └───────┼────────────┼────────────┼────────────────────────┘  │
│          │            │            │                            │
│  ┌───────▼────────────▼────────────▼────────────────────────┐  │
│  │                   Database Lock Layer                     │  │
│  │  PostgreSQL advisory locks with heartbeat                 │  │
│  │  ┌─────────────────────────────────────────────────────┐ │  │
│  │  │  SELECT * FROM lockable_plan_ids FOR UPDATE/SHARE   │ │  │
│  │  │  INSERT INTO repo_locks (scope, branch, ...)        │ │  │
│  │  └─────────────────────────────────────────────────────┘ │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 5.2 Lock Scope Rules

| Operation | Lock Scope | Can Run Concurrently With |
|-----------|------------|---------------------------|
| Tell | Write | Nothing |
| Build | Write | Nothing |
| Load context | Read | Other reads on same branch |
| View diffs | Read | Other reads on same branch |
| List plans | None | Everything |
| Archive plan | Write | Nothing |

### 5.3 Queue Batching Strategy

```go
// Batching rules in nextBatch():

// 1. Write operations: Always single-item batch
if firstOp.scope == LockScopeWrite {
    return []*repoOperation{firstOp}
}

// 2. Root branch reads: Single-item (branch="" means no branch)
if firstOp.branch == "" {
    return []*repoOperation{firstOp}
}

// 3. Same-branch reads: Batch together
for _, op := range remainingOps {
    if op.scope == LockScopeRead && op.branch == firstOp.branch {
        batch = append(batch, op)
    }
}
```

### 5.4 Heartbeat System

```
┌─────────────────────────────────────────────────────────────────┐
│                      Lock Heartbeat Flow                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Lock Acquired ────▶ Start Heartbeat Goroutine                  │
│                             │                                   │
│                             ▼                                   │
│                     ┌───────────────┐                           │
│                     │ Wait 3 seconds │◀──────────────┐          │
│                     │ (+ jitter)     │               │          │
│                     └───────┬───────┘               │          │
│                             │                        │          │
│                             ▼                        │          │
│                     ┌───────────────┐               │          │
│               ┌─────┤ Update DB     │               │          │
│               │     │ last_heartbeat│               │          │
│               │     └───────────────┘               │          │
│               │             │                        │          │
│       Error?  │             ▼ Success                │          │
│               │     ┌───────────────┐               │          │
│               │     │ rows_affected │               │          │
│               │     │ == 0?         │               │          │
│               │     └───────┬───────┘               │          │
│               │             │                        │          │
│               ▼             ▼ No                     │          │
│       ┌───────────┐        │                        │          │
│       │ Increment │        └────────────────────────┘          │
│       │ error cnt │                                             │
│       └─────┬─────┘   Yes (lock deleted)                        │
│             │              │                                    │
│             ▼              ▼                                    │
│       ┌───────────┐  ┌───────────┐                              │
│       │ cnt > 5?  │  │ Stop loop │                              │
│       └─────┬─────┘  └───────────┘                              │
│             │ Yes                                               │
│             ▼                                                   │
│       ┌───────────┐                                             │
│       │ Cancel    │                                             │
│       │ context   │                                             │
│       └───────────┘                                             │
│                                                                 │
│  Lock expired if no heartbeat for 60 seconds                    │
│  Other operations can reclaim expired locks                     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 5.5 Retry Strategy

```
┌─────────────────────────────────────────────────────────────────┐
│                 Exponential Backoff with Jitter                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Attempt │ Base Delay │ Jitter Range │  Total Range            │
│  ────────┼────────────┼──────────────┼───────────────           │
│     0    │   300ms    │   ±90ms      │  210ms - 390ms          │
│     1    │   600ms    │   ±180ms     │  420ms - 780ms          │
│     2    │   1.2s     │   ±360ms     │  840ms - 1.56s          │
│     3    │   2.4s     │   ±720ms     │  1.68s - 3.12s          │
│     4    │   4.8s     │   ±1.44s     │  3.36s - 6.24s          │
│     5    │   9.6s     │   ±2.88s     │  6.72s - 12.48s         │
│                                                                 │
│  Formula: delay = initialDelay * 2^attempt                     │
│  Jitter:  ± (delay * 0.3)                                      │
│  Max attempts: 6                                                │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 6. Debugging Concurrency Issues

### 6.1 Verbose Logging

Enable detailed lock logging by setting the constant:

```go
// In /app/server/db/locks.go
const locksVerboseLogging = true
```

This enables logging for:
- Lock acquisition attempts and timing
- Transaction start/commit/rollback
- Queue operations and batching
- Heartbeat updates

### 6.2 Log Format

```
[Lock][<GoroutineID>] <ACTION> <message> | reason: <operation_reason>
[Queue] <message>
[Lock][Heartbeat] <message>
[Lock][Retry][<GoroutineID>] <message>
[Lock][Delete][<GoroutineID>] <message>
```

### 6.3 Common Debug Scenarios

#### 6.3.1 Lock Acquisition Timeout

**Symptoms:**
```
[Lock][Retry][123] Failed to acquire lock after 6 attempts
```

**Investigation:**
1. Check `repo_locks` table for active locks:
   ```sql
   SELECT * FROM repo_locks WHERE plan_id = '<plan_id>';
   ```
2. Check if heartbeat is stale (last_heartbeat_at > 60s ago)
3. Check active operations on the server

**Resolution:**
- If lock is stale, it will be cleaned up on next acquisition attempt
- If operation is truly stuck, restart the server to clean up locks

#### 6.3.2 Queue Processing Stalled

**Symptoms:**
- Operations queued but not executing
- No errors in logs

**Investigation:**
1. Enable verbose logging
2. Check `isProcessing` flag on queue
3. Look for panic recovery in logs

**Resolution:**
- Queue processor recovers from panics and continues
- Server restart resets all queues

#### 6.3.3 Timer Deadlock

**Symptoms:**
- Goroutine count increasing
- Operations never complete
- No error messages

**Investigation:**
1. Use `runtime.Stack()` to dump all goroutines
2. Look for goroutines blocked on timer channel reads
3. Check timer reset patterns in code

**Prevention:**
Always use non-blocking timer drain:
```go
if !timer.Stop() {
    select {
    case <-timer.C:
    default:
    }
}
```

### 6.4 Diagnostic Queries

#### Active Locks
```sql
SELECT
    rl.id,
    rl.plan_id,
    rl.scope,
    rl.branch,
    rl.last_heartbeat_at,
    NOW() - rl.last_heartbeat_at AS age,
    p.name AS plan_name
FROM repo_locks rl
JOIN plans p ON p.id = rl.plan_id
ORDER BY rl.last_heartbeat_at DESC;
```

#### Expired Locks
```sql
SELECT * FROM repo_locks
WHERE last_heartbeat_at < NOW() - INTERVAL '60 seconds';
```

#### Lock History (if audit table exists)
```sql
SELECT * FROM repo_locks_audit
WHERE plan_id = '<plan_id>'
ORDER BY created_at DESC
LIMIT 50;
```

### 6.5 Error Messages and Their Causes

| Error Message | Likely Cause | Action |
|---------------|--------------|--------|
| "failed to acquire lock after N attempts" | Long-running operation or stale lock | Wait or check for stale locks |
| "context canceled while waiting to retry" | User canceled or timeout | Normal behavior |
| "error starting transaction" | Database connection issue | Check database connectivity |
| "deadlock detected" | Concurrent conflicting operations | Automatic retry handles this |
| "lock conflict: cannot acquire read/write lock" | Another operation has incompatible lock | Wait for completion |

---

## 7. Testing Concurrency

### 7.1 Test Categories

#### 7.1.1 Unit Tests

Test individual concurrent components in isolation:

```go
func TestQueueConcurrentAccess(t *testing.T) {
    var wg sync.WaitGroup
    queue := &repoQueue{}

    // Simulate 100 concurrent additions
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            queue.add(&repoOperation{
                id:     fmt.Sprintf("op-%d", i),
                branch: "main",
                scope:  LockScopeRead,
                done:   make(chan error, 1),
            })
        }(i)
    }

    wg.Wait()
    // Verify no race conditions with -race flag
}
```

#### 7.1.2 Integration Tests

Test lock acquisition and release across operations:

```go
func TestConcurrentTellOperations(t *testing.T) {
    // Create test plan
    planId := createTestPlan(t)

    // Launch concurrent tell operations
    results := make(chan error, 3)

    for i := 0; i < 3; i++ {
        go func() {
            err := executeTell(planId, "test message")
            results <- err
        }()
    }

    // Verify only one succeeds, others wait or fail gracefully
    var successCount, waitCount int
    for i := 0; i < 3; i++ {
        err := <-results
        if err == nil {
            successCount++
        } else if isWaitError(err) {
            waitCount++
        }
    }

    assert.Equal(t, 1, successCount, "Only one operation should succeed")
}
```

#### 7.1.3 Stress Tests

Test behavior under high load:

```go
func TestStressLockAcquisition(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping stress test in short mode")
    }

    planId := createTestPlan(t)

    var wg sync.WaitGroup
    errors := make(chan error, 1000)

    // 100 goroutines, 10 operations each
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 10; j++ {
                err := performOperation(planId)
                if err != nil {
                    errors <- err
                }
            }
        }()
    }

    wg.Wait()
    close(errors)

    // Count error types
    var lockFailures, deadlocks, successes int
    for err := range errors {
        switch classifyError(err) {
        case "lock_failure":
            lockFailures++
        case "deadlock":
            deadlocks++
        }
    }

    t.Logf("Lock failures: %d, Deadlocks: %d", lockFailures, deadlocks)
    // All operations should eventually succeed or fail gracefully
}
```

### 7.2 Simulated Crash Tests

#### 7.2.1 Server Crash During Lock

```go
func TestLockCleanupAfterCrash(t *testing.T) {
    // Create lock manually (simulating server crash)
    _, err := db.Exec(`
        INSERT INTO repo_locks (id, plan_id, scope, last_heartbeat_at)
        VALUES ($1, $2, 'w', NOW() - INTERVAL '120 seconds')
    `, "stale-lock-id", testPlanId)
    require.NoError(t, err)

    // New operation should clean up stale lock and succeed
    err = executeOperation(testPlanId)
    assert.NoError(t, err, "Should clean up stale lock and proceed")

    // Verify stale lock was deleted
    var count int
    db.QueryRow("SELECT COUNT(*) FROM repo_locks WHERE id = $1", "stale-lock-id").Scan(&count)
    assert.Equal(t, 0, count, "Stale lock should be deleted")
}
```

#### 7.2.2 Heartbeat Failure

```go
func TestHeartbeatFailureRecovery(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    // Start operation
    go func() {
        executeOperationWithContext(ctx, testPlanId)
    }()

    // Wait for lock to be acquired
    time.Sleep(100 * time.Millisecond)

    // Block heartbeat updates (simulate network partition)
    blockHeartbeatUpdates()

    // Wait for heartbeat timeout
    time.Sleep(70 * time.Second)

    // Verify operation was canceled
    assertOperationCanceled(t, testPlanId)

    // Verify another operation can now proceed
    err := executeOperation(testPlanId)
    assert.NoError(t, err, "New operation should succeed after heartbeat timeout")
}
```

### 7.3 Race Condition Detection

Always run tests with race detector:

```bash
# Run all tests with race detection
go test -race ./...

# Run specific package
go test -race ./app/server/db/...

# Run with verbose output
go test -race -v ./app/server/db/... 2>&1 | tee race-results.log
```

### 7.4 File Locking Tests (Python)

```python
import unittest
import threading
import tempfile
import os
from capture_session_event import write_log_entry_atomic

class TestFileLocking(unittest.TestCase):
    def test_concurrent_writes(self):
        """Test that concurrent writes don't corrupt file."""
        with tempfile.NamedTemporaryFile(delete=False) as f:
            log_file = f.name

        threads = []
        for i in range(100):
            t = threading.Thread(
                target=write_log_entry_atomic,
                args=(log_file, {"id": i, "data": "x" * 1000})
            )
            threads.append(t)

        for t in threads:
            t.start()
        for t in threads:
            t.join()

        # Verify all entries are valid JSON
        with open(log_file) as f:
            lines = f.readlines()

        self.assertEqual(len(lines), 100, "All entries should be written")

        for i, line in enumerate(lines):
            try:
                json.loads(line)
            except json.JSONDecodeError:
                self.fail(f"Line {i} is not valid JSON: {line[:50]}...")
```

---

## 8. CLI Communication

### 8.1 Concurrency Status Messages

#### 8.1.1 Waiting for Lock

```
Waiting for another operation to complete...

Current operation: Building changes on branch 'main'
Position in queue: 2 of 3
Estimated wait: ~30 seconds

Press Ctrl+C to cancel and try later.
```

#### 8.1.2 Lock Acquired After Wait

```
Operation started after waiting 12 seconds.
```

#### 8.1.3 Lock Acquisition Failed

```
Could not start operation after 30 seconds.

Reason: Another operation is still in progress on this plan.

Options:
  1. Wait and retry: plandex tell "your message"
  2. Check status:   plandex status
  3. Force stop:     plandex stop (may lose progress)

If this persists, the previous operation may have crashed.
Run 'plandex doctor' to check for stale locks.
```

### 8.2 Progress During Concurrent Operations

#### 8.2.1 Stream Progress

```
Thinking...  [=====>    ] 45% (2.3s)
├─ Tokens generated: 1,234 / ~2,700
├─ Files analyzed: 3/5
└─ Current: Analyzing src/auth/login.go
```

#### 8.2.2 Queue Status

```
Your operation is queued.

Queue status for plan 'my-feature':
  1. [Running] Building changes (started 15s ago)
  2. [Waiting] Your operation: Load context
  3. [Waiting] Another user: Tell operation

Estimated start time: ~20 seconds
```

### 8.3 Error Recovery Guidance

#### 8.3.1 Connection Lost

```
Connection lost during operation.

Attempting to reconnect... Connected!

Checking operation status...
  Plan: my-feature
  Status: Building
  Progress: 75% complete

Resuming stream...
```

#### 8.3.2 Partial Failure

```
Operation partially completed.

Summary:
  ✓ 8 files processed successfully
  ✗ 2 files failed (permission denied)
  ○ 3 files skipped (already up to date)

Failed files:
  - /etc/hosts: Permission denied
  - /root/.bashrc: Permission denied

To retry failed files with sudo:
  plandex apply --retry-failed --sudo
```

### 8.4 Doctor Command Output

```
$ plandex doctor

Checking system health...

✓ Database connection: OK
✓ API server: OK (latency: 45ms)
✓ Git repository: OK

Checking for issues...

⚠ Found 1 stale lock:
  Plan: old-feature
  Lock age: 2 hours 15 minutes
  Last heartbeat: 2 hours ago

This lock appears to be from a crashed operation.

Options:
  1. Clean up stale lock: plandex doctor --fix
  2. View details: plandex doctor --verbose
  3. Ignore for now (lock will expire eventually)

Would you like to clean up the stale lock? [y/N]
```

---

## 9. Implementation Checklist

### 9.1 Before Adding New Concurrent Code

- [ ] Identify all shared mutable state
- [ ] Choose appropriate synchronization primitive
- [ ] Document expected concurrency behavior
- [ ] Add tests with race detection
- [ ] Consider failure modes and recovery

### 9.2 Code Review Checklist

- [ ] Mutex usage follows lock/unlock pattern with defer
- [ ] Channels are properly sized (buffered when needed)
- [ ] Timers use non-blocking drain pattern
- [ ] Error channels include return after send
- [ ] Context cancellation is properly propagated
- [ ] Panic recovery doesn't hide errors

### 9.3 Testing Checklist

- [ ] Unit tests pass with `-race` flag
- [ ] Integration tests cover concurrent scenarios
- [ ] Stress tests verify behavior under load
- [ ] Crash recovery tests verify cleanup
- [ ] File locking tests verify atomic writes

### 9.4 Documentation Checklist

- [ ] Lock scope requirements documented
- [ ] Queue batching behavior documented
- [ ] Error messages are actionable
- [ ] Recovery procedures documented
- [ ] Diagnostic queries available

---

## Appendix A: Quick Reference

### Lock Scope Selection

```
Need to read plan state?     → LockScopeRead
Need to modify plan state?   → LockScopeWrite
Need to list/query plans?    → No lock needed
```

### Timer Management

```go
// Creating
timer := time.NewTimer(duration)

// Resetting (safe pattern)
if !timer.Stop() {
    select {
    case <-timer.C:
    default:
    }
}
timer.Reset(newDuration)

// Cleanup
timer.Stop()
```

### Channel Patterns

```go
// Error channel with goroutine
errCh := make(chan error, 1)
go func() {
    if err := work(); err != nil {
        errCh <- err
        return  // Don't forget!
    }
    errCh <- nil
}()
```

---

## Appendix B: Implementation Results

### B.1 Files Created

| File | Lines | Description |
|------|-------|-------------|
| `app/shared/doctor.go` | 115 | Shared types for diagnostics |
| `app/shared/doctor_test.go` | 250 | Type serialization tests |
| `app/server/handlers/doctor.go` | 220 | Server handler for /doctor endpoint |
| `app/cli/cmd/doctor.go` | 200 | CLI doctor command implementation |
| `app/server/db/concurrency_test.go` | 400 | Comprehensive concurrency tests |

### B.2 Files Modified

| File | Changes |
|------|---------|
| `app/cli/api/methods.go` | Added `Doctor()` API method |
| `app/server/routes/routes.go` | Added `POST /doctor` route |
| `.claude/hooks/capture_session_event.py` | Added atomic file locking with fcntl |

### B.3 Test Coverage

Tests implemented and passing:

```
=== Concurrency Tests ===
TestConcurrentQueueAccessStress/concurrent_queue_creation      PASS
TestConcurrentQueueAccessStress/concurrent_operation_addition  PASS
TestQueueBatching/write_operations_are_not_batched             PASS
TestQueueBatching/same-branch_reads_are_batched                PASS
TestQueueBatching/different-branch_reads_stop_batching         PASS
TestQueueBatching/root_branch_reads_are_not_batched            PASS
TestTimerDrainPattern/drain_works_when_timer_fired_and_read    PASS
TestTimerDrainPattern/drain_works_when_timer_has_not_fired     PASS
TestTimerDrainPattern/drain_works_when_timer_fired_not_read    PASS
TestChannelPatterns/buffered_error_channel_prevents_leak       PASS
TestChannelPatterns/done_channel_pattern_with_timeout          PASS
TestLockConflictDetection/write_conflicts_with_read            PASS
TestLockConflictDetection/read_conflicts_different_branch      PASS
TestLockConflictDetection/read_compatible_same_branch          PASS
TestRetryBackoff/attempt_0                                     PASS
TestRetryBackoff/attempt_1                                     PASS
TestRetryBackoff/attempt_2                                     PASS
TestRetryBackoff/attempt_3                                     PASS
TestContextCancellation/operation_respects_cancellation        PASS
TestContextCancellation/operation_respects_timeout             PASS
TestMutexUsage/activeLockIds_concurrent_access                 PASS
TestStressQueueMapAccess/high_volume_queue_map_access          PASS

=== Doctor Type Tests ===
TestDoctorResponseSerialization/serialize_empty_response       PASS
TestDoctorResponseSerialization/serialize_with_stale_locks     PASS
TestDoctorIssueTypes/*                                         PASS (7 tests)
TestIssueSeverityLevels/*                                      PASS (4 tests)
TestCheckStatusValues/*                                        PASS (3 tests)
TestConcurrencyErrorTypes/*                                    PASS (5 tests)
TestServerMetrics                                              PASS
TestDoctorRequest/*                                            PASS (2 tests)
```

### B.4 Doctor Command Usage

```bash
# Basic health check
plandex doctor

# Fix stale locks automatically
plandex doctor --fix

# Verbose output with server metrics
plandex doctor --verbose

# Combined
plandex doctor --fix --verbose
```

### B.5 Sample Doctor Output

```
Checking system health...

Health Checks

✓ Database Connection: Connected (latency: 45ms)
✓ Memory Usage: 256MB
✓ Goroutines: 150

⚠ Stale Locks Found

+------------+-------+--------+------------+
|    Plan    | Scope | Branch |    Age     |
+------------+-------+--------+------------+
| my-feature | write | main   | 2 hours    |
+------------+-------+--------+------------+

These locks appear to be from crashed operations.

Options:
  1. Clean up stale locks: plandex doctor --fix
  2. View details: plandex doctor --verbose
  3. Wait for locks to expire automatically (60 seconds from last heartbeat)
```

### B.6 CI Pipeline

**File:** `.github/workflows/concurrency-tests.yml`

**Triggers:**
- Push to main/develop (concurrency-related files only)
- Pull requests (concurrency-related files only)
- Daily at 3 AM UTC (scheduled)
- Manual dispatch

**Jobs:**

| Job | Purpose | Duration |
|-----|---------|----------|
| `race-detection` | Run tests with `-race` flag | ~5 min |
| `concurrency-unit-tests` | Timer, channel, queue, lock tests | ~3 min |
| `doctor-tests` | Doctor types serialization | ~1 min |
| `stress-tests` | High-volume concurrent access | ~10 min |
| `backoff-tests` | Retry calculation verification | ~1 min |
| `summary` | Aggregate results | ~30 sec |

**Manual Trigger with Options:**
```bash
gh workflow run concurrency-tests.yml \
  --field run_stress_tests=true
```

**Artifacts Saved:**
- Race detection logs (14 days)
- Unit test logs (14 days)
- Stress test logs (14 days)

### B.7 Atomic File Locking (Python)

The session capture hook now uses atomic writes:

```python
def write_log_entry_atomic(log_file, log_entry):
    """Write a log entry atomically with file locking."""
    os.makedirs(os.path.dirname(log_file), exist_ok=True)

    with open(log_file, "a", encoding="utf-8") as f:
        if HAS_FCNTL:  # Unix systems
            fcntl.flock(f.fileno(), fcntl.LOCK_EX)
            try:
                f.write(json.dumps(log_entry) + "\n")
                f.flush()
                os.fsync(f.fileno())
            finally:
                fcntl.flock(f.fileno(), fcntl.LOCK_UN)
        else:  # Windows fallback
            f.write(json.dumps(log_entry) + "\n")
            f.flush()
```

---

*Document generated: January 2026*
