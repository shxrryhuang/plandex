# Progress Reporting System — Feature Documentation

**Date:** 2026-01-28
**Scope:** Plandex CLI — real-time execution progress with phase tracking, stall detection, and TTY/non-TTY rendering
**Packages touched:** `plandex-shared`, `plandex-cli/term`, `plandex-cli/stream_tui`

---

## Table of Contents

1. [Motivation](#1-motivation)
2. [Architecture Overview](#2-architecture-overview)
3. [Data Model (`shared/progress.go`)](#3-data-model-sharedprogressgo)
4. [Renderer (`cli/term/progress_renderer.go`)](#4-renderer-clitermprogressrenderergo)
5. [Adapter (`cli/stream_tui/progress_adapter.go`)](#5-adapter-clistream_tuiprogress_adaptergo)
6. [Integration with Stream TUI](#6-integration-with-stream-tui)
7. [Two-Tier Confidence Model](#7-two-tier-confidence-model)
8. [Stall Detection & Heartbeat](#8-stall-detection--heartbeat)
9. [TTY vs Non-TTY Output](#9-tty-vs-non-tty-output)
10. [Scenarios](#10-scenarios)
11. [Bug Fixes Applied](#11-bug-fixes-applied)
12. [Test Coverage](#12-test-coverage)

---

## 1. Motivation

Before this change the CLI displayed only a bare spinner or per-file token counts while work was in progress. Users had no visibility into *where* in the pipeline execution was, *how long* each phase had been running, or *whether the system was actually making progress* versus waiting on an unresponsive external service.

Goals:
- Show the user exactly which phase is active and which phases have already completed.
- Surface stall conditions (no server heartbeat for >15 s) before the user has to guess.
- Degrade gracefully: colour + in-place updates in a terminal; plain timestamped lines when piped to a log file.
- Never block the rendering path or mutate shared state without synchronisation.

---

## 2. Architecture Overview

```
┌──────────────────────┐
│   Server / Wire      │   StreamMessage protocol (JSON over SSE)
└──────────┬───────────┘
           │  OnMessage(msg)
┌──────────▼───────────┐
│   ProgressAdapter    │   Translates wire messages → Progress state
│   (stream_tui)       │   Owns sync.RWMutex, stall timer
└──────────┬───────────┘
           │  Progress()
┌──────────▼───────────┐
│   ProgressRenderer   │   Pure function: Progress → []string
│   (cli/term)         │   TTY: coloured, in-place refresh
│                      │   Non-TTY: plain, timestamped append
└──────────┬───────────┘
           │  renders into
┌──────────▼───────────┐
│   Bubble Tea View    │   renderProcessing() in view.go
│   (stream_tui)       │   Falls back to spinner when adapter has no data
└──────────────────────┘
```

Three clean layers:

| Layer | File | Responsibility |
|-------|------|----------------|
| Model | `shared/progress.go` | Data types, lifecycle methods, queries |
| Renderer | `cli/term/progress_renderer.go` | Formatting, colour, truncation |
| Adapter | `cli/stream_tui/progress_adapter.go` | Protocol translation, concurrency, stall detection |

---

## 3. Data Model (`shared/progress.go`)

### 3.1 Phases

Six canonical phases executed in order:

| Phase | Constant | Description |
|-------|----------|-------------|
| Connect | `PhaseConnect` | Establishing or re-establishing the server connection |
| Context | `PhaseContext` | Loading or auto-loading context files |
| Model | `PhaseModel` | Sending the prompt and streaming the LLM reply |
| Build | `PhaseBuild` | Generating per-file edits from the reply |
| Apply | `PhaseApply` | Writing files to disk / running shell commands |
| Finalize | `PhaseFinalize` | Cleanup and summary generation |

`AllPhases` is the canonical slice used by the renderer to draw the phase bar left-to-right.

### 3.2 Step Statuses

| Status | Constant | Meaning |
|--------|----------|---------|
| Pending | `StepPending` | Not yet started |
| Running | `StepRunning` | In progress (best-effort — may revert to Stalled) |
| Completed | `StepCompleted` | Server-confirmed success |
| Failed | `StepFailed` | Server-confirmed failure |
| Skipped | `StepSkipped` | Intentionally not executed |
| Stalled | `StepStalled` | Was running but no heartbeat received in time |

`IsTerminal()` returns `true` for Completed, Failed, Skipped.
`IsGuaranteed()` is an alias — same set.

### 3.3 Step Struct

```go
type Step struct {
    ID          string
    Phase       PhaseID
    Label       string     // e.g. "model", "build"
    Status      StepStatus
    Confidence  Confidence // Guaranteed or BestEffort
    Detail      string     // e.g. file path, token count, error reason
    StartedAt   *time.Time
    FinishedAt  *time.Time
    DurationMs  int64
    Error       string
}
```

`Duration()` returns live elapsed time for running steps, or the recorded duration for completed ones.

### 3.4 Progress Struct

```go
type Progress struct {
    ActivePhase  PhaseID
    Steps        []*Step
    StartedAt    time.Time
    LastHeartbeat *time.Time
    Finished     bool
    Error        string
}
```

### 3.5 Lifecycle Methods

| Method | Effect |
|--------|--------|
| `NewProgress()` | Creates snapshot; sets `ActivePhase = PhaseConnect`, `StartedAt = now` |
| `AddStep(phase, label, detail)` | Appends a Pending/BestEffort step |
| `StartStep(step)` | Sets Running, records StartedAt, updates ActivePhase |
| `CompleteStep(step)` | Sets Completed/Guaranteed, records FinishedAt + DurationMs |
| `FailStep(step, errMsg)` | Sets Failed/Guaranteed, records error on step and on Progress |
| `SkipStep(step, reason)` | Sets Skipped/Guaranteed; reason goes into `Error` to preserve `Detail` |
| `MarkStalled(phase)` | Transitions Running → Stalled for all steps in the given phase |
| `RecordHeartbeat()` | Updates `LastHeartbeat` timestamp |

### 3.6 Query Methods

| Method | Returns |
|--------|---------|
| `ActiveSteps()` | Steps with status Running or Stalled |
| `CompletedSteps()` | Steps with status Completed |
| `FailedSteps()` | Steps with status Failed |
| `PhaseSteps(phase)` | All steps belonging to a given phase |
| `Summary()` | One-line human-readable overview |

### 3.7 Summary Format

```
[model] 2 done | 1 running | 0 failed (8s elapsed)
```

Running and stalled are reported as *separate* counters so the user is not misled into thinking a stalled operation is actively making progress.

### 3.8 Duration Formatting

`formatProgressDuration` produces compact strings:

| Input | Output |
|-------|--------|
| 0 – 999 ms | `<1s` |
| 1 – 59 s | `5s` |
| 1 – 59 min | `2m30s` or `5m` |
| ≥ 1 h | `1h5m` |

---

## 4. Renderer (`cli/term/progress_renderer.go`)

The renderer is a **pure function**: it reads a `*Progress` snapshot and produces `[]string`. It never modifies state.

### 4.1 Output Structure (top to bottom)

```
[✓ connect] [✓ context] [▶ model] [  build] [  apply] [  finalize]   ← Phase bar
  ▸ [running  ]  model › streaming reply                        5s    ← Active steps
  ✓ [completed]  context › loaded auth.go                      120ms  ← Recent completions (≤ 3)
  ✕ [failed   ]  build › src/main.go                   API timeout    ← Failures (all)
  No server heartbeat for >15s — ...                                  ← Stall warning (if any)
  [model] 2 done | 1 running (8s elapsed)                             ← Summary footer
```

### 4.2 Phase Bar

Each of the six phases rendered as a bracketed segment:

| State | Marker | Colour |
|-------|--------|--------|
| Completed (all steps terminal) | `[✓ phase]` | Bold green |
| Active (`== ActivePhase`) | `[▶ phase]` | Bold cyan |
| Pending (not yet reached) | `[  phase]` | Dim/faint |

`completedPhases()` uses two maps (`hasSteps`, `hasNonTerminal`) so the result is order-independent — a single non-terminal step keeps the whole phase incomplete regardless of when it was appended.

### 4.3 Step Lines

```
  <icon> [<badge>]  <label> › <detail>  <duration>
```

| Status | Icon | Badge Colour |
|--------|------|--------------|
| Completed | `✓` | Green |
| Failed | `✕` | Red |
| Running | `▸` | Cyan |
| Stalled | `⚠` | Yellow |
| Skipped | `◌` | Yellow |
| Pending | `·` | Dim |

Labels are truncated with `…` when they would exceed `width − 20` characters.

### 4.4 Stall Warning

Displayed whenever any step has status `StepStalled`:

```
  No server heartbeat for >15s — the operation may be waiting on an external service.
```

Rendered in yellow to draw the eye without implying failure.

### 4.5 Summary Footer

Delegates to `Progress.Summary()`. Colour-keyed:

| Condition | Colour |
|-----------|--------|
| Finished, no error | Bold green |
| Has error | Bold red |
| In progress | Dim |

### 4.6 TTY vs Non-TTY

| Mode | Behaviour |
|------|-----------|
| TTY | Cursor-up + clear (`\033[%dA` / `\033[J`) to overwrite previous block in-place |
| Non-TTY | Append timestamped plain-text lines; `stripANSI` removes any colour codes |

`stripANSI` has a 32-byte guard so a malformed/truncated escape sequence cannot swallow the rest of the input.

### 4.7 Colour Helpers

`colorize(s, attr, bold)` and `dim(s)` both call `c.EnableColor()` on the `fatih/color` object before rendering. This is required because the library's global `NoColor` flag is set `true` when stdout is not a terminal — but the renderer may be called from the Bubble Tea goroutine where the check is not meaningful. Explicit opt-in per-object is the correct override.

---

## 5. Adapter (`cli/stream_tui/progress_adapter.go`)

The adapter owns all mutable progress state and is the single place where `StreamMessage` events are translated into step lifecycle transitions.

### 5.1 Concurrency Model

```
┌─────────────────────────────────────────────┐
│  sync.RWMutex                               │
│                                             │
│  Write lock held by:                        │
│    • OnMessage (Bubble Tea update goroutine) │
│    • stallTimer callback (AfterFunc goroutine)│
│                                             │
│  Read lock held by:                         │
│    • Progress() (Bubble Tea view goroutine)  │
└─────────────────────────────────────────────┘
```

`OnMessage` acquires the write lock once and dispatches to `onMessageLocked`. Multi-envelope sub-messages recurse through `onMessageLocked` (not back to `OnMessage`) to avoid double-locking and double-processing.

### 5.2 Message → State Mapping

| StreamMessage Type | Adapter Action |
|--------------------|----------------|
| `Start` | Complete the connect step |
| `ConnectActive` | Complete connect; set detail to "reconnected to active plan" |
| `LoadContext` | Complete connect (if pending); create + start context step; set detail to file count |
| `Describing` | Complete context (if running); create/update model step with "generating description" |
| `Reply` (non-empty chunk) | Complete context (if running); create/update model step with "streaming reply" |
| `Reply` (empty chunk) | No-op |
| `RepliesFinished` | Complete model step |
| `BuildInfo` (new path) | Create + start per-file build step; set `ActivePhase = PhaseBuild` |
| `BuildInfo` (Finished) | Complete the build step for that path |
| `BuildInfo` (Removed) | Complete with detail "(removed)" |
| `BuildInfo` (nil) | No-op (guard) |
| `Finished` | Close all running steps; create + immediately complete finalize step; set `Finished = true` |
| `Aborted` | Close all running steps; set error "stopped by user"; set `Finished = true` |
| `Error` | Fail all running/stalled steps with the error message; set `Finished = true` |
| `Heartbeat` | Stall timer reset + heartbeat timestamp (no other state change) |
| `Multi` | Iterate sub-messages; each dispatched via `onMessageLocked` |

### 5.3 Model Re-entry

When the model phase is re-entered (e.g. after a missing-file prompt), the existing model step is recycled:

```go
a.modelStep.Status    = StepRunning
a.modelStep.StartedAt = &now
a.modelStep.FinishedAt = nil
a.modelStep.DurationMs = 0                        // cleared to avoid stale display
a.modelStep.Detail    = "streaming reply (continued)"
```

### 5.4 Stall Detection

A `time.AfterFunc` fires after `HeartbeatTimeout` (15 s). The callback:
1. Checks `stallDone` channel — returns immediately if the adapter has been shut down.
2. Acquires the write lock.
3. Transitions every `StepRunning` step to `StepStalled`.

Every call to `onMessageLocked` resets the timer via `resetStallTimer()` and records a heartbeat timestamp.

### 5.5 Shutdown

`Shutdown()` closes `stallDone` and stops the timer. It is idempotent — multiple calls are safe. Called on Finished, Aborted, or Error paths, and again in the Bubble Tea model's `cleanup()`.

---

## 6. Integration with Stream TUI

Three modifications to the existing `stream_tui` package wire the progress system in:

### 6.1 `model.go`

```go
// New fields on streamUIModel
progressAdapter  *ProgressAdapter
progressRenderer *term.ProgressRenderer

// Initialised in initialModel()
progressAdapter:  NewProgressAdapter(),
progressRenderer: term.NewProgressRenderer(),

// Shutdown in cleanup()
if m.progressAdapter != nil {
    m.progressAdapter.Shutdown()
}
```

### 6.2 `update.go`

Every `StreamMessage` is forwarded to the adapter *before* the existing per-type switch:

```go
func (m *streamUIModel) streamUpdate(msg *shared.StreamMessage, ...) {
    if m.progressAdapter != nil {
        m.progressAdapter.OnMessage(msg)   // ← new
    }
    switch msg.Type { ... }                // ← existing handlers unchanged
}
```

Viewport dimension calculation uses the cheap `progressHeight()` estimate instead of a full render:

```go
processingHeight := m.progressHeight()   // O(n) count, no string allocation
```

### 6.3 `view.go`

`renderProcessing()` now renders the phase bar when data is available, falling back to the original spinner:

```go
func (m streamUIModel) renderProcessing() string {
    // ... guard: !starting && !processing → ""

    if m.progressAdapter != nil && m.progressRenderer != nil {
        p := m.progressAdapter.Progress()
        if len(p.Steps) > 0 {
            lines := m.progressRenderer.Lines(p)
            return "\n" + strings.Join(lines, "\n")
        }
    }
    // Fallback: original spinner
    return "\n " + m.spinner.View()
}
```

`progressHeight()` mirrors the line-count logic of `Lines()` without building any strings — counts active, recent completions (capped at 3), failures, and an optional stall-warning line.

---

## 7. Two-Tier Confidence Model

| Tier | Value | When Set | Meaning |
|------|-------|----------|---------|
| **Guaranteed** | `ConfidenceGuaranteed` | `CompleteStep`, `FailStep`, `SkipStep` | Server explicitly confirmed this state. Immutable. Safe to act on (logs, retries, UX decisions). |
| **Best-Effort** | `ConfidenceBestEffort` | `AddStep`, `StartStep` | Client-side inference. The system *believes* the step is running but has not received terminal confirmation. May revert to Stalled if the heartbeat times out. Informational only. |

This distinction lets the user make informed decisions:

- **"1 done (guaranteed)"** → that phase is definitely finished; no need to watch it.
- **"1 running (best-effort)"** → probably fine, but if it stays here too long the stall warning will appear.
- **"1 stalled (best-effort)"** → something may be wrong; consider cancelling or waiting for an external service.

---

## 8. Stall Detection & Heartbeat

| Parameter | Value |
|-----------|-------|
| `HeartbeatTimeout` | 15 seconds |
| Timer location | `ProgressAdapter.stallTimer` |
| Reset trigger | Every call to `onMessageLocked` (any message type) |
| Stall action | All `StepRunning` steps → `StepStalled` |
| Visual indicator | Yellow `⚠` icon + warning line in renderer |
| User guidance | "No server heartbeat for >15s — the operation may be waiting on an external service." |

The timer callback acquires the write lock, so it cannot race with `OnMessage` or `Progress()`.

---

## 9. TTY vs Non-TTY Output

### TTY (interactive terminal)

```
[✓ connect] [✓ context] [▶ model] [  build] [  apply] [  finalize]
  ▸ [running  ]  model › streaming reply                        5s
  ✓ [completed]  context › 2 file(s)                           230ms
  [model] 1 done | 1 running (8s elapsed)
```

- Colour: green (done), cyan (active), yellow (warning), red (error), dim (pending).
- In-place refresh: cursor moved up `lastLen` lines, screen cleared from cursor, new block printed.

### Non-TTY (pipe / log file)

```
[2026-01-28T05:44:41Z] [✓ connect] [✓ context] [▶ model] ...
[2026-01-28T05:44:41Z]   ▸ [running  ]  model › streaming reply   5s
[2026-01-28T05:44:41Z]   [model] 1 done | 1 running (8s elapsed)
```

- No ANSI codes (`stripANSI` applied).
- Each render call appends; logs are fully replayable.
- Timestamps in RFC 3339 UTC.

---

## 10. Scenarios

### 10.1 Normal Execution (happy path)

```
[✓ connect] [✓ context] [✓ model] [✓ build] [  apply] [  finalize]
  ✓ [completed]  connect › established server connection         80ms
  ✓ [completed]  context › 2 file(s)                            230ms
  ✓ [completed]  model › reply complete                        2400ms
  ✓ [completed]  build › auth.go                               1100ms
  [build] 4 done (4s elapsed)
```

### 10.2 Mid-Stream Snapshot

```
[✓ connect] [✓ context] [▶ model] [  build] [  apply] [  finalize]
  ▸ [running  ]  model › streaming reply                        5s
  ✓ [completed]  context › 3 file(s)                           410ms
  [model] 1 done | 1 running (6s elapsed)
```

### 10.3 Stall Detected

```
[✓ connect] [✓ context] [▶ model] [  build] [  apply] [  finalize]
  ⚠ [stalled  ]  model › streaming reply                       18s
  ✓ [completed]  context › 1 file(s)                           120ms
  No server heartbeat for >15s — the operation may be waiting on an external service.
  [model] 1 done | 1 stalled (18s elapsed)
```

### 10.4 Failure

```
[✓ connect] [✓ context] [▶ model] [  build] [  apply] [  finalize]
  ✕ [failed   ]  model › streaming reply                        3s
  ✓ [completed]  context › 2 file(s)                           200ms
  [model] 1 done | 1 failed (3s elapsed) — model API rate limit exceeded
```

### 10.5 Concurrent Build Operations

```
[✓ connect] [✓ context] [✓ model] [▶ build] [  apply] [  finalize]
  ▸ [running  ]  build › src/auth.go — 150 tokens               2s
  ▸ [running  ]  build › src/main.go — 80 tokens                1s
  ▸ [running  ]  build › src/util.go — 45 tokens                1s
  ✓ [completed]  model › reply complete                        2400ms
  [build] 1 done | 3 running (4s elapsed)
```

### 10.6 User Abort

```
[✓ connect] [✓ context] [▶ model] [  build] [  apply] [  finalize]
  ✓ [completed]  connect › established                           80ms
  [connect] 1 done (1s elapsed) — stopped by user
```

---

## 11. Bug Fixes Applied

Nine bugs were identified and fixed during implementation and review:

| # | Bug | Root Cause | Fix |
|---|-----|-----------|-----|
| 1 | **Race condition** — stall timer goroutine writes `Step.Status` concurrently with `OnMessage` | `time.AfterFunc` callback had no synchronisation | Added `sync.RWMutex`; callback acquires write lock |
| 2 | **Double-feed of Multi sub-messages** — each sub-message processed twice by adapter | Public `OnMessage` called at top of `streamUpdate`, then the existing switch recurses calling `OnMessage` again per sub-message | Split into `OnMessage` (acquires lock once) + `onMessageLocked` (internal recursion without re-locking) |
| 3 | **`completedPhases` order-dependent** — a terminal step seen before a non-terminal one in the same phase left the phase marked complete | Single `seen` map with early-return logic | Separate `hasSteps` and `hasNonTerminal` maps; phase complete only if `hasSteps && !hasNonTerminal` |
| 4 | **`SkipStep` clobbers Detail** — unconditionally set `step.Detail = reason`, losing file-path metadata | Design oversight | Store reason in `step.Error` instead; `Detail` preserved |
| 5 | **Stale `DurationMs` on model re-entry** — recycling the model step reset `StartedAt`/`FinishedAt` but not `DurationMs` | Missing field reset | Added `a.modelStep.DurationMs = 0` on re-entry |
| 6 | **Unsynchronized read in View** — `renderProcessing` called `Progress()` without holding the mutex | `Progress()` returned the raw pointer without locking | `Progress()` now acquires `RLock` before returning |
| 7 | **`stripANSI` swallows trailing text** — a malformed escape sequence with no letter terminator consumed everything to EOF | No recovery mechanism | Added `maxEscLen = 32` guard; stops consuming after 32 bytes without a terminator |
| 8 | **Summary lumps stalled as running** — `ActiveSteps()` returns both Running and Stalled; `Summary()` used a single counter for both | Single "active" counter | Count `StepRunning` and `StepStalled` as separate categories in `Summary()` |
| 9 | **Expensive render in viewport dimension calc** — `getViewportDimensions` called `renderProcessing()` which now builds coloured strings on every chunk/tick | Performance regression from phase-bar integration | Added `progressHeight()` — returns line count via cheap integer arithmetic |

---

## 12. Test Coverage

### 12.1 Summary

| Package | Tests | Race-Clean |
|---------|-------|------------|
| `plandex-shared` (progress) | 22 | Yes |
| `plandex-cli/term` (renderer) | 16 | Yes |
| `plandex-cli/stream_tui` (adapter) | 21 | Yes |
| **Total** | **59** | **Yes** |

All tests run with `-race` flag enabled. Zero data-race warnings.

### 12.2 Progress Model Tests (`shared/progress_test.go`)

| Test | Verifies |
|------|----------|
| `TestNewProgress_InitialState` | Fresh Progress: ActivePhase=connect, 0 steps, Finished=false, StartedAt set |
| `TestAddStep_AppendsWithCorrectDefaults` | AddStep sets phase/label/detail, status=pending, confidence=BestEffort |
| `TestStartStep_SetsRunningAndTimestamp` | StartStep → running, StartedAt within time window, ActivePhase updated |
| `TestCompleteStep_GuaranteedState` | CompleteStep → completed, confidence=Guaranteed, FinishedAt set, DurationMs ≥ 0 |
| `TestFailStep_GuaranteedFailure` | FailStep → failed, confidence=Guaranteed, Error on step + Progress |
| `TestSkipStep_MarkedWithReason` | SkipStep → skipped, reason in Error, Detail preserved |
| `TestMarkStalled_OnlyAffectsRunning` | MarkStalled: running→stalled, completed and pending untouched |
| `TestRecordHeartbeat_UpdatesTimestamp` | RecordHeartbeat sets LastHeartbeat within time window |
| `TestActiveSteps_ReturnsRunningAndStalled` | ActiveSteps returns running + stalled, excludes completed |
| `TestPhaseSteps_FiltersCorrectly` | PhaseSteps returns only steps matching the requested phase |
| `TestSummary_FormatsCorrectly` | Summary returns non-empty meaningful string |
| `TestSummary_StalledReportedSeparately` | Stalled counted as "1 stalled", not "running" |
| `TestStepStatus_IsTerminal` | completed/failed/skipped=terminal; pending/running/stalled=non-terminal |
| `TestStepStatus_IsGuaranteed` | Same terminal set confirmed as guaranteed |
| `TestStep_Duration_Running` | Duration = time.Since(StartedAt) for running steps |
| `TestStep_Duration_Completed` | Duration = FinishedAt − StartedAt for completed steps |
| `TestFormatProgressDuration` | `<1s`, `5s`, `59s`, `2m30s`, `5m`, `1h5m` all formatted correctly |
| `TestPhaseID_PhaseLabel` | All 6 phases + unknown map to correct labels |
| `TestMultipleStepsPerPhase` | 3 build steps, out-of-order completion → correct active/completed counts |
| `TestExecutionPhaseConstants` | (pre-existing) Phase enum values are distinct |
| `TestReplayStep*` (4 tests) | (pre-existing) Replay step destructiveness, model interaction, duration, constants |

### 12.3 Renderer Tests (`cli/term/progress_renderer_test.go`)

| Test | Verifies |
|------|----------|
| `TestRenderer_PhaseBar_AllPending` | Initial state: `[▶ connect]` active, `[  model]` pending |
| `TestRenderer_PhaseBar_CompletedPhases` | `[✓ connect]` after completion, `[▶ model]` when active |
| `TestRenderer_StepLine_Running` | Running step contains label + detail + "running" |
| `TestRenderer_StepLine_Failed` | Failed step contains "failed" + label |
| `TestRenderer_StepLine_Stalled` | Stalled step contains "stalled" + heartbeat warning |
| `TestRenderer_SummaryLine_Normal` | Summary shows "1 done" + "1 running" |
| `TestRenderer_SummaryLine_Finished` | Finished summary shows "1 done" |
| `TestRenderer_SummaryLine_Error` | Error summary includes the error message |
| `TestRenderer_NoColorOutput` | Non-color mode: zero ANSI escape codes |
| `TestRenderer_ColorOutput` | Color mode: ANSI escape codes present |
| `TestRenderer_LabelTruncation` | Narrow terminal (40 cols) truncates with `…` |
| `TestRenderer_DurationStr_Seconds` | 5-second step → `5s` |
| `TestRenderer_DurationStr_Minutes` | 2m30s step → `2m30s` |
| `TestStripANSI` | Strips `\033[1m\033[32m...\033[0m` cleanly |
| `TestStripANSI_TruncatedSequence` | Malformed escape does not swallow trailing text |
| `TestRenderer_MultipleBuilds` | 3 concurrent build steps all appear in output |

### 12.4 Adapter Tests (`cli/stream_tui/progress_adapter_test.go`)

| Test | Verifies |
|------|----------|
| `TestAdapter_InitialState` | Fresh adapter: 1 running connect step |
| `TestAdapter_OnStart_CompletesConnect` | Start message completes connect with Guaranteed confidence |
| `TestAdapter_OnConnectActive_ResumeScenario` | ConnectActive sets "reconnected" detail |
| `TestAdapter_OnLoadContext_CreatesContextStep` | LoadContext creates running context step with file count |
| `TestAdapter_OnDescribing_StartsModelPhase` | Describing starts model step with "generating description" |
| `TestAdapter_OnReply_TransitionsToModelPhase` | Non-empty reply chunk → model step "streaming reply" |
| `TestAdapter_OnReply_EmptyChunkIgnored` | Empty ReplyChunk is a no-op |
| `TestAdapter_OnRepliesFinished_CompletesModelStep` | RepliesFinished completes model step with Guaranteed |
| `TestAdapter_OnBuildInfo_CreatesBuildSteps` | BuildInfo creates per-file build step |
| `TestAdapter_OnBuildInfo_CompletesOnFinished` | BuildInfo Finished=true completes the step |
| `TestAdapter_OnBuildInfo_HandlesRemoval` | BuildInfo Removed=true marks "(removed)" and completes |
| `TestAdapter_OnBuildInfo_MultipleConcurrentFiles` | 3 concurrent files tracked as 3 independent steps |
| `TestAdapter_OnFinished_ClosesAllSteps` | Finished message: all steps terminal, Finished=true |
| `TestAdapter_OnAborted_SetsErrorMessage` | Aborted: error="stopped by user", Finished=true |
| `TestAdapter_OnError_FailsRunningSteps` | Error: running steps failed with message, Finished=true |
| `TestAdapter_OnMulti_ProcessesAllSubmessages` | Multi envelope: each sub-message processed exactly once |
| `TestAdapter_HeartbeatResetsStallTimer` | Heartbeat recorded, step not stalled immediately after |
| `TestAdapter_StallDetection` | Manual MarkStalled transitions running→stalled |
| `TestAdapter_FullLifecycle` | End-to-end: connect → context → model → build → finalize |
| `TestAdapter_Shutdown_Idempotent` | Multiple Shutdown() calls: no panic |
| `TestAdapter_NilBuildInfo_Ignored` | nil BuildInfo field: no-op |

---

*End of document.*
