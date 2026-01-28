# Progress Pipeline - Standalone Runner

This document describes the standalone progress pipeline that allows testing and demonstrating the progress reporting features **without modifying the original `stream_tui` code**.

---

## Overview

The pipeline package provides:

1. **Standalone Pipeline** - A self-contained progress tracking system
2. **Mock Stream Generator** - Simulates various execution scenarios
3. **Visual Runner** - Displays progress with TTY and non-TTY output formats
4. **Test Suite** - Comprehensive tests including race detection

---

## Architecture

```
progress/pipeline/
├── pipeline.go       # Core pipeline with phase/step tracking
├── mock_stream.go    # Scenario-based event generator
├── runner.go         # Visual output handler
├── pipeline_test.go  # Tests with race detection
└── cmd/
    └── main.go       # Standalone entry point
```

### Independence from Original Code

The pipeline operates completely independently:

- **No imports** from `stream_tui` package
- **No modifications** to existing files
- **Uses shared types** from `plandex-shared` (read-only)
- **Own callbacks** for progress events

This means you can test all progress features without any risk of breaking the existing TUI.

---

## Running the Pipeline

### Quick Start

```bash
cd app/cli

# Run all scenarios
go run ./progress/pipeline/cmd/

# Run specific scenario
go run ./progress/pipeline/cmd/ -scenario=normal
go run ./progress/pipeline/cmd/ -scenario=failure
go run ./progress/pipeline/cmd/ -scenario=stalled

# Use log format (for CI/non-TTY)
go run ./progress/pipeline/cmd/ -log
```

### Available Scenarios

| Scenario | Description |
|----------|-------------|
| `normal` | Successful execution with multiple files |
| `slow_llm` | Slow LLM response (extended streaming) |
| `stalled` | Operation that triggers stall detection |
| `failure` | Build failure with error message |
| `user_input` | Waiting for user input |
| `large_task` | Many files (20+) to build |
| `quick_task` | Fast, single-file task |
| `mixed` | Mix of success, failure, and skip |
| `all` | Run all scenarios sequentially |

### Command Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-scenario` | `all` | Which scenario to run |
| `-tty` | `true` | Enable TTY mode (animated output) |
| `-log` | `false` | Use log format (disables TTY) |

---

## Example Output

### TTY Mode (Interactive)

```
▶ Scenario: Normal Execution
----------------------------------------

🚀 Initializing [starting]
⠋ 📚 Loading context (5 files)
● 📚 Loading context (5 files) 150ms

🧠 Planning task [starting]
⠙ 🤖 Calling LLM (claude-3-opus)
● 🤖 Calling LLM (claude-3-opus) 600ms

🏗 Building files [starting]
⠼ 📄 Building file (src/api/handlers.go)
● 📄 Building file (src/api/handlers.go) 150ms
● 📄 Building file (src/models/user.go) 150ms
● 📄 Building file (src/config/config.go) 150ms

✅ Completed [541ms]
  5 completed
```

### Log Mode (CI/Non-TTY)

```
[14:23:45] [START ] context > Loading context | 5 files
[14:23:45] [COMPLETED] context > Loading context
[14:23:45] [PHASE ] planning > Planning task
[14:23:45] [START ] llm_call > Calling LLM | claude-3-opus
[14:23:45] [UPDATE] llm_call > Calling LLM | 500 tokens
[14:23:46] [COMPLETED] llm_call > Calling LLM
[14:23:46] [PHASE ] building > Building files
[14:23:46] [START ] file_build > Building file | src/api.go
[14:23:46] [COMPLETED] file_build > Building file
```

### Failure Scenario

```
🏗 Building files [starting]
⠦ 📄 Building file (src/api.go)
● 📄 Building file (src/api.go) 151ms
⠦ 📄 Building file (src/broken.go)
✗ 📄 Building file (src/broken.go) 151ms [syntax error at line 42: unexpected token]

❌ Failed [build failed: syntax error in src/broken.go]
```

---

## API Usage

### Creating a Pipeline

```go
import "plandex-cli/progress/pipeline"

// Create with default config
config := pipeline.DefaultConfig()
p := pipeline.New(config)

// Start tracking
p.Start()
defer p.Stop()
```

### Tracking Progress

```go
// Set execution phase
p.SetPhase(shared.PhasePlanning, "Planning task")

// Start a step
stepID := p.StartStep(shared.StepKindLLMCall, "Calling LLM", "gpt-4")

// Update step progress
p.UpdateStep(stepID, pipeline.StepUpdates{
    Tokens:   500,
    Progress: 0.5,
})

// Complete step
p.CompleteStep(stepID)

// Or fail step
p.FailStep(stepID, "rate limit exceeded")
```

### Using Callbacks

```go
config := pipeline.DefaultConfig()
config.OnPhaseChange = func(phase shared.ProgressPhase, label string) {
    fmt.Printf("Phase: %s - %s\n", phase, label)
}
config.OnStepStart = func(step *shared.Step) {
    fmt.Printf("Started: %s\n", step.Label)
}
config.OnStepEnd = func(step *shared.Step) {
    fmt.Printf("Ended: %s (%s)\n", step.Label, step.State)
}
config.OnStall = func(step *shared.Step) {
    fmt.Printf("STALLED: %s\n", step.Label)
}
```

### Running Mock Scenarios

```go
p := pipeline.New(pipeline.DefaultConfig())
p.Start()
defer p.Stop()

// Create mock stream
mockConfig := pipeline.DefaultMockConfig()
mockConfig.Scenario = pipeline.ScenarioNormal
mockConfig.TimeScale = 0.5 // 2x speed

mock := pipeline.NewMockStream(p, mockConfig)
err := mock.Run()
```

---

## Testing

### Run All Tests

```bash
cd app/cli
go test -v ./progress/pipeline/...
```

### Run with Race Detection

```bash
go test -race ./progress/pipeline/...
```

### Test Coverage

The test suite covers:

- Basic pipeline operations (create, start, stop)
- Step lifecycle (start, update, complete, fail, skip, wait)
- Callback invocation
- Mock stream scenarios
- Runner output (TTY and log formats)
- Concurrent access safety

---

## Relationship to stream_tui

| Aspect | Pipeline | stream_tui |
|--------|----------|------------|
| Purpose | Testing/demo | Production UI |
| Dependencies | Standalone | Bubble Tea framework |
| Event source | Mock stream | Real streaming API |
| Output | Direct stdout | TUI rendering |
| State storage | Local structs | Integrated model |

### Integration Path

The pipeline validates the progress model and rendering logic. Once verified, the same concepts are implemented in `stream_tui`:

1. **Pipeline defines** → Step, State, Phase types
2. **Pipeline tests** → All scenarios work correctly
3. **stream_tui implements** → Same logic with real events
4. **Both share** → Types from `plandex-shared`

---

## Files

| File | Lines | Description |
|------|-------|-------------|
| `pipeline.go` | ~380 | Core tracking with mutex-safe state |
| `mock_stream.go` | ~520 | 8 simulation scenarios |
| `runner.go` | ~250 | TTY/log output rendering |
| `pipeline_test.go` | ~230 | Tests with race detection |
| `cmd/main.go` | ~90 | CLI entry point |

---

## Configuration

### Pipeline Config

```go
type PipelineConfig struct {
    IsTTY          bool          // Enable TTY output
    Width          int           // Terminal width
    EnableColor    bool          // Use ANSI colors
    StallThreshold time.Duration // Time before stall warning

    // Callbacks
    OnPhaseChange func(phase, label)
    OnStepStart   func(step)
    OnStepUpdate  func(step)
    OnStepEnd     func(step)
    OnStall       func(step)
    OnComplete    func(report)
    OnError       func(err)
}
```

### Mock Config

```go
type MockStreamConfig struct {
    Scenario      Scenario      // Which scenario to run
    TimeScale     float64       // Speed multiplier (0.5 = 2x faster)
    FileCount     int           // Files to process
    SimulateFailure bool        // Force failure
    FailAtFile    int           // Which file fails
    SimulateStall bool          // Force stall
    StallDuration time.Duration // How long to stall
}
```

---

## Summary

The standalone pipeline provides a safe environment to:

1. **Test** all progress reporting features
2. **Demo** different execution scenarios
3. **Validate** the progress model design
4. **Verify** thread safety with race detection

All without touching the production `stream_tui` code.
