# Concurrency Patterns in Plandex

**Version:** 1.0
**Last Updated:** January 2026

---

## Table of Contents

1. [Overview](#1-overview)
2. [Timer Management](#2-timer-management)
3. [Channel Patterns](#3-channel-patterns)
4. [Queue Processing](#4-queue-processing)
5. [Lock Strategies](#5-lock-strategies)
6. [Error Channel Patterns](#6-error-channel-patterns)
7. [Testing Concurrent Code](#7-testing-concurrent-code)

---

## 1. Overview

Plandex uses extensive concurrent programming patterns for:
- Real-time streaming of AI model responses
- Queue-based repository operations
- Timeout handling for API calls
- Parallel data loading

This document describes the patterns used and pitfalls to avoid.

---

## 2. Timer Management

### 2.1 The Timer Drain Problem

Go's `time.Timer` has a subtle behavior that can cause deadlocks:

```go
// DANGEROUS: Can deadlock!
timer := time.NewTimer(timeout)
// ... timer fires, value consumed by select case ...

if !timer.Stop() {
    <-timer.C  // BLOCKS FOREVER if channel is empty!
}
```

**Why it deadlocks:**
- `timer.Stop()` returns `false` if timer already fired
- But if the fired value was already consumed (e.g., by a select case), the channel is empty
- `<-timer.C` blocks forever waiting for a value that will never come

### 2.2 Safe Timer Drain Pattern

```go
// SAFE: Non-blocking drain
if !timer.Stop() {
    select {
    case <-timer.C:
        // Timer value was pending, now drained
    default:
        // Timer value was already consumed, nothing to drain
    }
}
timer.Reset(newTimeout)
```

### 2.3 Files Using This Pattern

| File | Location | Purpose |
|------|----------|---------|
| `tell_stream_main.go` | Lines 148-153, 236-241 | Stream chunk timeouts |
| `client_stream.go` | Lines 160-166, 200-208 | API response timeouts |

### 2.4 Timer Pattern Test

```go
func TestNonBlockingTimerDrain(t *testing.T) {
    t.Run("drain works when timer has fired and already read", func(t *testing.T) {
        timer := time.NewTimer(1 * time.Millisecond)
        time.Sleep(10 * time.Millisecond)
        <-timer.C  // Consume the timer value

        // This is the scenario that caused deadlock
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
            // Success - didn't block
        case <-time.After(100 * time.Millisecond):
            t.Error("Timer drain blocked - this is the bug!")
        }
    })
}
```

---

## 3. Channel Patterns

### 3.1 Error Channel with Goroutine

When spawning a goroutine that can fail, use a buffered error channel:

```go
errCh := make(chan error, 1)

go func() {
    defer close(errCh)  // Always close when done

    result, err := doWork()
    if err != nil {
        errCh <- err
        return  // CRITICAL: Must return after sending error!
    }

    // Continue with success path...
    errCh <- nil
}()

// Wait for result
if err := <-errCh; err != nil {
    return err
}
```

### 3.2 Common Mistake: Missing Return

```go
// BUG: Missing return after error
go func() {
    if err != nil {
        errCh <- fmt.Errorf("failed: %v", err)
        // Missing return! Goroutine continues executing...
    }

    // This code runs even after error was sent!
    doMoreWork()  // Undefined behavior
}()
```

### 3.3 Done Channel Pattern

For signaling completion without error details:

```go
done := make(chan struct{})

go func() {
    defer close(done)
    // Do work...
}()

select {
case <-done:
    // Work completed
case <-ctx.Done():
    // Context canceled
case <-time.After(timeout):
    // Timed out
}
```

---

## 4. Queue Processing

### 4.1 Repository Operation Queue

Plandex uses a per-plan queue for serializing repository operations:

```go
type repoQueue struct {
    mu           sync.Mutex
    ops          []*repoOperation
    isProcessing bool
}

type repoQueueMap map[string]*repoQueue

func (m repoQueueMap) getQueue(planId string) *repoQueue {
    queuesMu.Lock()
    defer queuesMu.Unlock()

    if q, exists := m[planId]; exists {
        return q
    }
    q := &repoQueue{}
    m[planId] = q
    return q
}
```

### 4.2 Batch Processing Logic

Read operations on the same branch can be batched; writes are exclusive:

```go
func (q *repoQueue) nextBatch() []*repoOperation {
    q.mu.Lock()
    defer q.mu.Unlock()

    if len(q.ops) == 0 {
        return nil
    }

    first := q.ops[0]

    // Writes are always single-item batches
    if first.scope == LockScopeWrite {
        q.ops = q.ops[1:]
        return []*repoOperation{first}
    }

    // Batch reads on same branch (but not root branch)
    if first.branch == "" {
        q.ops = q.ops[1:]
        return []*repoOperation{first}
    }

    var batch []*repoOperation
    var remaining []*repoOperation

    for _, op := range q.ops {
        if op.scope == LockScopeRead && op.branch == first.branch {
            batch = append(batch, op)
        } else {
            remaining = append(remaining, op)
        }
    }

    q.ops = remaining
    return batch
}
```

---

## 5. Lock Strategies

### 5.1 Database Locks

Repository operations use database-level locks for distributed safety:

```go
type LockRepoParams struct {
    OrgId  string
    UserId string
    PlanId string
    Branch string
    Scope  LockScope  // "read" or "write"
    Reason string
}
```

### 5.2 Lock Scope Rules

| Scope | Concurrent Reads | Concurrent Writes | Use Case |
|-------|------------------|-------------------|----------|
| Read | Yes (same branch) | No | Viewing files |
| Write | No | No | Modifying files |

### 5.3 Lock Acquisition with Retry

```go
const maxRetries = 6

for attempt := 0; attempt < maxRetries; attempt++ {
    lock, err := acquireLock(params)
    if err == nil {
        return lock, nil
    }

    if !isRetryableError(err) {
        return nil, err
    }

    time.Sleep(backoff(attempt))
}

return nil, fmt.Errorf(
    "failed to acquire lock after %d attempts (cause: %v). "+
    "Another operation may be in progress or a previous operation "+
    "did not release its lock properly. Try again in a few seconds",
    maxRetries, lastErr,
)
```

---

## 6. Error Channel Patterns

### 6.1 Multiple Goroutines with WaitGroup

```go
var wg sync.WaitGroup
errCh := make(chan error, numWorkers)

for i := 0; i < numWorkers; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        if err := doWork(id); err != nil {
            errCh <- err
        }
    }(i)
}

// Close channel when all workers done
go func() {
    wg.Wait()
    close(errCh)
}()

// Collect errors
var errs []error
for err := range errCh {
    errs = append(errs, err)
}
```

### 6.2 First-Error-Wins Pattern

```go
errCh := make(chan error, 1)  // Buffered to prevent goroutine leak

go func() { errCh <- worker1() }()
go func() { errCh <- worker2() }()
go func() { errCh <- worker3() }()

// Wait for first completion (success or error)
if err := <-errCh; err != nil {
    // Cancel other workers via context
    cancel()
    return err
}
```

---

## 7. Testing Concurrent Code

### 7.1 Race Detection

Always run tests with race detector:

```bash
go test -race ./...
```

### 7.2 Concurrent Access Test

```go
func TestConcurrentQueueAccess(t *testing.T) {
    var wg sync.WaitGroup
    numGoroutines := 100

    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            planId := fmt.Sprintf("plan-%d", i%10)
            _ = repoQueues.getQueue(planId)
        }(i)
    }

    wg.Wait()
    // If we get here without deadlock or panic, test passes
}
```

### 7.3 Timeout-Based Deadlock Detection

```go
func TestNoDeadlock(t *testing.T) {
    done := make(chan bool, 1)

    go func() {
        // Code that might deadlock
        potentiallyDeadlockingOperation()
        done <- true
    }()

    select {
    case <-done:
        // Success
    case <-time.After(1 * time.Second):
        t.Fatal("Operation deadlocked")
    }
}
```

---

## Appendix: Common Pitfalls

| Pitfall | Symptom | Solution |
|---------|---------|----------|
| Blocking timer drain | Goroutine hangs forever | Use select with default |
| Missing return after errCh send | Undefined behavior | Always return after error |
| Unbuffered channel in goroutine | Goroutine leak | Use buffered channel |
| Missing mutex unlock | Deadlock on next access | Use defer for unlock |
| Race on shared state | Data corruption | Use mutex or channels |

---

*Document generated: January 2026*
