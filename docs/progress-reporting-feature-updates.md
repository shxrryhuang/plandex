# Progress Reporting System - Feature Updates

This document summarizes all the latest feature updates for the Plandex CLI progress reporting system.

---

## Overview

The progress reporting system has been redesigned to provide clear, real-time visibility into execution without overwhelming users. The system integrates seamlessly with the existing `stream_tui` Bubble Tea-based terminal interface.

---

## New Features

### 1. Guaranteed vs Best-Effort State Model

The system now distinguishes between reliable and transient state information:

| State Type | States | Meaning |
|------------|--------|---------|
| **Guaranteed** | `completed`, `failed`, `skipped` | Reflects committed, durable state changes |
| **Best-Effort** | `pending`, `running`, `waiting`, `stalled` | Signals about current activity |

This helps users understand what information they can rely on when deciding whether to wait, cancel, or investigate.

### 2. Execution Phase Tracking

Plan execution now proceeds through well-defined, visible phases:

```
Initializing → Planning → Describing → Building → Applying → Validating → Completed
                                                                        ↘ Failed
                                                                        ↘ Stopped
```

Each phase displays:
- Phase icon (e.g., 🚀, 🧠, 🏗, ✅)
- Phase label
- Elapsed time since phase started

### 3. Step-Level Progress Tracking

Each operation is tracked as an individual step with:

- **Unique ID** - For tracking and updates
- **Kind** - `llm_call`, `context`, `file_build`, `file_write`, `tool_exec`, `user_input`, `validation`
- **State** - Current status with reliability indicator
- **Label** - Human-readable description
- **Detail** - Additional context (file path, model name, etc.)
- **Timestamps** - Start and completion times
- **Metrics** - Tokens processed, progress percentage

### 4. Stall Detection

Steps are automatically monitored for stalls based on expected durations:

| Step Kind | Expected Duration | Stall Threshold |
|-----------|-------------------|-----------------|
| `llm_call` | 30s | 60s |
| `file_read` | 1s | 2s |
| `file_write` | 2s | 4s |
| `file_build` | 60s | 120s |
| `tool_exec` | 30s | 60s |
| `user_input` | ∞ | Never stalls |

When a step exceeds its stall threshold:
- State changes to `stalled`
- Warning icon (⚠) appears
- Suggested action guides the user

### 5. Toggle Progress View

Press `p` in the TUI to toggle between:
- **Classic View** - Original build output
- **Progress View** - New structured progress display

This allows users to choose their preferred visualization.

### 6. Visual Progress Indicators

**TTY Mode (Interactive Terminal):**
```
🏗 Building files                                     [1m12s]
  [████████████████████░░░░░░░░░░░░░░░░░░░░]  50%
⠹ 📄 Building file (src/api/handlers.go) 847🪙
● 📄 Building file (src/models/user.go)               15s
● 📄 Building file (src/config/config.go)              8s
(s)top • (b)ackground • (j/k) scroll • (p)rogress
```

**Icon Legend:**
| Icon | State | Meaning |
|------|-------|---------|
| ⠹ (spinner) | Running | Operation in progress |
| ● | Completed | Successfully finished (guaranteed) |
| ○ | Pending | Not started yet |
| ⚠ | Stalled | Needs attention |
| ✗ | Failed | Error occurred (guaranteed) |
| ◔ | Waiting | Blocked on external resource |

### 7. Non-TTY Log Format

For piped output, CI systems, or log files:

```
[14:23:45] [RUN ] planning > 🤖 Calling LLM (gpt-4-turbo)
[14:24:15] [DONE] planning > 🤖 Calling LLM (gpt-4-turbo) [30s]
[14:24:15] [RUN ] building > 📄 Building file (src/api.go)
[14:24:18] [DONE] building > 📄 Building file (src/api.go) [3s]
[14:24:25] [FAIL] building > 📄 Building file (src/broken.go) ERROR: syntax error [7s]
```

Format: `[TIME] [STATE] PHASE > ICON LABEL (DETAIL) [DURATION]`

State tags: `PEND`, `RUN`, `WAIT`, `STAL`, `DONE`, `FAIL`, `SKIP`

### 8. Contextual User Guidance

The system provides suggested actions based on current state:

| Scenario | Suggested Action |
|----------|------------------|
| LLM processing | "LLM is processing. Large tasks may take time." |
| Stalled operation | "Operation appears stalled. Consider canceling (s) if no progress." |
| Waiting for input | "Waiting for your input." |
| Tool executing | "External tool running. Cancel (s) if stuck." |

### 9. Token Count Tracking

File build steps display accumulated token counts:
- Shows tokens as they stream in (e.g., `847🪙`)
- Tokens accumulate correctly across multiple updates
- Helps users understand LLM activity

---

## Stream TUI Integration

### Automatic Phase Transitions

The TUI automatically transitions phases based on stream messages:

| Stream Message | Phase Transition |
|----------------|------------------|
| `StreamMessageReply` | → `PhasePlanning` |
| `StreamMessageDescribing` | → `PhaseDescribing` |
| `StreamMessageBuildInfo` | → `PhaseBuilding` |
| `StreamMessageFinished` | → `PhaseCompleted` |
| `StreamMessageError` | → `PhaseFailed` |
| `StreamMessageAborted` | → `PhaseStopped` |

### Step Tracking by Message Type

| Message Type | Step Created |
|--------------|--------------|
| `StreamMessageReply` (first) | LLM call step |
| `StreamMessageLoadContext` | Context loading step |
| `StreamMessageBuildInfo` | File build step (per path) |
| `StreamMessageMissingFilePath` | User input step |

### Thread-Safe State Management

All progress state updates use proper mutex synchronization:
- `stateMu` RWMutex protects all progress fields
- Map reads occur inside lock to prevent races
- Functions that acquire locks are called outside closures to prevent deadlocks

---

## New Files

| File | Purpose |
|------|---------|
| `app/shared/progress.go` | Core types shared between CLI and server |
| `app/cli/progress/renderer.go` | TTY and non-TTY rendering logic |
| `app/cli/progress/tracker.go` | Standalone tracker for external use |
| `app/cli/progress/stream_adapter.go` | Bridge for streaming protocol |
| `app/cli/progress/examples_test.go` | Test cases for all scenarios |
| `app/cli/progress/demo/main.go` | Demo program showing output examples |

---

## Modified Files

| File | Changes |
|------|---------|
| `app/cli/stream_tui/model.go` | Added progress tracking fields, toggle keybinding |
| `app/cli/stream_tui/view.go` | Added `renderProgressView()` and helpers |
| `app/cli/stream_tui/update.go` | Integrated progress tracking into message handlers |

---

## Configuration

Environment variables for customization:

| Variable | Default | Description |
|----------|---------|-------------|
| `PLANDEX_PROGRESS_VERBOSE` | `false` | Log all step events in non-TTY mode |
| `PLANDEX_STALL_THRESHOLD` | `60s` | Time before marking step as stalled |
| `PLANDEX_NO_SPINNER` | `false` | Disable animated spinner |
| `PLANDEX_PROGRESS_WIDTH` | auto | Force specific terminal width |

---

## Keyboard Shortcuts

New keybinding added to stream TUI:

| Key | Action |
|-----|--------|
| `p` | Toggle between classic and progress view |

Existing keybindings remain unchanged:
- `s` - Stop execution
- `b` - Background task
- `j/k` - Scroll through output

---

## Testing

All features verified with:

```bash
# Build verification
go build ./...

# Unit tests
go test -v ./progress/...

# Race detection
go test -race ./progress/...
```

---

## Demo

Run the demo to see example output:

```bash
cd app/cli
go run ./progress/demo/
```

Shows examples of:
1. Normal execution
2. Slow LLM call
3. Stalled operation (needs attention)
4. Failure scenario
5. Waiting for user input
6. Completed successfully
7. Non-TTY log format

---

## Related Documentation

- `docs/progress-reporting.md` - Full design documentation
- `docs/progress-reporting-bug-fixes.md` - Bug fixes applied during implementation
