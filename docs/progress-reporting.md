# Plandex Progress Reporting System

This document describes the redesigned progress reporting system for the Plandex CLI. The goal is to provide clear, real-time visibility into execution without overwhelming users.

## Design Principles

### 1. Guaranteed vs Best-Effort State

The core insight is that not all progress information carries the same reliability:

| State Type | Meaning | User Interpretation |
|------------|---------|---------------------|
| **Guaranteed** | Reflects committed, durable state changes | "This definitely happened" |
| **Best-Effort** | Signals about current activity | "This is probably happening now" |

**Guaranteed States:**
- `completed` - Step finished successfully
- `failed` - Step failed with an error
- `skipped` - Step was intentionally skipped

**Best-Effort States:**
- `pending` - Step is queued
- `running` - Step appears to be executing
- `waiting` - Step is blocked on external resource
- `stalled` - Step hasn't progressed; may need attention

This distinction helps users understand what they can rely on when deciding whether to wait, cancel, or investigate.

### 2. Execution Phases

Plan execution proceeds through well-defined phases:

```
Initializing → Planning → Describing → Building → Applying → Validating → Completed
                                                                      ↘ Failed
                                                                      ↘ Stopped
```

Each phase has distinct characteristics:

| Phase | What Happens | User Expectations |
|-------|--------------|-------------------|
| **Initializing** | Loading context, validating config | Fast, < 10s typically |
| **Planning** | LLM reasoning about the task | Variable, depends on complexity |
| **Describing** | LLM describing proposed changes | Usually quick after planning |
| **Building** | Generating file content | Proportional to file count |
| **Applying** | Writing files, running commands | Fast per-file, but commands vary |
| **Validating** | Checking results, running tests | Depends on validation rules |

### 3. Step-Level Granularity

Each step represents a single unit of work:

```go
type Step struct {
    ID          string    // Unique identifier
    Kind        StepKind  // llm_call, file_build, tool_exec, etc.
    State       StepState // running, completed, failed, etc.
    Label       string    // Human-readable description
    Detail      string    // Additional context (file path, etc.)
    StartedAt   time.Time
    CompletedAt time.Time
    Progress    float64   // 0.0-1.0, or -1 for indeterminate
    Error       string    // Error message if failed
}
```

Step kinds have expected durations used for stall detection:

| Kind | Expected Duration | Stall Threshold |
|------|-------------------|-----------------|
| `llm_call` | 30s | 60s |
| `file_read` | 1s | 2s |
| `file_write` | 2s | 4s |
| `file_build` | 60s | 120s |
| `tool_exec` | 30s | 60s |
| `user_input` | ∞ | Never stalls |

---

## Visual Output

### TTY Mode (Interactive Terminal)

In an interactive terminal, progress updates dynamically with animations:

```
🧠 Planning task                                      [45s]
  [████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░]  30%
⠸ 🤖 Calling LLM (gpt-4-turbo)
● 📄 Loading context (12 files)                        8s
● 📄 Reading file (src/main.py)                        1s
💡 LLM is processing. Large tasks may take time.
(s)top • (b)ackground • (j/k) scroll
```

**Legend:**
- `⠸` Spinner (animates) - Operation in progress
- `●` Filled circle - Completed (guaranteed)
- `○` Empty circle - Pending
- `⚠` Warning - Stalled or needs attention
- `✗` X mark - Failed (guaranteed)

### Non-TTY Mode (Logs/CI)

For piped output or log files, each state change produces a structured line:

```
[14:23:45] [RUN ] planning > 🤖 Calling LLM (gpt-4-turbo)
[14:24:15] [DONE] planning > 🤖 Calling LLM (gpt-4-turbo) [30s]
[14:24:15] [RUN ] building > 📄 Building file (src/api.go)
[14:24:18] [DONE] building > 📄 Building file (src/api.go) [3s]
[14:24:18] [RUN ] building > 📄 Building file (src/handler.go)
[14:24:25] [FAIL] building > 📄 Building file (src/handler.go) ERROR: syntax error [7s]
```

Format: `[TIME] [STATE] PHASE > ICON LABEL (DETAIL) [DURATION]`

State tags:
- `PEND` - Pending
- `RUN ` - Running (best-effort)
- `WAIT` - Waiting (best-effort)
- `STAL` - Stalled (best-effort)
- `DONE` - Completed (guaranteed)
- `FAIL` - Failed (guaranteed)
- `SKIP` - Skipped (guaranteed)

---

## Example Scenarios

### Normal Execution

```
🚀 Initializing                                       [2s]
  [████████████████████████████████████████]  100%
● 📚 Loading context (5 files)                         2s
(s)top • (b)ackground • (j/k) scroll
```

```
🧠 Planning task                                      [35s]
  [████████████████████████████████████████]  100%
⠋ 🤖 Calling LLM (claude-3-opus)
● 📚 Loading context (5 files)                         2s
(s)top • (b)ackground • (j/k) scroll
```

```
🏗 Building files                                     [1m12s]
  [██████████████████░░░░░░░░░░░░░░░░░░░░░░]  45%
⠹ 📄 Building file (src/api/handlers.go) 847🪙
● 📄 Building file (src/models/user.go)               15s
● 📄 Building file (src/config/config.go)              8s
(s)top • (b)ackground • (j/k) scroll
```

```
✅ Completed                                          [2m34s]
  [████████████████████████████████████████]  100%
● 📄 Writing file (src/api/handlers.go)                2s
● 🔍 Validating (syntax check)                         3s
● 📦 Applied 4 files
```

### Slow External Call

When an LLM call takes longer than expected:

```
🧠 Planning task                                      [1m45s]
  [████████████████░░░░░░░░░░░░░░░░░░░░░░░░]  40%
⠧ 🤖 Calling LLM (gpt-4-turbo) [1m30s]
● 📚 Loading context (12 files)                       10s
💡 LLM is processing. Large tasks may take time.
(s)top • (b)ackground • (j/k) scroll
```

If it exceeds the stall threshold:

```
🧠 Planning task                                      [2m30s]
  [████████████████░░░░░░░░░░░░░░░░░░░░░░░░]  40%
⚠ 🤖 Calling LLM (gpt-4-turbo) [2m15s]
● 📚 Loading context (12 files)                       10s
⚠ Operation may be stalled
💡 Operation appears stalled. Consider canceling (s) if no progress.
(s)top • (b)ackground • (j/k) scroll
```

### Waiting for User Input

```
🏗 Building files                                     [45s]
  [████████████████████████████░░░░░░░░░░░░]  70%
◔ ⌨ Waiting for input (missing file: src/config.json)
● 📄 Building file (src/api.go)                       12s
● 📄 Building file (src/models.go)                     8s
💡 Waiting for your input.
(s)top • (b)ackground • (j/k) scroll
```

### Failure Scenario

```
🏗 Building files                                     [32s]
  [██████████████████████████░░░░░░░░░░░░░░]  65%
✗ 📄 Building file (src/broken.go) [syntax error at line 42]
● 📄 Building file (src/api.go)                       12s
● 📄 Building file (src/models.go)                     8s

Build failed: 1 error, 2 completed, 1 remaining
(s)top • (r)etry • view (l)ogs
```

### Tool Execution

```
📦 Applying changes                                   [15s]
  [████████████████████████░░░░░░░░░░░░░░░░]  60%
⠼ 🔧 Running tool (npm test)
● 📄 Writing file (src/api.go)                         1s
● 📄 Writing file (src/models.go)                      1s
💡 External tool running. Cancel (s) if stuck.
(s)top • (b)ackground • (j/k) scroll
```

---

## User Decision Guide

The progress system helps users decide what to do:

### When to Wait

- Phase is progressing (spinner animating, progress bar moving)
- Steps are completing regularly
- Current operation is within expected duration
- No stall warnings shown

### When to Consider Canceling

- ⚠ Stall warning appears
- No progress for extended period (2x expected duration)
- Suggested action mentions canceling
- You realize the task is wrong and want to retry

### When to Investigate

- `failed` state on a step - check the error message
- Warnings accumulating
- Unexpected behavior in output

### When to Background

- Task is proceeding normally but will take a while
- You want to do other work
- Large file set being processed

---

## API Usage

### Basic Tracking

```go
import "plandex-cli/progress"

// Create tracker
tracker := progress.NewTracker(progress.TrackerConfig{
    PlanID:  planID,
    Branch:  branch,
    Output:  os.Stdout,
    Verbose: true,
})
tracker.Start()
defer tracker.Stop()

// Track phases and steps
tracker.SetPhase(shared.PhasePlanning, "Planning task")

stepID := tracker.TrackLLMCall("gpt-4-turbo")
// ... do LLM call ...
tracker.UpdateStep(stepID, progress.StepUpdates{
    Tokens: 1500,
})
tracker.CompleteStep(stepID)

// Or on failure
tracker.FailStep(stepID, "rate limit exceeded")
```

### Streaming Integration

```go
// In stream handler
func handleStreamMessage(msg shared.StreamMessage, tracker *progress.Tracker) {
    switch msg.Type {
    case shared.StreamMessageBuildInfo:
        if msg.BuildInfo.Finished {
            tracker.CompleteStep(msg.BuildInfo.Path)
        } else {
            tracker.UpdateStep(msg.BuildInfo.Path, progress.StepUpdates{
                Tokens: msg.BuildInfo.NumTokens,
            })
        }
    case shared.StreamMessageError:
        tracker.FailStep(currentStepID, msg.Error.Msg)
    }
}
```

### Stall Callbacks

```go
tracker := progress.NewTracker(progress.TrackerConfig{
    // ...
    OnStall: func(step *shared.Step) {
        log.Printf("Step %s appears stalled after %v", step.ID, step.Duration())
        // Could notify user, attempt recovery, etc.
    },
})
```

---

## Log Format

For log aggregation systems, the non-TTY format provides structured data:

```
[14:23:45.123] START running: Loading context (12 files) [SIGNAL]
[14:23:47.456] END completed: Loading context (12 files) [CONFIRMED] | 2.3s
[14:23:47.500] START running: Calling LLM [SIGNAL] | gpt-4-turbo
[14:24:17.890] UPDATE running: Calling LLM [SIGNAL] | 1500 tokens
[14:24:35.123] END completed: Calling LLM [CONFIRMED] | gpt-4-turbo | 47.6s
```

Key fields:
- Timestamp with milliseconds
- Event type (START, UPDATE, END)
- State with reliability marker ([SIGNAL] or [CONFIRMED])
- Label and detail
- Duration for END events

This format:
- Parses easily with standard log tools
- Maintains readability for humans
- Distinguishes guaranteed from best-effort state
- Enables duration analysis and alerting

---

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PLANDEX_PROGRESS_VERBOSE` | `false` | Log all step events in non-TTY mode |
| `PLANDEX_STALL_THRESHOLD` | `60s` | Time before marking step as stalled |
| `PLANDEX_NO_SPINNER` | `false` | Disable animated spinner |
| `PLANDEX_PROGRESS_WIDTH` | auto | Force specific terminal width |

---

## Comparison with Previous System

| Aspect | Previous | New |
|--------|----------|-----|
| State clarity | Single spinner | Distinct states with reliability markers |
| Phase visibility | Implied by context | Explicit phase header |
| Step tracking | File-level only | Full step-level with nested support |
| Stall detection | Flash warning after 3s | Automatic detection with thresholds |
| Non-TTY output | Limited | Structured log format |
| User guidance | Help text only | Contextual suggested actions |
| Progress metrics | Token counts | Tokens, bytes, duration |

---

## Implementation Files

- `app/shared/progress.go` - Core types and state model
- `app/cli/progress/tracker.go` - State management and tracking (standalone usage)
- `app/cli/progress/renderer.go` - TTY and non-TTY rendering (standalone usage)
- `app/cli/progress/stream_adapter.go` - Bridge for streaming protocol (standalone usage)
- `app/cli/stream_tui/model.go` - Progress fields integrated into TUI model
- `app/cli/stream_tui/view.go` - Progress view rendering with `renderProgressView()`
- `app/cli/stream_tui/update.go` - Progress tracking on stream message handlers

## Stream TUI Integration

The progress system is fully integrated into the existing stream_tui. Key features:

### Toggle Progress View
Press `p` to toggle between the new progress view and the classic build view.

### Progress View Features
When enabled, the progress view shows:
- **Phase header** with icon and elapsed time (e.g., `🏗 Building files [1m12s]`)
- **Progress bar** showing completed vs total steps
- **Current step** with animated spinner
- **Recent completed steps** (last 3) with completion times
- **Stall warnings** when operations exceed expected duration
- **Suggested actions** for user guidance

### Automatic Phase Tracking
The TUI automatically transitions through phases based on stream messages:
- `StreamMessageReply` → `PhasePlanning`
- `StreamMessageDescribing` → `PhaseDescribing`
- `StreamMessageBuildInfo` → `PhaseBuilding`
- `StreamMessageFinished` → `PhaseCompleted`
- `StreamMessageError` → `PhaseFailed`
- `StreamMessageAborted` → `PhaseStopped`

### Step Tracking
Each operation is tracked as a step:
- **LLM calls** - Started on first reply, completed when replies finish
- **Context loading** - Tracked during `StreamMessageLoadContext`
- **File builds** - Each file path gets its own step with token counts
- **Missing file prompts** - Tracked as user input steps

The system integrates with the existing streaming architecture while providing richer visibility into execution state.
