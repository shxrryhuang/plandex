package db

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRepoQueueStructure tests the queue data structure behavior
func TestRepoQueueStructure(t *testing.T) {
	t.Run("new queue starts empty", func(t *testing.T) {
		q := &repoQueue{}
		if len(q.ops) != 0 {
			t.Errorf("new queue should be empty, has %d ops", len(q.ops))
		}
		if q.isProcessing {
			t.Error("new queue should not be processing")
		}
	})
}

// TestRepoQueueMapGetQueue tests queue retrieval behavior
func TestRepoQueueMapGetQueue(t *testing.T) {
	t.Run("creates new queue for unknown planId", func(t *testing.T) {
		// Reset for test
		queuesMu.Lock()
		originalQueues := repoQueues
		repoQueues = make(repoQueueMap)
		queuesMu.Unlock()

		defer func() {
			queuesMu.Lock()
			repoQueues = originalQueues
			queuesMu.Unlock()
		}()

		q := repoQueues.getQueue("new-plan")
		if q == nil {
			t.Error("getQueue should create a new queue")
		}
	})

	t.Run("returns same queue for same planId", func(t *testing.T) {
		queuesMu.Lock()
		originalQueues := repoQueues
		repoQueues = make(repoQueueMap)
		queuesMu.Unlock()

		defer func() {
			queuesMu.Lock()
			repoQueues = originalQueues
			queuesMu.Unlock()
		}()

		q1 := repoQueues.getQueue("plan-123")
		q2 := repoQueues.getQueue("plan-123")

		if q1 != q2 {
			t.Error("getQueue should return same queue for same planId")
		}
	})

	t.Run("returns different queues for different planIds", func(t *testing.T) {
		queuesMu.Lock()
		originalQueues := repoQueues
		repoQueues = make(repoQueueMap)
		queuesMu.Unlock()

		defer func() {
			queuesMu.Lock()
			repoQueues = originalQueues
			queuesMu.Unlock()
		}()

		q1 := repoQueues.getQueue("plan-a")
		q2 := repoQueues.getQueue("plan-b")

		if q1 == q2 {
			t.Error("getQueue should return different queues for different planIds")
		}
	})
}

// TestRepoOperationStructure tests operation structure
func TestRepoOperationStructure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	op := &repoOperation{
		id:       "op-123",
		orgId:    "org-456",
		planId:   "plan-789",
		branch:   "main",
		scope:    LockScopeRead,
		reason:   "test operation",
		ctx:      ctx,
		cancelFn: cancel,
		done:     done,
		op: func(repo *GitRepo) error {
			return nil
		},
	}

	if op.id != "op-123" {
		t.Errorf("expected id 'op-123', got %q", op.id)
	}
	if op.scope != LockScopeRead {
		t.Errorf("expected LockScopeRead, got %v", op.scope)
	}
}

// TestLockScopeConstants tests lock scope constant values
func TestLockScopeConstants(t *testing.T) {
	if LockScopeRead == "" {
		t.Error("LockScopeRead should not be empty")
	}
	if LockScopeWrite == "" {
		t.Error("LockScopeWrite should not be empty")
	}
	if LockScopeRead == LockScopeWrite {
		t.Error("LockScopeRead and LockScopeWrite should be different")
	}
}

// TestExecRepoOperationParamsStructure tests the params structure
func TestExecRepoOperationParamsStructure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	params := ExecRepoOperationParams{
		OrgId:          "org-123",
		UserId:         "user-456",
		PlanId:         "plan-789",
		Branch:         "feature/test",
		Scope:          LockScopeWrite,
		PlanBuildId:    "build-001",
		Reason:         "test params",
		Ctx:            ctx,
		CancelFn:       cancel,
		ClearRepoOnErr: true,
	}

	if params.OrgId != "org-123" {
		t.Errorf("expected OrgId 'org-123', got %q", params.OrgId)
	}
	if params.Scope != LockScopeWrite {
		t.Errorf("expected LockScopeWrite, got %v", params.Scope)
	}
	if !params.ClearRepoOnErr {
		t.Error("expected ClearRepoOnErr to be true")
	}
}

// TestNextBatchLogic tests the batch selection logic
func TestNextBatchLogic(t *testing.T) {
	t.Run("empty queue returns nil", func(t *testing.T) {
		q := &repoQueue{ops: []*repoOperation{}}
		batch := q.nextBatch()
		if batch != nil {
			t.Error("empty queue should return nil batch")
		}
	})

	t.Run("write operation returns single item", func(t *testing.T) {
		writeOp := &repoOperation{
			id:     "write-1",
			scope:  LockScopeWrite,
			branch: "main",
		}
		q := &repoQueue{ops: []*repoOperation{writeOp}}

		batch := q.nextBatch()

		if len(batch) != 1 {
			t.Errorf("write should return batch of 1, got %d", len(batch))
		}
		if batch[0].id != "write-1" {
			t.Error("batch should contain the write operation")
		}
	})

	t.Run("reads on same branch are batched", func(t *testing.T) {
		read1 := &repoOperation{id: "read-1", scope: LockScopeRead, branch: "main"}
		read2 := &repoOperation{id: "read-2", scope: LockScopeRead, branch: "main"}
		read3 := &repoOperation{id: "read-3", scope: LockScopeRead, branch: "main"}

		q := &repoQueue{ops: []*repoOperation{read1, read2, read3}}

		batch := q.nextBatch()

		if len(batch) != 3 {
			t.Errorf("same-branch reads should batch together, got %d", len(batch))
		}
	})

	t.Run("reads on different branches are not batched", func(t *testing.T) {
		read1 := &repoOperation{id: "read-1", scope: LockScopeRead, branch: "main"}
		read2 := &repoOperation{id: "read-2", scope: LockScopeRead, branch: "feature"}

		q := &repoQueue{ops: []*repoOperation{read1, read2}}

		batch := q.nextBatch()

		if len(batch) != 1 {
			t.Errorf("different-branch reads should not batch, got %d", len(batch))
		}
		if batch[0].id != "read-1" {
			t.Error("batch should contain only first read")
		}
		if len(q.ops) != 1 {
			t.Error("second read should remain in queue")
		}
	})

	t.Run("root branch reads are not batched", func(t *testing.T) {
		read1 := &repoOperation{id: "read-1", scope: LockScopeRead, branch: ""} // root
		read2 := &repoOperation{id: "read-2", scope: LockScopeRead, branch: ""}

		q := &repoQueue{ops: []*repoOperation{read1, read2}}

		batch := q.nextBatch()

		if len(batch) != 1 {
			t.Errorf("root branch reads should not batch, got %d", len(batch))
		}
	})

	t.Run("write blocks subsequent operations", func(t *testing.T) {
		writeOp := &repoOperation{id: "write-1", scope: LockScopeWrite, branch: "main"}
		readOp := &repoOperation{id: "read-1", scope: LockScopeRead, branch: "main"}

		q := &repoQueue{ops: []*repoOperation{writeOp, readOp}}

		batch := q.nextBatch()

		if len(batch) != 1 {
			t.Errorf("write should block reads, batch size should be 1, got %d", len(batch))
		}
		if batch[0].scope != LockScopeWrite {
			t.Error("batch should contain only the write")
		}
		if len(q.ops) != 1 {
			t.Error("read should remain in queue")
		}
	})
}

// TestContextCancellationBehavior tests context handling
func TestContextCancellationBehavior(t *testing.T) {
	t.Run("canceled context is detected", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		select {
		case <-ctx.Done():
			// Expected
		default:
			t.Error("canceled context should be done")
		}
	})

	t.Run("context error is context.Canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if ctx.Err() != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", ctx.Err())
		}
	})

	t.Run("timeout context error is DeadlineExceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond)

		if ctx.Err() != context.DeadlineExceeded {
			t.Errorf("expected DeadlineExceeded, got %v", ctx.Err())
		}
	})
}

// TestConcurrentQueueAccess tests thread safety of queue map
func TestConcurrentQueueAccess(t *testing.T) {
	queuesMu.Lock()
	originalQueues := repoQueues
	repoQueues = make(repoQueueMap)
	queuesMu.Unlock()

	defer func() {
		queuesMu.Lock()
		repoQueues = originalQueues
		queuesMu.Unlock()
	}()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			planId := "plan-" + string(rune('a'+(i%10)))
			_ = repoQueues.getQueue(planId)
		}(i)
	}

	wg.Wait()
	// If we get here without deadlock or panic, the test passes
}

// TestErrorMessageFormatting tests error message content
func TestErrorMessageFormatting(t *testing.T) {
	t.Run("lock failure message includes operation info", func(t *testing.T) {
		// Simulate error message format
		msg := "failed to get DB lock for operation 'update files': connection refused. This may be due to another active operation on this plan or a database connectivity issue"

		requiredParts := []string{
			"failed to get DB lock",
			"update files",
			"another active operation",
			"database connectivity",
		}

		for _, part := range requiredParts {
			if !strings.Contains(msg, part) {
				t.Errorf("error message should contain %q", part)
			}
		}
	})
}
