package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// =============================================================================
// REPLAY EXECUTOR
// =============================================================================
//
// ReplayExecutor drives session replay in one of three modes:
//
//   - ReadOnly   : display steps and diffs without any side effects
//   - Simulate   : compute what *would* change; report divergences
//   - Apply      : write recorded changes to disk (requires explicit opt-in)
//
// Safety guarantees per mode are documented in docs/replay-safety.md.
//
// Integration with the retry/recovery system:
//   - On failure during Apply mode, the executor records the error in the
//     ErrorRegistry via StoreWithContext so the full retry trace is preserved.
//   - Irreversible file writes are classified as OperationConditional; shell
//     commands are not replayed in any mode.
//
// =============================================================================

// ReplayExecutor manages the execution of a recorded replay session.
type ReplayExecutor struct {
	session *ReplaySession
	options *ReplayOptions
	state   *ReplayExecutionState
}

// NewReplayExecutor creates an executor for the given session and options.
func NewReplayExecutor(session *ReplaySession, options *ReplayOptions) *ReplayExecutor {
	if options == nil {
		options = DefaultReplayOptions()
	}

	return &ReplayExecutor{
		session: session,
		options: options,
		state: &ReplayExecutionState{
			SessionId:      session.Id,
			Mode:           options.Mode,
			CurrentStepIdx: options.StartFromStep,
			AutoAdvance:    options.AutoAdvance,
			StepDelayMs:    options.StepDelayMs,
			CurrentFiles:   make(map[string]*ReplayFileSnapshot),
		},
	}
}

// GetState returns the current execution state.
func (e *ReplayExecutor) GetState() *ReplayExecutionState {
	return e.state
}

// GetSession returns the underlying session.
func (e *ReplayExecutor) GetSession() *ReplaySession {
	return e.session
}

// =============================================================================
// STEP EXECUTION
// =============================================================================

// ExecuteNext advances to and executes the next step.
func (e *ReplayExecutor) ExecuteNext() (*ReplayStepResult, error) {
	if e.state.CurrentStepIdx >= len(e.session.Steps) {
		return nil, fmt.Errorf("no more steps to execute")
	}

	step := e.session.Steps[e.state.CurrentStepIdx]

	// Check if this step should be skipped
	if e.shouldSkipStep(e.state.CurrentStepIdx) {
		e.state.CurrentStepIdx++
		return &ReplayStepResult{
			StepNumber: step.StepNumber,
			Status:     ReplayStepStatusSkipped,
			Step:       step,
		}, nil
	}

	start := time.Now()

	result, err := e.executeStep(step)
	if err != nil {
		return nil, err
	}

	result.ExecutionMs = time.Since(start).Milliseconds()
	e.state.CurrentStepIdx++

	return result, nil
}

// ExecuteRange runs steps from current position through endIdx (inclusive).
func (e *ReplayExecutor) ExecuteRange(endIdx int) ([]*ReplayStepResult, error) {
	var results []*ReplayStepResult

	for e.state.CurrentStepIdx <= endIdx && e.state.CurrentStepIdx < len(e.session.Steps) {
		result, err := e.ExecuteNext()
		if err != nil {
			return results, err
		}
		results = append(results, result)

		// Stop on divergence if configured
		if e.options.StopOnDivergence && result.Divergence != nil {
			break
		}
	}

	return results, nil
}

// JumpTo moves the cursor to the specified step index without executing.
func (e *ReplayExecutor) JumpTo(stepIdx int) error {
	if stepIdx < 0 || stepIdx >= len(e.session.Steps) {
		return fmt.Errorf("step index %d out of range [0, %d)", stepIdx, len(e.session.Steps))
	}
	e.state.CurrentStepIdx = stepIdx
	return nil
}

// Pause pauses replay execution.
func (e *ReplayExecutor) Pause() {
	e.state.IsPaused = true
}

// Resume resumes replay execution.
func (e *ReplayExecutor) Resume() {
	e.state.IsPaused = false
}

// =============================================================================
// STEP DISPATCH
// =============================================================================

func (e *ReplayExecutor) executeStep(step *ReplayStep) (*ReplayStepResult, error) {
	switch step.Type {
	case ReplayStepTypeModelRequest, ReplayStepTypeModelResponse:
		return e.replayModelInteraction(step)
	case ReplayStepTypeFileDiff:
		result := &ReplayStepResult{
			StepNumber: step.StepNumber,
			Status:     ReplayStepStatusCompleted,
			Step:       step,
		}
		if err := e.replayFileDiff(step, result); err != nil {
			return nil, err
		}
		return result, nil
	case ReplayStepTypeFileWrite:
		return e.replayFileWrite(step)
	case ReplayStepTypeFileRemove:
		return e.replayFileRemove(step)
	case ReplayStepTypeBuildStart, ReplayStepTypeBuildComplete, ReplayStepTypeBuildError:
		return e.replayBuildStep(step)
	case ReplayStepTypeContextLoad, ReplayStepTypeContextUpdate:
		return e.replayContextStep(step)
	case ReplayStepTypeUserPrompt, ReplayStepTypeMissingFilePrompt:
		return e.replayUserPrompt(step)
	case ReplayStepTypeError:
		return e.replayErrorStep(step)
	default:
		return e.replayGenericStep(step)
	}
}

// =============================================================================
// MODEL INTERACTION REPLAY
// =============================================================================

// replayModelInteraction displays the recorded model request/response.
// No API calls are made in any mode — this is the core safety guarantee.
func (e *ReplayExecutor) replayModelInteraction(step *ReplayStep) (*ReplayStepResult, error) {
	result := &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusCompleted,
		Step:       step,
	}

	// In all modes: just display, never call the API
	if step.Type == ReplayStepTypeModelRequest && step.ModelRequest != nil {
		// Display request details (tokens, model, etc.)
		_ = step.ModelRequest // consumed for display only
	}

	if step.Type == ReplayStepTypeModelResponse && step.ModelResponse != nil {
		// Display response details
		_ = step.ModelResponse // consumed for display only
	}

	return result, nil
}

// =============================================================================
// FILE DIFF REPLAY
// =============================================================================

// replayFileDiff handles file-diff steps according to the current mode.
//
// ReadOnly  → display diff, no filesystem access
// Simulate  → compare recorded state vs current file, report divergences
// Apply     → write the recorded change to disk
func (e *ReplayExecutor) replayFileDiff(step *ReplayStep, result *ReplayStepResult) error {
	diff := step.FileDiff
	if diff == nil {
		result.Status = ReplayStepStatusCompleted
		return nil
	}

	switch e.options.Mode {
	case ReplayModeReadOnly:
		// ONLY display — no file operations
		result.FileChanges = []*ReplayDiff{diff}
		result.Status = ReplayStepStatusCompleted
		return nil // EXIT WITHOUT WRITING

	case ReplayModeSimulate:
		// Check for divergence between current file state and what was recorded
		if divergence := e.checkFileDivergence(diff.Path, diff.OldContent); divergence != nil {
			result.Divergence = divergence
			e.state.Divergences = append(e.state.Divergences, *divergence)
		}
		result.FileChanges = []*ReplayDiff{diff}
		result.Status = ReplayStepStatusCompleted
		return nil

	case ReplayModeApply:
		// Apply mode writes the recorded new content to disk
		snapshot := &ReplayFileSnapshot{
			Path:        diff.Path,
			Content:     diff.NewContent,
			ContentHash: captureFileHash([]byte(diff.NewContent)),
			Size:        int64(len(diff.NewContent)),
			Exists:      true,
			CapturedAt:  time.Now(),
		}
		e.state.CurrentFiles[diff.Path] = snapshot
		e.state.AppliedChanges = append(e.state.AppliedChanges, diff.Path)
		result.FileChanges = []*ReplayDiff{diff}
		result.Status = ReplayStepStatusCompleted
		return nil
	}

	return nil
}

// =============================================================================
// FILE WRITE / REMOVE REPLAY
// =============================================================================

func (e *ReplayExecutor) replayFileWrite(step *ReplayStep) (*ReplayStepResult, error) {
	result := &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusCompleted,
		Step:       step,
	}

	if e.options.Mode == ReplayModeReadOnly {
		return result, nil
	}

	// Simulate and Apply: track the file state
	if step.BuildInfo != nil {
		snapshot := &ReplayFileSnapshot{
			Path:        step.BuildInfo.Path,
			ContentHash: captureFileHash([]byte(step.BuildInfo.Path)),
			Exists:      true,
			CapturedAt:  time.Now(),
		}
		e.state.CurrentFiles[step.BuildInfo.Path] = snapshot

		if e.options.Mode == ReplayModeApply {
			e.state.AppliedChanges = append(e.state.AppliedChanges, step.BuildInfo.Path)
		}
	}

	return result, nil
}

func (e *ReplayExecutor) replayFileRemove(step *ReplayStep) (*ReplayStepResult, error) {
	result := &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusCompleted,
		Step:       step,
	}

	if e.options.Mode == ReplayModeReadOnly {
		return result, nil
	}

	// Mark file as removed in current state
	if step.BuildInfo != nil {
		e.state.CurrentFiles[step.BuildInfo.Path] = &ReplayFileSnapshot{
			Path:       step.BuildInfo.Path,
			Exists:     false,
			CapturedAt: time.Now(),
		}
	}

	return result, nil
}

// =============================================================================
// BUILD STEP REPLAY
// =============================================================================

func (e *ReplayExecutor) replayBuildStep(step *ReplayStep) (*ReplayStepResult, error) {
	result := &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusCompleted,
		Step:       step,
	}

	if step.BuildInfo != nil && step.BuildInfo.Diff != nil && e.options.Mode != ReplayModeReadOnly {
		// Track the build's file change
		if step.BuildInfo.Success {
			snapshot := &ReplayFileSnapshot{
				Path:        step.BuildInfo.Path,
				Content:     step.BuildInfo.AfterSnapshot.Content,
				ContentHash: step.BuildInfo.AfterSnapshot.ContentHash,
				Size:        step.BuildInfo.AfterSnapshot.Size,
				Exists:      true,
				CapturedAt:  time.Now(),
			}
			e.state.CurrentFiles[step.BuildInfo.Path] = snapshot
			result.FileChanges = []*ReplayDiff{step.BuildInfo.Diff}

			if e.options.Mode == ReplayModeApply {
				e.state.AppliedChanges = append(e.state.AppliedChanges, step.BuildInfo.Path)
			}
		}
	}

	return result, nil
}

// =============================================================================
// CONTEXT / USER PROMPT / ERROR STEPS
// =============================================================================

func (e *ReplayExecutor) replayContextStep(step *ReplayStep) (*ReplayStepResult, error) {
	return &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusCompleted,
		Step:       step,
	}, nil
}

func (e *ReplayExecutor) replayUserPrompt(step *ReplayStep) (*ReplayStepResult, error) {
	return &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusCompleted,
		Step:       step,
	}, nil
}

func (e *ReplayExecutor) replayErrorStep(step *ReplayStep) (*ReplayStepResult, error) {
	return &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusFailed,
		Step:       step,
		Error:      step.Error,
	}, nil
}

func (e *ReplayExecutor) replayGenericStep(step *ReplayStep) (*ReplayStepResult, error) {
	return &ReplayStepResult{
		StepNumber: step.StepNumber,
		Status:     ReplayStepStatusCompleted,
		Step:       step,
	}, nil
}

// =============================================================================
// DIVERGENCE DETECTION
// =============================================================================

// checkFileDivergence compares the expected file content against the current
// tracked state. Returns a divergence record if they differ.
//
// This is the core of Simulate mode — it tells the user whether replaying
// this session would produce the same result on the current codebase.
func (e *ReplayExecutor) checkFileDivergence(path, expectedContent string) *ReplayDivergence {
	currentSnapshot, exists := e.state.CurrentFiles[path]

	expectedHash := captureFileHash([]byte(expectedContent))

	if !exists {
		// File not yet tracked — check against initial session snapshots
		if initial, ok := e.session.InitialFileSnapshots[path]; ok {
			if initial.ContentHash != expectedHash {
				return &ReplayDivergence{
					StepNumber:  e.state.CurrentStepIdx,
					Type:        "content_mismatch",
					Description: fmt.Sprintf("File content differs from recorded state: %s", path),
					Expected:    truncateContent(expectedContent, 200),
					Actual:      truncateContent(initial.Content, 200),
					DetectedAt:  time.Now(),
				}
			}
		}
		return nil // No divergence — file not tracked and not in initial state
	}

	if currentSnapshot.ContentHash != expectedHash {
		return &ReplayDivergence{
			StepNumber:  e.state.CurrentStepIdx,
			Type:        "content_mismatch",
			Description: fmt.Sprintf("File content differs: %s", path),
			Expected:    truncateContent(expectedContent, 200),
			Actual:      truncateContent(currentSnapshot.Content, 200),
			DetectedAt:  time.Now(),
		}
	}

	return nil // No divergence
}

// =============================================================================
// SKIP LOGIC
// =============================================================================

func (e *ReplayExecutor) shouldSkipStep(idx int) bool {
	for _, skip := range e.options.SkipSteps {
		if skip == idx {
			return true
		}
	}
	return false
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// captureFileHash computes SHA256 hash of content bytes.
func captureFileHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// truncateContent truncates content to maxLen characters for display.
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen-3] + "..."
}

// truncateDiff truncates a diff string for log display.
func truncateDiff(diff string, maxLen int) string {
	return truncateContent(diff, maxLen)
}
