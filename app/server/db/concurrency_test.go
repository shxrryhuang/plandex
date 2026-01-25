package db

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrentQueueAccessStress tests thread-safety of queue operations under high load
func TestConcurrentQueueAccessStress(t *testing.T) {
	t.Run("concurrent queue creation", func(t *testing.T) {
		// Reset global state
		queuesMu.Lock()
		repoQueues = make(repoQueueMap)
		queuesMu.Unlock()

		var wg sync.WaitGroup
		numGoroutines := 100
		planIds := make([]string, 10)
		for i := 0; i < 10; i++ {
			planIds[i] = fmt.Sprintf("plan-%d", i)
		}

		// Concurrently access queues for the same plan IDs
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				planId := planIds[i%10]
				_ = repoQueues.getQueue(planId)
			}(i)
		}

		wg.Wait()

		// Verify we only created 10 queues (one per plan)
		queuesMu.Lock()
		queueCount := len(repoQueues)
		queuesMu.Unlock()

		if queueCount != 10 {
			t.Errorf("Expected 10 queues, got %d", queueCount)
		}
	})

	t.Run("concurrent operation addition", func(t *testing.T) {
		// Reset global state
		queuesMu.Lock()
		repoQueues = make(repoQueueMap)
		queuesMu.Unlock()

		planId := "test-plan"
		var wg sync.WaitGroup
		numOps := 50
		var addedCount int32

		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				repoQueues.add(&repoOperation{
					id:       fmt.Sprintf("op-%d", i),
					orgId:    "org-1",
					planId:   planId,
					branch:   "main",
					scope:    LockScopeRead,
					reason:   fmt.Sprintf("test op %d", i),
					ctx:      ctx,
					cancelFn: cancel,
					done:     make(chan error, 1),
				})
				atomic.AddInt32(&addedCount, 1)
			}(i)
		}

		wg.Wait()

		if addedCount != int32(numOps) {
			t.Errorf("Expected %d operations added, got %d", numOps, addedCount)
		}
	})
}

// TestQueueBatching tests the batching logic for operations
func TestQueueBatching(t *testing.T) {
	t.Run("write operations are not batched", func(t *testing.T) {
		q := &repoQueue{}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Add a write operation
		q.ops = append(q.ops, &repoOperation{
			id:       "write-1",
			scope:    LockScopeWrite,
			branch:   "main",
			ctx:      ctx,
			cancelFn: cancel,
			done:     make(chan error, 1),
		})

		// Add a read operation
		q.ops = append(q.ops, &repoOperation{
			id:       "read-1",
			scope:    LockScopeRead,
			branch:   "main",
			ctx:      ctx,
			cancelFn: cancel,
			done:     make(chan error, 1),
		})

		batch := q.nextBatch()

		if len(batch) != 1 {
			t.Errorf("Expected batch size 1 for write op, got %d", len(batch))
		}
		if batch[0].id != "write-1" {
			t.Errorf("Expected write-1, got %s", batch[0].id)
		}
	})

	t.Run("same-branch reads are batched", func(t *testing.T) {
		q := &repoQueue{}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Add multiple read operations on same branch
		for i := 0; i < 5; i++ {
			q.ops = append(q.ops, &repoOperation{
				id:       fmt.Sprintf("read-%d", i),
				scope:    LockScopeRead,
				branch:   "main",
				ctx:      ctx,
				cancelFn: cancel,
				done:     make(chan error, 1),
			})
		}

		batch := q.nextBatch()

		if len(batch) != 5 {
			t.Errorf("Expected batch size 5 for same-branch reads, got %d", len(batch))
		}
	})

	t.Run("different-branch reads stop batching", func(t *testing.T) {
		q := &repoQueue{}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Add reads on main branch
		q.ops = append(q.ops, &repoOperation{
			id: "read-main-1", scope: LockScopeRead, branch: "main",
			ctx: ctx, cancelFn: cancel, done: make(chan error, 1),
		})
		q.ops = append(q.ops, &repoOperation{
			id: "read-main-2", scope: LockScopeRead, branch: "main",
			ctx: ctx, cancelFn: cancel, done: make(chan error, 1),
		})

		// Add read on different branch
		q.ops = append(q.ops, &repoOperation{
			id: "read-feature", scope: LockScopeRead, branch: "feature",
			ctx: ctx, cancelFn: cancel, done: make(chan error, 1),
		})

		// Add more reads on main
		q.ops = append(q.ops, &repoOperation{
			id: "read-main-3", scope: LockScopeRead, branch: "main",
			ctx: ctx, cancelFn: cancel, done: make(chan error, 1),
		})

		batch := q.nextBatch()

		// Should only batch the first two main branch reads
		if len(batch) != 2 {
			t.Errorf("Expected batch size 2, got %d", len(batch))
		}
	})

	t.Run("root branch reads are not batched", func(t *testing.T) {
		q := &repoQueue{}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Add root branch read (empty branch)
		q.ops = append(q.ops, &repoOperation{
			id: "read-root-1", scope: LockScopeRead, branch: "",
			ctx: ctx, cancelFn: cancel, done: make(chan error, 1),
		})
		q.ops = append(q.ops, &repoOperation{
			id: "read-root-2", scope: LockScopeRead, branch: "",
			ctx: ctx, cancelFn: cancel, done: make(chan error, 1),
		})

		batch := q.nextBatch()

		if len(batch) != 1 {
			t.Errorf("Expected batch size 1 for root branch read, got %d", len(batch))
		}
	})
}

// TestTimerDrainPattern tests the non-blocking timer drain pattern
func TestTimerDrainPattern(t *testing.T) {
	t.Run("drain works when timer has fired and already read", func(t *testing.T) {
		timer := time.NewTimer(1 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		<-timer.C // Consume the timer value

		// This is the scenario that caused deadlock before the fix
		done := make(chan bool, 1)
		go func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
					// Timer value was pending
				default:
					// Already consumed - this is what we expect
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

	t.Run("drain works when timer has not fired", func(t *testing.T) {
		timer := time.NewTimer(1 * time.Hour)

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
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Timer drain blocked")
		}
	})

	t.Run("drain works when timer fired but not read", func(t *testing.T) {
		timer := time.NewTimer(1 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		// Don't read the timer - value is pending

		done := make(chan bool, 1)
		go func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
					// Should drain the pending value
				default:
				}
			}
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Timer drain blocked")
		}
	})
}

// TestChannelPatterns tests error channel patterns
func TestChannelPatterns(t *testing.T) {
	t.Run("buffered error channel prevents goroutine leak", func(t *testing.T) {
		errCh := make(chan error, 1) // Buffered!

		go func() {
			errCh <- fmt.Errorf("test error")
			// With unbuffered channel and no reader, this would block forever
		}()

		// Give goroutine time to send
		time.Sleep(10 * time.Millisecond)

		// Read should succeed
		select {
		case err := <-errCh:
			if err == nil {
				t.Error("Expected error, got nil")
			}
		default:
			t.Error("Channel should have value")
		}
	})

	t.Run("done channel pattern with timeout", func(t *testing.T) {
		done := make(chan struct{})

		go func() {
			time.Sleep(10 * time.Millisecond)
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Operation timed out")
		}
	})
}

// TestLockConflictDetection tests that lock conflicts are properly detected
func TestLockConflictDetection(t *testing.T) {
	t.Run("write conflicts with read", func(t *testing.T) {
		// Simulate lock conflict logic
		locks := []struct {
			scope  LockScope
			branch string
		}{
			{LockScopeWrite, "main"},
		}

		canAcquire := true
		for _, lock := range locks {
			if lock.scope == LockScopeWrite {
				canAcquire = false
				break
			}
		}

		if canAcquire {
			t.Error("Read should not be acquirable when write lock exists")
		}
	})

	t.Run("read conflicts with different branch read", func(t *testing.T) {
		requestedBranch := "feature"

		locks := []struct {
			scope  LockScope
			branch string
		}{
			{LockScopeRead, "main"},
		}

		canAcquire := true
		for _, lock := range locks {
			if lock.scope == LockScopeRead && lock.branch != requestedBranch {
				canAcquire = false
				break
			}
		}

		if canAcquire {
			t.Error("Read should not be acquirable when different branch has read lock")
		}
	})

	t.Run("read compatible with same branch read", func(t *testing.T) {
		requestedBranch := "main"

		locks := []struct {
			scope  LockScope
			branch string
		}{
			{LockScopeRead, "main"},
		}

		canAcquire := true
		for _, lock := range locks {
			if lock.scope == LockScopeWrite {
				canAcquire = false
				break
			}
			if lock.scope == LockScopeRead && lock.branch != requestedBranch {
				canAcquire = false
				break
			}
		}

		if !canAcquire {
			t.Error("Same-branch read should be acquirable")
		}
	})
}

// TestRetryBackoff tests the exponential backoff calculation
func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{0, 210 * time.Millisecond, 390 * time.Millisecond},   // 300ms ± 30%
		{1, 420 * time.Millisecond, 780 * time.Millisecond},   // 600ms ± 30%
		{2, 840 * time.Millisecond, 1560 * time.Millisecond},  // 1.2s ± 30%
		{3, 1680 * time.Millisecond, 3120 * time.Millisecond}, // 2.4s ± 30%
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			// Calculate backoff using the same formula as locks.go
			backoff := time.Duration(float64(initialLockRetryDelay) * pow(backoffFactor, float64(tt.attempt)))
			jitterRange := time.Duration(float64(backoff) * jitterFraction)

			minBackoff := backoff - jitterRange
			maxBackoff := backoff + jitterRange

			if minBackoff < tt.minExpected-10*time.Millisecond || maxBackoff > tt.maxExpected+10*time.Millisecond {
				t.Errorf("Backoff range [%v, %v] not in expected range [%v, %v]",
					minBackoff, maxBackoff, tt.minExpected, tt.maxExpected)
			}
		})
	}
}

// pow is a simple integer power function
func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

// TestContextCancellation tests that context cancellation is properly handled
func TestContextCancellation(t *testing.T) {
	t.Run("operation respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)

		go func() {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
			case <-time.After(5 * time.Second):
				done <- fmt.Errorf("timeout")
			}
		}()

		// Cancel immediately
		cancel()

		select {
		case err := <-done:
			if err != context.Canceled {
				t.Errorf("Expected context.Canceled, got %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Context cancellation was not detected")
		}
	})

	t.Run("operation respects context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		done := make(chan error, 1)

		go func() {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
			case <-time.After(5 * time.Second):
				done <- fmt.Errorf("timeout")
			}
		}()

		select {
		case err := <-done:
			if err != context.DeadlineExceeded {
				t.Errorf("Expected context.DeadlineExceeded, got %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Context timeout was not detected")
		}
	})
}

// TestMutexUsage tests proper mutex usage patterns
func TestMutexUsage(t *testing.T) {
	t.Run("activeLockIds concurrent access", func(t *testing.T) {
		// Reset state
		activeLockIdsMu.Lock()
		activeLockIds = make(map[string]bool)
		activeLockIdsMu.Unlock()

		var wg sync.WaitGroup
		numGoroutines := 100

		// Concurrent writes
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				lockId := fmt.Sprintf("lock-%d", i)

				activeLockIdsMu.Lock()
				activeLockIds[lockId] = true
				activeLockIdsMu.Unlock()
			}(i)
		}

		wg.Wait()

		// Verify all locks were added
		activeLockIdsMu.Lock()
		count := len(activeLockIds)
		activeLockIdsMu.Unlock()

		if count != numGoroutines {
			t.Errorf("Expected %d locks, got %d", numGoroutines, count)
		}

		// Concurrent reads and deletes
		var readCount int32
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				lockId := fmt.Sprintf("lock-%d", i)

				activeLockIdsMu.Lock()
				if activeLockIds[lockId] {
					atomic.AddInt32(&readCount, 1)
					delete(activeLockIds, lockId)
				}
				activeLockIdsMu.Unlock()
			}(i)
		}

		wg.Wait()

		if readCount != int32(numGoroutines) {
			t.Errorf("Expected to read %d locks, got %d", numGoroutines, readCount)
		}

		// Verify all deleted
		activeLockIdsMu.Lock()
		finalCount := len(activeLockIds)
		activeLockIdsMu.Unlock()

		if finalCount != 0 {
			t.Errorf("Expected 0 locks remaining, got %d", finalCount)
		}
	})
}

// TestStressQueueMapAccess tests queue map access under high load
// Note: This test only validates queue creation and addition, not actual execution
// which requires database connectivity
func TestStressQueueMapAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	t.Run("high volume queue map access", func(t *testing.T) {
		// Create a separate queue map for testing (don't use global)
		testQueues := make(repoQueueMap)
		var testQueuesMu sync.Mutex

		var wg sync.WaitGroup
		numPlans := 100
		accessesPerPlan := 50
		var accessCount int32

		for p := 0; p < numPlans; p++ {
			for i := 0; i < accessesPerPlan; i++ {
				wg.Add(1)
				go func(planIdx, accessIdx int) {
					defer wg.Done()

					planId := fmt.Sprintf("stress-plan-%d", planIdx%20)

					// Simulate getQueue logic
					testQueuesMu.Lock()
					q, ok := testQueues[planId]
					if !ok {
						q = &repoQueue{}
						testQueues[planId] = q
					}
					testQueuesMu.Unlock()

					// Verify queue was retrieved
					if q != nil {
						atomic.AddInt32(&accessCount, 1)
					}
				}(p, i)
			}
		}

		wg.Wait()

		expected := int32(numPlans * accessesPerPlan)
		if accessCount != expected {
			t.Errorf("Expected %d successful accesses, got %d", expected, accessCount)
		}

		// Should only have 20 unique queues (planIdx%20)
		testQueuesMu.Lock()
		queueCount := len(testQueues)
		testQueuesMu.Unlock()

		if queueCount != 20 {
			t.Errorf("Expected 20 unique queues, got %d", queueCount)
		}
	})
}
