# Replay Mode Safety Guarantees

This document explains how replay mode differs from normal execution in terms of safety, side effects, and guarantees.

## Execution Modes Comparison

| Aspect | Normal Execution | Replay (Read-Only) | Replay (Simulate) | Replay (Apply) |
|--------|------------------|-------------------|-------------------|----------------|
| **AI API Calls** | ✅ Yes (live) | ❌ None | ❌ None | ❌ None |
| **File Writes** | ✅ Yes | ❌ None | ❌ None | ✅ Yes |
| **Token Cost** | ✅ Incurs cost | ❌ Free | ❌ Free | ❌ Free |
| **Deterministic** | ❌ No | ✅ Yes | ✅ Yes | ✅ Yes |
| **Reversible** | ✅ Atomic rollback via FileTransaction | ✅ N/A (no changes) | ✅ N/A (no changes) | ⚠️ Git revert |
| **Safe by Default** | ⚠️ No | ✅ Yes | ✅ Yes | ❌ Requires opt-in |

---

## Normal Execution

### What Happens
```
User Prompt → AI Model (live) → Parse Response → Build Files → Write to Disk
                  ↑                                                ↓
            Non-deterministic                              Permanent changes
```

### Risks
1. **Non-deterministic outputs** - Same prompt may yield different results
2. **Irreversible changes** - Files are modified immediately
3. **Cost accumulation** - Each execution consumes API tokens
4. **No preview** - Changes applied without inspection opportunity
5. **Cascading errors** - One bad model response can corrupt multiple files

### Safety Mechanisms
- Git history for rollback
- Manual review before `plandex apply`
- Build validation catches some syntax errors

---

## Replay Mode: Read-Only (Default)

### What Happens
```
Recorded Session → Display Steps → Show Diffs → No Side Effects
                        ↓
                  User inspects
```

### Guarantees

| Guarantee | Description |
|-----------|-------------|
| **No file modifications** | Filesystem is never touched |
| **No API calls** | No network requests to AI providers |
| **No cost** | Zero token consumption |
| **Fully reversible** | Nothing to reverse - no changes made |
| **Idempotent** | Can replay unlimited times with same result |
| **Safe to abort** | Quit anytime with no cleanup needed |

### Use Cases
- Understanding what happened in a failed run
- Learning how Plandex processed a prompt
- Debugging unexpected behavior
- Auditing changes before applying

### Code Path Difference

```go
// Normal execution
func executeStep(step *Step) {
    result := callAIModel(step.prompt)      // LIVE API CALL
    changes := parseResponse(result)
    writeFiles(changes)                      // WRITES TO DISK
}

// Replay read-only
func replayStep(step *RecordedStep) {
    displayRecordedResponse(step.response)   // Just display
    showRecordedDiff(step.diff)              // Just display
    // NO file writes, NO API calls
}
```

---

## Replay Mode: Simulate

### What Happens
```
Recorded Session → Compute What Would Happen → Compare to Current State → Report Divergences
                                                        ↓
                                               No actual changes
```

### Guarantees

| Guarantee | Description |
|-----------|-------------|
| **No file modifications** | Still read-only |
| **Divergence detection** | Warns if current state differs from recorded |
| **Preview changes** | See exactly what would be modified |
| **Validation** | Verify replay would succeed before committing |

### Additional Safety: Divergence Detection

```
Recorded State:  file.go contains "func old() {}"
Current State:   file.go contains "func new() {}"
                           ↓
              ⚠️ DIVERGENCE DETECTED
              "File content differs from recorded state"
```

Divergences indicate:
- Files were modified after the original run
- External processes changed the codebase
- Git operations altered the working directory
- Replay may not produce expected results

---

## Replay Mode: Apply

### What Happens
```
Recorded Session → Validate → Confirm with User → Apply Recorded Changes → Write to Disk
                                    ↑                        ↓
                           EXPLICIT OPT-IN            Uses recorded responses
                                                      (no new AI calls)
```

### Safety Mechanisms

1. **Explicit opt-in required**
   ```bash
   # Must specify --mode=apply
   plandex replay start abc123 --mode=apply
   ```

2. **Confirmation prompt**
   ```
   ⚠️  WARNING: Apply mode will make actual changes to files.
   This may modify, create, or delete files in your project.

   Are you sure you want to proceed in apply mode? (y/n)
   ```

3. **Uses recorded responses only**
   - No new AI API calls
   - Deterministic - same recorded response yields same changes
   - No risk of different/worse AI output

4. **Step-by-step control**
   - Can pause before each file change
   - Can skip specific steps
   - Can abort at any point

### Guarantees

| Guarantee | Description |
|-----------|-------------|
| **No AI calls** | Uses only recorded responses |
| **Deterministic** | Same recorded data → same changes |
| **User-controlled** | Explicit confirmation required |
| **Incremental** | Can apply step-by-step with inspection |
| **Git-backed** | Changes can be reverted via git |

### What Apply Mode Does NOT Guarantee

| Not Guaranteed | Why |
|----------------|-----|
| **Identical to original** | Current file state may differ |
| **No conflicts** | Recorded changes may not apply cleanly |
| **Correct behavior** | If original had bugs, replay has bugs |
| **External state** | Database, APIs, etc. not replayed |

---

## Safety Decision Matrix

### When to Use Each Mode

| Situation | Recommended Mode |
|-----------|------------------|
| "What happened in that failed run?" | Read-Only |
| "Will this replay work on my current code?" | Simulate |
| "Re-apply changes I reviewed and approved" | Apply |
| "Debug why the AI gave unexpected output" | Read-Only |
| "Verify divergence before re-running" | Simulate |
| "Apply specific steps only" | Apply with step selection |

### Risk Assessment

```
                    LOW RISK                              HIGH RISK
                        │                                     │
    Read-Only ──────────┼─── Simulate ─────────── Apply ─────┼─── Normal Exec
        │               │         │                  │        │        │
   No changes      No changes   Shows what      Actually    Live AI   Live AI
   No API calls    Validates    would happen    writes      Writes    + Writes
```

---

## Implementation Details

### How Read-Only Mode Prevents Side Effects

```go
func (e *ReplayExecutor) replayFileDiff(step *ReplayStep, result *ReplayStepResult) error {
    if e.options.Mode == ReplayModeReadOnly {
        // ONLY display - no file operations
        log.Printf("[Replay] File Diff: %s", diff.Path)
        if diff.UnifiedDiff != "" {
            log.Printf("[Replay] Diff preview:\n%s", truncateDiff(diff.UnifiedDiff, 500))
        }
        result.FileChanges = []*ReplayDiff{diff}
        result.Status = ReplayStepStatusCompleted
        return nil  // EXIT WITHOUT WRITING
    }
    // ... simulate and apply modes continue here
}
```

### How Simulate Mode Detects Divergence

```go
func (e *ReplayExecutor) checkFileDivergence(path, expectedContent string) *ReplayDivergence {
    currentSnapshot, _ := captureFileSnapshot(path)

    expectedHash := hashContent(expectedContent)
    if currentSnapshot.ContentHash != expectedHash {
        return &ReplayDivergence{
            Type:        "content_mismatch",
            Description: fmt.Sprintf("File content differs: %s", path),
            Expected:    truncate(expectedContent, 200),
            Actual:      truncate(currentSnapshot.Content, 200),
        }
    }
    return nil  // No divergence
}
```

### How Apply Mode Requires Confirmation

```go
// CLI enforces confirmation for apply mode
case "apply":
    mode = shared.ReplayModeApply
    fmt.Println(color.YellowString("⚠️  WARNING: Apply mode will make actual changes to files."))
    fmt.Println("This may modify, create, or delete files in your project.")

    confirmed, err := term.ConfirmYesNo("Are you sure you want to proceed in apply mode?")
    if err != nil || !confirmed {
        fmt.Println("Cancelled.")
        return  // EXIT WITHOUT STARTING REPLAY
    }
```

---

## Summary

| Mode | Side Effects | Cost | Deterministic | Default |
|------|-------------|------|---------------|---------|
| **Normal Execution** | Files + API | $$$ | ❌ | N/A |
| **Replay Read-Only** | None | Free | ✅ | ✅ Yes |
| **Replay Simulate** | None | Free | ✅ | No |
| **Replay Apply** | Files only | Free | ✅ | No (opt-in) |

**Key Insight**: Replay mode's primary safety advantage is **determinism** - the same recorded session always produces the same output, unlike normal execution where AI responses vary. Combined with the default read-only mode, this makes replay inherently safe for inspection and debugging.

---

## Integration with Recovery System

Replay mode integrates with the broader recovery and resilience system:

### Related Components

| Component | File | Role in Safety |
|-----------|------|----------------|
| **Provider Failures** | `provider_failures.go` | Classifies errors as retryable vs non-retryable; defines per-type `RetryStrategy` |
| **Retry Config** | `retry_config.go` | Configurable retry policy — env overrides, per-type backoff, Retry-After cap |
| **Operation Safety** | `operation_safety.go` | Blocks retrying irreversible operations (shell exec, external writes) |
| **Retry Context** | `retry_context.go` | Tracks every attempt with timing, strategy, fallback; bridges to journal + registry |
| **File Transactions** | `file_transaction.go` | WAL-backed transactions with persisted snapshots, sequential apply, reverse-order rollback, and crash recovery via `RecoverTransaction()` |
| **Resume Algorithm** | `resume_algorithm.go` | Safe continuation from checkpoints |
| **Error Reporting** | `error_report.go` | Surfaces root cause, context, recovery options |
| **Error Registry** | `error_registry.go` | Persistent error store; `StoreWithContext()` enriches with retry history |
| **Run Journal** | `run_journal.go` | `RecordRetryAttempt()` / `RecordRetryOutcome()` capture structured retry trace |
| **Unrecoverable Errors** | `unrecoverable_errors.go` | Documents edge cases; `DetectUnrecoverableCondition()` invoked from retry loop |

### How Components Work Together

```
Normal Execution (with failure):
    │
    ├─▶ Provider Failure ──▶ ClassifyModelError() ──▶ sets ProviderFailureType
    │         │
    │         ├─▶ RetryContext.RecordAttemptStart()
    │         │
    │         ├─▶ Retryable? ──▶ FileTransaction.Rollback()
    │         │                   (reverse-order restore from persisted snapshots)
    │         │                   ──▶ Retry
    │         │
    │         ├─▶ IsOperationSafe()? ──▶ block if irreversible
    │         │
    │         ├─▶ CanRetry()? ──▶ GetRetryStrategy() ──▶ ComputeBackoffDelay()
    │         │         │
    │         │         ├─▶ Sleep(delay)
    │         │         └─▶ RetryContext.RecordAttemptFailure()
    │         │
    │         ├─▶ Retryable? ──▶ FileTransaction.Rollback() ──▶ Retry attempt
    │         │
    │         └─▶ Non-Retryable / Exhausted?
    │                   │
    │                   ├─▶ DetectUnrecoverableCondition()
    │                   ├─▶ StoreWithContext(report, retryCtx)
    │                   ├─▶ RunJournal.RecordRetryOutcome()
    │                   └─▶ ErrorReport.Format() ──▶ User Action
    │
    └─▶ Journal + Checkpoint saved (snapshots flushed before writes)

Replay Mode (after failure):
    │
    ├─▶ List available sessions with `plandex replay list`
    │
    ├─▶ Inspect failure with `plandex replay show <id>`
    │   └─ Full retry trace visible in journal (attempt count, delays, fallbacks)
    │
    ├─▶ ResumeFromCheckpoint() ──▶ Validate state ──▶ Continue safely
    │
    └─▶ Apply mode with confirmation for controlled re-execution
```

### See Also

- [Provider Failures Documentation](./provider-failures.md) - Detailed failure classification
- [System Design - Recovery Section](./SYSTEM_DESIGN.md#11-recovery--resilience-system) - Architecture overview
