# Concurrency Patterns in Plandex

**Version:** 2.1
**Last Updated:** January 2026 (Updated with implemented fixes)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Shared Mutable State Map](#2-shared-mutable-state-map)
3. [Single-Run Execution Assumptions](#3-single-run-execution-assumptions)
4. [Identified Failure Modes](#4-identified-failure-modes)
5. [Concurrency Design](#5-concurrency-design)
6. [Timer Management](#6-timer-management)
7. [Channel Patterns](#7-channel-patterns)
8. [Queue Processing](#8-queue-processing)
9. [Lock Strategies](#9-lock-strategies)
10. [Debugging Concurrency Issues](#10-debugging-concurrency-issues)
11. [CLI Communication of Concurrency State](#11-cli-communication-of-concurrency-state)
12. [Testing Strategy](#12-testing-strategy)

---

## 1. Overview

Plandex is a concurrent system by nature. A server handles multiple users,
each user may have multiple plans, and each plan execution involves parallel
goroutines for streaming, building, summarizing, and managing subscriptions.
Concurrency is not an edge case; it is the normal operating mode.

This document describes where shared mutable state exists, what assumptions
break under concurrent usage, what failure modes result, and how the system
makes concurrency explicit and safe through locking, isolation, queuing, and
clear user-facing diagnostics.

### Concurrency surfaces

| Surface | Mechanism | Scope |
|---------|-----------|-------|
| AI model streaming | Goroutines + channels | Per-plan, per-branch |
| Repository operations | Queue + database locks | Per-plan |
| Build execution | Parallel per-file goroutines | Per-plan |
| Subscription fan-out | Stream manager goroutine | Per-ActivePlan |
| Lock heartbeats | Background goroutine per lock | Per-lock |
| Summary generation | Independent context/goroutine | Per-plan |
| Session capture hooks | File-level locking (fcntl) | Per-log-file |

---

## 2. Shared Mutable State Map

Every concurrency bug originates from unprotected access to shared mutable
state. The following table maps every piece of shared mutable state in the
server, what protects it, and what risks remain.

### 2.1 In-Memory State

| State | Location | Protection | Risk |
|-------|----------|------------|------|
| `ActivePlan` fields (`CurrentReplyContent`, `Operations`, `BuiltFiles`, `IsBuildingByPath`, `BuildQueuesByPath`, `RepliesFinished`, `SkippedPaths`, `AllowOverwritePaths`) | `active_plan.go` | **None** (accessed by multiple goroutines during tell/build) | Race conditions between stream processor, build executor, and status checks. Fields are read and written from different goroutines without synchronization. |
| `ActivePlan.streamMessageBuffer`, `lastStreamMessageSent` | `active_plan.go` | `streamMu` (sync.Mutex) | Protected. Buffer flushing and rate-limiting are serialized. |
| `ActivePlan.subscriptions` | `active_plan.go` | `subscriptionMu` (sync.Mutex) | Protected for add/remove. The stream manager goroutine copies the map reference under lock, then iterates outside the lock, which is safe for the fan-out pattern since subscriptions are only added/removed, not mutated in place. |
| `subscription.messageQueue` | `active_plan.go` | `sub.mu` + `sub.cond` (sync.Cond) | Protected. Condition variable coordinates producer (enqueue) and consumer (processMessages). |
| `repoQueues` (global map of per-plan queues) | `queue.go` | `queuesMu` (sync.Mutex) | Protected for map access. Individual queue operations use `q.mu`. |
| `repoQueue.ops`, `isProcessing` | `queue.go` | `q.mu` (sync.Mutex) | Protected. Queue draining and batch selection are serialized. |
| `activeLockIds` (global map of held lock IDs) | `locks.go` | `activeLockIdsMu` (sync.Mutex) | Protected. Used only for cleanup tracking. |
| `SafeMap` instances (active plans registry, model streams) | `safe_map.go` | Internal `sync.Mutex` | Protected for individual operations. `Items()` returns a shallow copy. `SetIfAbsent` provides atomic check-and-set for plan activation. |
| `activeTellStreamState` fields | `tell_stream_*.go` | **None** (single-goroutine ownership assumed) | Safe only if the `listenStream` goroutine is the sole writer. The `onError` retry path re-enters `listenStream`, preserving single-writer semantics. |
| `chunkProcessor` state | `tell_stream_processor.go` | **None** (single-goroutine ownership) | Safe under the same single-writer assumption as `activeTellStreamState`. |

### 2.2 Database State

| State | Table | Protection | Risk |
|-------|-------|------------|------|
| Plan status | `plans`, `branches` | Database transactions + repo locks | Write lock required to change status. Read lock for reads on the same branch. Status transitions are serialized through the queue. |
| Repo locks | `repo_locks` | `REPEATABLE READ` transactions + `SELECT FOR UPDATE/SHARE` | Lock acquisition uses database-level serialization. Heartbeat expiration (60s) prevents permanent lock leaks. |
| Model streams | `model_streams` | Database check in `activatePlan` + `SetIfAbsent` on in-memory registry | The database check guards against cross-instance races. On a single instance, `CreateActivePlan` uses `SafeMap.SetIfAbsent` to atomically reject duplicate registrations. |
| Conversation messages | `convo_messages` | Repo write lock | Protected during tell execution. |
| Build results | `plan_builds` | Repo write lock | Protected during build execution. |

### 2.3 File System State

| State | Location | Protection | Risk |
|-------|----------|------------|------|
| Plan git repositories | `{base_dir}/orgs/{orgId}/plans/{planId}` | Repo lock + queue | Write lock ensures exclusive git access. `gitRemoveIndexLockFileIfExists` cleans stale `.git/index.lock` files before operations. |
| Session log files | `.claude/hooks/*.jsonl` | `fcntl.flock` (exclusive file lock) | Protected with POSIX advisory locks. Atomic write pattern: lock, write, flush, fsync, unlock. |

---

## 3. Single-Run Execution Assumptions

These are code paths that implicitly assume they are the only running
instance. When this assumption is violated, the system misbehaves.

### 3.1 ActivePlan field access without synchronization

**Files**: `active_plan.go`, `tell_exec.go`, `build_exec.go`

The `ActivePlan` struct has 30+ fields. Only `streamMessageBuffer`,
`lastStreamMessageSent`, and `subscriptions` are protected by mutexes. The
remaining fields (`CurrentReplyContent`, `Operations`, `BuiltFiles`,
`IsBuildingByPath`, `BuildQueuesByPath`, `NumTokens`, `RepliesFinished`,
etc.) are read and written by the stream processor, build executor, and
status-check code without synchronization.

This works **only** because the system enforces that a single plan+branch
combination has exactly one active execution at a time (checked in
`activatePlan`). If that gate ever fails, or if internal goroutines access
these fields concurrently (e.g., the build executor reads `Operations`
while the stream processor appends to it), data races occur.

### 3.2 Plan activation race window -- FIXED

**File**: `activate.go`, `state.go`

Previously, `activatePlan` used a 100ms `time.Sleep` followed by a
`GetActivePlan` check, creating a TOCTOU gap where two concurrent
requests could both pass the check.

**Fix applied**: The 100ms sleep was removed. `CreateActivePlan` now uses
`SafeMap.SetIfAbsent` to atomically register the plan. If the key already
exists, the new plan is canceled and `nil` is returned to the caller.
`activatePlan` checks for `nil` and returns an error with actionable
guidance (`plandex connect` / `plandex stop`).

### 3.3 SafeMap.Items() returns the inner map -- FIXED

**File**: `safe_map.go`

Previously, `Items()` returned a direct reference to the underlying map,
allowing callers to race with concurrent `Set`/`Delete` calls.

**Fix applied**: `Items()` now returns a shallow copy of the map. Callers
can safely iterate or mutate the returned map without affecting the
`SafeMap` or racing with other goroutines.

### 3.4 needsRollback flag in queue batch processing -- FIXED

**File**: `queue.go`

Previously, `needsRollback` was a bare `bool` written from goroutines
without synchronization. While safe in practice (write batches are
single-operation), the code did not structurally enforce this.

**Fix applied**: Changed to `atomic.Bool` with `.Store(true)` and
`.Load()`. The code is now correct regardless of future batching changes.

---

## 4. Identified Failure Modes

### 4.1 Race Conditions

| Failure | Trigger | Impact | Current Mitigation |
|---------|---------|--------|-------------------|
| Duplicate active plans | Two `tell` calls racing on the same plan+branch | Both streams write to the same plan state, corrupting reply content and build queues | **Fixed**: `CreateActivePlan` uses `SafeMap.SetIfAbsent` for atomic registration. Only one goroutine wins; the loser's plan is canceled and nil is returned. |
| Stale subscription map reference | Subscriber added/removed during fan-out iteration | Missed messages or send-to-closed-channel panic | The stream manager copies the map reference under `subscriptionMu` before iterating, and `enqueueMessage` uses the subscription's own mutex, so this is safe |
| ActivePlan field races | Build executor reads `Operations` while stream processor writes it | Corrupted operation list, missed or duplicate file builds | Single-execution enforcement in `activatePlan` (works as long as the gate holds) |

### 4.2 Lost Updates

| Failure | Trigger | Impact |
|---------|---------|--------|
| Lock heartbeat failure | Database connectivity issues during a long operation | After 60s without heartbeat, other operations treat the lock as expired and acquire their own lock, leading to concurrent writes to the same plan repository |
| Queue operation dropped on context cancel | Client disconnects during a queued write | The write operation's `done` channel receives `ctx.Err()` but the operation may have partially executed. The `clearRepoOnErr` flag triggers rollback only if the operation function returned an error, not if the context was canceled mid-operation |

### 4.3 Inconsistent User Feedback

| Failure | Trigger | User Experience |
|---------|---------|----------------|
| Lock retry exhaustion | Plan locked by a long-running build | **Fixed**: Error now includes actionable guidance: `plandex ps` to check active operations, `plandex stop` to cancel, and notes that stale locks expire after 60s. |
| Silent queue wait | Multiple operations queued for the same plan | User sees no output while their operation waits behind others in the queue. The CLI shows a spinner with no explanation of the delay. |
| Duplicate error messages | Stream error during build | Both the stream error handler and the build error handler may send error messages to the client, resulting in duplicate or contradictory error output |

### 4.4 Deadlock Scenarios

| Scenario | Root Cause | Status |
|----------|-----------|--------|
| Timer drain blocking | `<-timer.C` after value already consumed | **Fixed** (non-blocking select with default) |
| StreamDoneCh blocking | `active.StreamDoneCh <- err` sent when no reader is waiting | **Fixed**: `StreamDoneCh` is now buffered (capacity 1) in `NewActivePlan`, preventing send-side blocking across all 40+ send sites. |
| Lock + queue interaction | Lock acquired inside queue processing, lock heartbeat fails, lock expires, new queue entry acquires same lock | Prevented by the queue serializing all operations for a given plan. Only one batch runs at a time per plan. |

---

## 5. Concurrency Design

The system uses a layered approach to concurrency safety. Each layer
addresses a different scope of shared state.

### 5.1 Layer 1: Per-Plan Operation Queue (application-level serialization)

All repository operations for a given plan go through `ExecRepoOperation`,
which enqueues them in a per-plan `repoQueue`. The queue processes
operations sequentially, with one exception: multiple read operations on
the same branch can run in parallel.

```
                  ExecRepoOperation(planId, scope, branch)
                           │
                           ▼
                  ┌─────────────────┐
                  │  repoQueueMap   │  Global map, protected by queuesMu
                  │  [planId] → q   │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │   repoQueue     │  Per-plan, protected by q.mu
                  │                 │
                  │  ops: [op1, op2]│
                  │  isProcessing   │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │  nextBatch()    │  Determines what can run together
                  │                 │
                  │  Write? → [op]  │  Single operation, exclusive
                  │  Read?  → [ops] │  Batch same-branch reads
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ lockRepoDB()    │  Acquire DB lock before execution
                  │ execute batch   │  Run operations under lock
                  │ deleteRepoLock  │  Release DB lock after
                  └─────────────────┘
```

**Invariants:**
- At most one batch runs at a time per plan.
- Writes are always a single-item batch.
- Reads batch only if they are on the same branch.
- Each batch holds the database lock for its duration.

### 5.2 Layer 2: Database Locks (distributed coordination)

Database locks (`repo_locks` table) prevent concurrent access across
multiple server instances. The lock protocol:

1. Begin a `REPEATABLE READ` transaction.
2. `SELECT FOR UPDATE` on `lockable_plan_ids` (serialization point).
3. Read existing locks. Check for conflicts:
   - Write lock: conflicts with any existing lock.
   - Read lock: conflicts with write locks or reads on a different branch.
4. If no conflict, insert a new lock row.
5. Commit the transaction.
6. Start a heartbeat goroutine (3s interval, 60s expiry).

**Retry strategy:** Exponential backoff starting at 300ms, 6 attempts max,
with 30% jitter. Delete retries use fixed 50ms delay, 60 attempts max
(deletes must succeed to avoid permanent lock leaks).

### 5.3 Layer 3: Active Plan Isolation (single-execution gate)

The `activatePlan` function enforces that only one execution (tell or
build) runs per plan+branch at a time. It checks both the in-memory
`ActivePlan` registry and the database `model_streams` table. If either
indicates an active execution, the new request is rejected.

This is the critical gate that allows `ActivePlan` fields to be accessed
without per-field mutexes. If this gate fails, the unprotected field
accesses become races.

### 5.4 Layer 4: Stream Infrastructure (channel-based isolation)

Each `ActivePlan` has its own stream channel (`streamCh`) and subscription
system. The stream manager goroutine is the sole consumer of `streamCh`
and the sole producer for subscription queues. Each subscription has its
own `processMessages` goroutine that drains its queue and writes to its
channel.

```
Stream Producer (tell/build goroutines)
        │
        │ ap.Stream(msg)
        ▼
   ┌──────────┐
   │ streamMu │  Rate-limit and buffer under mutex
   └────┬─────┘
        │
        │ streamCh <- json
        ▼
   ┌──────────────────┐
   │ Stream Manager   │  Single goroutine, reads streamCh
   │ (NewActivePlan)  │  Copies subscriptions map under subscriptionMu
   └────────┬─────────┘  Iterates and calls sub.enqueueMessage
            │
     ┌──────┼──────┐
     ▼      ▼      ▼
  ┌─────┐┌─────┐┌─────┐
  │sub 1││sub 2││sub 3│  Each subscription has its own
  │queue││queue││queue│  mutex + cond for message queue
  └──┬──┘└──┬──┘└──┬──┘
     │      │      │
     ▼      ▼      ▼
  ┌─────┐┌─────┐┌─────┐
  │ SSE ││ SSE ││ SSE │  HTTP response streams to CLI clients
  └─────┘└─────┘└─────┘
```

### 5.5 Summary of Guarantees

| Guarantee | Mechanism | Scope |
|-----------|-----------|-------|
| No concurrent writes to same plan repo | Queue + DB write lock | Per-plan |
| No concurrent writes during read | Queue + DB read/write lock | Per-plan |
| Parallel reads on same branch | Queue batch + DB read lock | Per-plan, per-branch |
| Single active execution per plan+branch | `activatePlan` gate | Per-plan, per-branch |
| Ordered message delivery per subscription | Mutex + cond on subscription queue | Per-subscriber |
| Rate-limited stream output | `streamMu` + time-based buffering | Per-ActivePlan |
| Lock liveness | Heartbeat goroutine (3s) + expiry (60s) | Per-lock |

---

## 6. Timer Management

### 6.1 The Timer Drain Problem

Go's `time.Timer` has a subtle behavior that causes deadlocks:

```go
// DANGEROUS: Can deadlock!
timer := time.NewTimer(timeout)
// ... timer fires, value consumed by select case ...

if !timer.Stop() {
    <-timer.C  // BLOCKS FOREVER if channel is empty!
}
```

`timer.Stop()` returns `false` if timer already fired.
But if the fired value was already consumed by a select case, the channel
is empty and `<-timer.C` blocks forever.

### 6.2 Safe Timer Drain Pattern

```go
if !timer.Stop() {
    select {
    case <-timer.C:
    default:
    }
}
timer.Reset(newTimeout)
```

### 6.3 Locations

| File | Lines | Purpose |
|------|-------|---------|
| `tell_stream_main.go` | 148-153, 236-241 | Stream chunk timeouts |
| `client_stream.go` | 160-166, 200-208 | API response timeouts |

---

## 7. Channel Patterns

### 7.1 Error Channel with Goroutine

```go
errCh := make(chan error, 1)  // Buffered: prevents goroutine leak

go func() {
    result, err := doWork()
    if err != nil {
        errCh <- err
        return  // Must return after sending error
    }
    errCh <- nil
}()

if err := <-errCh; err != nil {
    return err
}
```

### 7.2 Done Channel Pattern

```go
done := make(chan struct{})

go func() {
    defer close(done)
    // Do work...
}()

select {
case <-done:
case <-ctx.Done():
case <-time.After(timeout):
}
```

### 7.3 Subscription Condition Variable Pattern

The subscription system uses `sync.Cond` instead of channels for the
internal message queue, allowing the consumer to block without allocating
a channel per message:

```go
// Producer
sub.mu.Lock()
sub.messageQueue = append(sub.messageQueue, msg)
sub.mu.Unlock()
sub.cond.Signal()

// Consumer
sub.mu.Lock()
for len(sub.messageQueue) == 0 {
    sub.cond.Wait()  // Unlocks mu, blocks, re-locks mu on wake
    if sub.ctx.Err() != nil {
        sub.mu.Unlock()
        return
    }
}
msg := sub.messageQueue[0]
sub.messageQueue = sub.messageQueue[1:]
sub.mu.Unlock()
```

---

## 8. Queue Processing

### 8.1 Repository Operation Queue

Per-plan queues serialize repository operations:

```go
type repoQueue struct {
    ops          []*repoOperation
    mu           sync.Mutex
    isProcessing bool
}
```

### 8.2 Batch Processing Rules

| First Operation | Queue Behavior |
|----------------|---------------|
| Write (any branch) | Process alone, exclusive |
| Read (root/no branch) | Process alone |
| Read (named branch) | Batch with subsequent reads on the same branch |

### 8.3 Queue Lifecycle

1. `ExecRepoOperation` creates an operation with a `done` channel.
2. The operation is added to the per-plan queue.
3. If no goroutine is processing the queue, `runQueue` starts.
4. `runQueue` calls `nextBatch` to get the next set of operations.
5. A database lock is acquired for the batch.
6. Operations execute (single write, or parallel reads).
7. The database lock is released.
8. Each operation's `done` channel receives the result.
9. `runQueue` loops to the next batch, or exits if the queue is empty.

---

## 9. Lock Strategies

### 9.1 Lock Scope Rules

| Requested | Existing | Same Branch | Allowed |
|-----------|----------|-------------|---------|
| Read | None | - | Yes |
| Read | Read | Yes | Yes |
| Read | Read | No | No |
| Read | Write | - | No |
| Write | None | - | Yes |
| Write | Any | - | No |

### 9.2 Lock Lifecycle

```
Acquire (lockRepoDB)
    │
    ├─ Begin REPEATABLE READ transaction
    ├─ SELECT FOR UPDATE/SHARE on lockable_plan_ids
    ├─ Read existing locks, filter expired (>60s no heartbeat)
    ├─ Delete expired locks
    ├─ Check conflict rules
    ├─ INSERT new lock row
    ├─ Commit transaction
    ├─ Start heartbeat goroutine (3s interval)
    └─ Clean stale .git/index.lock, checkout branch
        │
        │  (operation runs)
        │
Release (deleteRepoLockDB)
    │
    ├─ DELETE FROM repo_locks WHERE id = $1
    ├─ Retry up to 60 times at 50ms intervals on failure
    └─ Remove from activeLockIds map
```

### 9.3 Retry Configuration

| Parameter | Value | Purpose |
|-----------|-------|---------|
| `maxLockRetries` | 6 | Max acquisition attempts |
| `initialLockRetryDelay` | 300ms | First retry delay |
| `backoffFactor` | 2 | Exponential multiplier |
| `jitterFraction` | 0.3 | Randomization range |
| `lockHeartbeatInterval` | 3s | Heartbeat frequency |
| `lockHeartbeatTimeout` | 60s | Expiry threshold |
| `maxDeleteRetries` | 60 | Max delete attempts |
| `deleteRetryDelay` | 50ms | Fixed delete retry delay |

---

## 10. Debugging Concurrency Issues

When something goes wrong due to concurrency, the system should explain
what happened, why, and how to resolve it. This section defines the
diagnostic messages and log patterns for each concurrency failure mode.

### 10.1 Lock Contention

**Server log pattern:**
```
[Lock][Retry][goroutine-id] Lock/transaction conflict (attempt #N).
Retrying in Xms... (cause: lock conflict)
```

**User-facing error (after retry exhaustion):**
```
Failed to acquire lock after 6 attempts.
Another operation may be in progress or a previous operation
did not release its lock properly. Try again in a few seconds.
```

**What happened:** Another operation holds a write lock (or a read lock
on a different branch) on this plan. The current operation waited through
6 exponential backoff retries and gave up.

**How to fix:**
- Wait for the other operation to finish (check `plandex ps`).
- If the lock is stale (the holding operation crashed), the heartbeat
  will expire after 60s and the lock will be cleaned up automatically.
- As a last resort, server restart cleans all active locks via
  `CleanupActiveLocks`.

### 10.2 Duplicate Active Plan

**Server log pattern:**
```
Tell: Active plan found for plan ID {id} on branch {branch}
```
or:
```
Tell: Active model stream found for plan ID {id} on branch {branch}
on host {ip}
```

**User-facing error:**
```
Plan {id} branch {branch} already has an active stream on [this host | host X].
```

**What happened:** A `tell` or `build` command was issued while the plan
already has an active execution. This can happen if the user opens two
terminals and runs commands on the same plan, or if a previous execution
did not clean up properly.

**How to fix:**
- Run `plandex stop` to cancel the existing execution.
- Wait for the existing execution to finish.
- If the execution is stuck, the `ActivePlanTimeout` (2 hours) will
  eventually clean it up, but `plandex stop` is faster.

### 10.3 Queue Stalls

**Symptom:** Operation appears to hang with no output.

**Server log pattern:**
```
[Queue] Operation {id} ({reason}) queued behind N operations
```

**What happened:** The operation is waiting in the per-plan queue behind
other operations. Write operations block everything; read operations only
block writes and cross-branch reads.

**How to diagnose:** Enable verbose logging (`locksVerboseLogging = true`)
to see the full queue state, or query the `repo_locks` table to see what
lock is currently held.

### 10.4 Heartbeat Failure

**Server log pattern:**
```
[Lock][Heartbeat] Too many errors updating repo lock last heartbeat
```

**What happened:** The lock heartbeat goroutine failed to update the
database 5 times in a row. It canceled the operation's context, which
will cause the operation to fail. The lock will expire after 60s, allowing
other operations to proceed.

**Impact:** The canceled operation may have partially completed. If
`clearRepoOnErr` was set, the queue processor will attempt to roll back
uncommitted git changes.

### 10.5 Log Correlation

All lock and queue operations include a `reason` string that propagates
through the entire chain:

```
ExecRepoOperation(reason: "tell plan abc branch main")
    → [Queue] Adding operation {id} (tell plan abc branch main)
    → [Lock] START lock attempt for plan abc (tell plan abc branch main)
    → [Lock] Lock acquired: {lockId} (tell plan abc branch main)
    → [Lock][Heartbeat] Will update heartbeat (tell plan abc branch main)
    → [Queue] Operation {id} (tell plan abc branch main) completed
    → [Lock][Delete] Lock released: {lockId} (tell plan abc branch main)
```

Goroutine IDs are included in lock logs (`[Lock][goroutine-id]`) to
distinguish concurrent operations in log output.

---

## 11. CLI Communication of Concurrency State

The CLI should communicate concurrency-related delays and blocks to users
so they are never left wondering why nothing is happening.

### 11.1 Current Behavior

When the server returns an error due to lock contention or an active plan,
the CLI displays the error message. There is no feedback during the
waiting period (queue wait or lock retry).

### 11.2 Recommended Communication Patterns

| Situation | Current CLI Behavior | Recommended Behavior |
|-----------|---------------------|---------------------|
| Operation queued behind others | Silent spinner | `Waiting for another operation on this plan to finish...` |
| Lock retry in progress | Silent until failure | Stream a `StreamMessageStatus` update: `Acquiring plan lock (attempt N/6)...` |
| Lock retry exhaustion | Error message | Error message with context: `Another operation is modifying this plan. Use "plandex ps" to see active operations, or "plandex stop" to cancel.` |
| Active plan rejection | Error message | `This plan is currently executing on [this host/host X]. Use "plandex connect" to attach to the running stream, or "plandex stop" to cancel it.` |
| Heartbeat failure | No indication | `The connection to the plan lock was lost. The operation has been canceled. Your changes may have been partially applied. Check "plandex diffs" for current state.` |

### 11.3 Stream Message Types for Concurrency

The existing `StreamMessage` type system supports concurrency feedback
through these message types:

| Type | Purpose |
|------|---------|
| `StreamMessageStatus` | General status updates (could include queue/lock status) |
| `StreamMessageError` | Error conditions (lock failures, duplicate plan) |
| `StreamMessageFinished` | Completion signal |

---

## 12. Testing Strategy

### 12.1 Unit Tests

**Race detection:** All tests must pass under `go test -race ./...`.

**Timer drain test:**
```go
func TestNonBlockingTimerDrain(t *testing.T) {
    timer := time.NewTimer(1 * time.Millisecond)
    time.Sleep(10 * time.Millisecond)
    <-timer.C

    done := make(chan bool, 1)
    go func() {
        if !timer.Stop() {
            select {
            case <-timer.C:
            default:
            }
        }
        done <- true
    }()

    select {
    case <-done:
    case <-time.After(100 * time.Millisecond):
        t.Fatal("Timer drain blocked")
    }
}
```

**SafeMap.Items() race test:**
```go
func TestSafeMapItemsRace(t *testing.T) {
    sm := NewSafeMap[int]()
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(2)
        go func(i int) {
            defer wg.Done()
            sm.Set(fmt.Sprintf("key-%d", i), i)
        }(i)
        go func() {
            defer wg.Done()
            _ = sm.Items()
        }()
    }
    wg.Wait()
}
```

### 12.2 Concurrency-Specific Tests

**Concurrent queue access:**
```go
func TestConcurrentQueueAccess(t *testing.T) {
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            planId := fmt.Sprintf("plan-%d", i%10)
            _ = repoQueues.getQueue(planId)
        }(i)
    }
    wg.Wait()
}
```

**Subscription fan-out under concurrent subscribe/unsubscribe:**
```go
func TestSubscriptionConcurrency(t *testing.T) {
    ap := NewActivePlan("org", "user", "plan", "main", "test", false, false, "sess")
    defer ap.CancelFn()

    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            ctx, cancel := context.WithCancel(context.Background())
            id, _ := ap.Subscribe(ctx)
            time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
            ap.Unsubscribe(id)
            cancel()
        }()
    }

    // Concurrent stream messages
    for i := 0; i < 20; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            ap.Stream(shared.StreamMessage{
                Type: shared.StreamMessageReply,
            })
        }(i)
    }

    wg.Wait()
}
```

### 12.3 Stress Tests

**Lock contention stress test:**

Simulates multiple goroutines competing for locks on the same plan to
verify that lock acquisition, heartbeat, and cleanup all work correctly
under pressure.

```go
func TestLockContentionStress(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping stress test in short mode")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    var wg sync.WaitGroup
    var acquired, contended atomic.Int64

    for i := 0; i < 20; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            err := ExecRepoOperation(ExecRepoOperationParams{
                OrgId:  "test-org",
                PlanId: "test-plan",
                Branch: "main",
                Scope:  LockScopeWrite,
                Reason: fmt.Sprintf("stress-test-%d", i),
                Ctx:    ctx,
                CancelFn: cancel,
            }, func(repo *GitRepo) error {
                acquired.Add(1)
                time.Sleep(50 * time.Millisecond) // Simulate work
                return nil
            })
            if err != nil {
                contended.Add(1)
            }
        }(i)
    }

    wg.Wait()
    t.Logf("acquired: %d, contended: %d", acquired.Load(), contended.Load())
}
```

### 12.4 Simulated Crash Tests

**Lock cleanup after crash:**

Verifies that locks are cleaned up after a simulated process crash
(heartbeat stops, lock expires).

```go
func TestLockExpiryAfterCrash(t *testing.T) {
    // Acquire a lock
    ctx, cancel := context.WithCancel(context.Background())
    lockId, err := lockRepoDB(LockRepoParams{
        OrgId:  "test-org",
        PlanId: "crash-test-plan",
        Branch: "main",
        Scope:  LockScopeWrite,
        Reason: "crash-test",
        Ctx:    ctx,
        CancelFn: cancel,
    }, 0)
    require.NoError(t, err)
    require.NotEmpty(t, lockId)

    // Simulate crash: cancel context (stops heartbeat)
    cancel()

    // Wait for lock to expire
    time.Sleep(lockHeartbeatTimeout + 5*time.Second)

    // Another operation should now succeed
    ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel2()

    lockId2, err := lockRepoDB(LockRepoParams{
        OrgId:  "test-org",
        PlanId: "crash-test-plan",
        Branch: "main",
        Scope:  LockScopeWrite,
        Reason: "post-crash",
        Ctx:    ctx2,
        CancelFn: cancel2,
    }, 0)
    require.NoError(t, err)
    require.NotEmpty(t, lockId2)

    // Clean up
    deleteRepoLockDB(lockId2, "crash-test-plan", "cleanup", 0)
}
```

### 12.5 Deadlock Detection in CI

```bash
# Run all tests with race detector and a timeout
go test -race -timeout 120s ./...

# Run stress tests separately with longer timeout
go test -race -timeout 300s -run TestStress ./app/server/db/
```

### 12.6 Test Coverage Matrix

| Scenario | Test Type | Files |
|----------|-----------|-------|
| Timer drain (all states) | Unit | `timer_utils_test.go` |
| SafeMap concurrent access | Unit + Race | `safe_map_test.go` |
| Queue concurrent add | Unit + Race | `queue_test.go` |
| Lock acquisition/release | Integration | `locks_test.go` |
| Lock expiry after crash | Integration | `locks_test.go` |
| Lock contention (20 goroutines) | Stress | `locks_test.go` |
| Subscription add/remove during stream | Unit + Race | `active_plan_test.go` |
| Duplicate plan activation | Integration | `activate_test.go` |
| Queue batch processing (mixed read/write) | Unit | `queue_test.go` |
| Heartbeat failure and recovery | Integration | `locks_test.go` |

---

## Appendix: Common Pitfalls

| Pitfall | Symptom | Solution | Status |
|---------|---------|----------|--------|
| Blocking timer drain | Goroutine hangs forever | Use select with default | Fixed |
| Missing return after errCh send | Undefined behavior | Always return after error | Fixed |
| Unbuffered channel in goroutine | Goroutine leak | Use buffered channel | Fixed (`StreamDoneCh` now buffered) |
| Missing mutex unlock | Deadlock on next access | Use defer for unlock | Pattern enforced |
| Race on shared state | Data corruption | Use mutex or channels | Ongoing discipline |
| SafeMap.Items() mutation | Race with concurrent Set/Delete | Items() returns a copy | Fixed |
| needsRollback unsynchronized write | Potential race | Changed to `atomic.Bool` | Fixed |
| 100ms sleep as synchronization | TOCTOU race on plan activation | `SetIfAbsent` on plan registry | Fixed |
| Missing `defer cancel()` | Context/goroutine leak on early return | Always `defer cancel()` after `WithCancel` | Fixed (22 handler locations) |
| Body close after ReadAll | Body leak on ReadAll error | Move `defer r.Body.Close()` before read | Fixed (10 handler locations) |
| Heuristic sleep before lock | Adds latency, doesn't prevent race | Lock-based serialization via `ExecRepoOperation` | Fixed (2 sleeps removed) |

---

*Document version 2.2 - January 2026*
