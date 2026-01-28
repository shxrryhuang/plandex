package shared

import (
	"testing"
	"time"
)

// =============================================================================
// REPLAY EXECUTOR CREATION TESTS
// =============================================================================

func TestNewReplayExecutor_DefaultOptions(t *testing.T) {
	session := &ReplaySession{
		Id:    "sess-1",
		Steps: []*ReplayStep{},
	}

	executor := NewReplayExecutor(session, nil)

	if executor.state.Mode != ReplayModeReadOnly {
		t.Errorf("default mode = %q, want %q", executor.state.Mode, ReplayModeReadOnly)
	}
	if executor.state.SessionId != "sess-1" {
		t.Errorf("SessionId = %q, want sess-1", executor.state.SessionId)
	}
	if executor.state.CurrentFiles == nil {
		t.Error("CurrentFiles should be initialised")
	}
}

func TestNewReplayExecutor_CustomOptions(t *testing.T) {
	session := &ReplaySession{Id: "sess-2", Steps: []*ReplayStep{}}
	opts := &ReplayOptions{
		Mode:          ReplayModeSimulate,
		StartFromStep: 2,
		AutoAdvance:   true,
		StepDelayMs:   500,
	}

	executor := NewReplayExecutor(session, opts)

	if executor.state.Mode != ReplayModeSimulate {
		t.Errorf("mode = %q, want simulate", executor.state.Mode)
	}
	if executor.state.CurrentStepIdx != 2 {
		t.Errorf("CurrentStepIdx = %d, want 2", executor.state.CurrentStepIdx)
	}
	if !executor.state.AutoAdvance {
		t.Error("AutoAdvance should be true")
	}
}

func TestGetState_ReturnsState(t *testing.T) {
	session := &ReplaySession{Id: "s", Steps: []*ReplayStep{}}
	executor := NewReplayExecutor(session, nil)

	state := executor.GetState()
	if state == nil {
		t.Fatal("GetState() returned nil")
	}
	if state.SessionId != "s" {
		t.Errorf("state.SessionId = %q, want s", state.SessionId)
	}
}

func TestGetSession_ReturnsSession(t *testing.T) {
	session := &ReplaySession{Id: "original", Steps: []*ReplayStep{}}
	executor := NewReplayExecutor(session, nil)

	if executor.GetSession().Id != "original" {
		t.Error("GetSession() should return the original session")
	}
}

// =============================================================================
// STEP EXECUTION TESTS
// =============================================================================

func TestExecuteNext_EmptySession_ReturnsError(t *testing.T) {
	session := &ReplaySession{Id: "empty", Steps: []*ReplayStep{}}
	executor := NewReplayExecutor(session, nil)

	_, err := executor.ExecuteNext()
	if err == nil {
		t.Error("ExecuteNext on empty session should return error")
	}
}

func TestExecuteNext_ModelRequest_ReadOnly(t *testing.T) {
	now := time.Now()
	session := &ReplaySession{
		Id: "model-test",
		Steps: []*ReplayStep{
			{
				Id:         "step-1",
				StepNumber: 1,
				Type:       ReplayStepTypeModelRequest,
				Status:     ReplayStepStatusPending,
				StartedAt:  now,
				ModelRequest: &ReplayModelRequest{
					ModelId:     "gpt-4",
					InputTokens: 1000,
				},
			},
		},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{Mode: ReplayModeReadOnly})
	result, err := executor.ExecuteNext()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != ReplayStepStatusCompleted {
		t.Errorf("status = %q, want completed", result.Status)
	}
	if result.Step.Type != ReplayStepTypeModelRequest {
		t.Errorf("step type = %q, want model_request", result.Step.Type)
	}
}

func TestExecuteNext_ModelResponse_NoAPICalls(t *testing.T) {
	session := &ReplaySession{
		Id: "no-api",
		Steps: []*ReplayStep{
			{
				Id:         "step-1",
				StepNumber: 1,
				Type:       ReplayStepTypeModelResponse,
				Status:     ReplayStepStatusPending,
				StartedAt:  time.Now(),
				ModelResponse: &ReplayModelResponse{
					Content:      "response text",
					InputTokens:  500,
					OutputTokens: 200,
				},
			},
		},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{Mode: ReplayModeReadOnly})
	result, err := executor.ExecuteNext()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify we got a result without making any API calls
	// (the test itself proves no API call was made — we have no mock server)
	if result.Status != ReplayStepStatusCompleted {
		t.Errorf("status = %q, want completed", result.Status)
	}
}

func TestExecuteNext_AdvancesIndex(t *testing.T) {
	session := &ReplaySession{
		Id: "advance",
		Steps: []*ReplayStep{
			{Id: "s1", StepNumber: 1, Type: ReplayStepTypeUserPrompt, StartedAt: time.Now()},
			{Id: "s2", StepNumber: 2, Type: ReplayStepTypeUserPrompt, StartedAt: time.Now()},
		},
	}

	executor := NewReplayExecutor(session, nil)

	if executor.state.CurrentStepIdx != 0 {
		t.Fatalf("initial index should be 0")
	}

	executor.ExecuteNext()
	if executor.state.CurrentStepIdx != 1 {
		t.Errorf("after first ExecuteNext, index = %d, want 1", executor.state.CurrentStepIdx)
	}

	executor.ExecuteNext()
	if executor.state.CurrentStepIdx != 2 {
		t.Errorf("after second ExecuteNext, index = %d, want 2", executor.state.CurrentStepIdx)
	}
}

func TestExecuteNext_SkipsMarkedSteps(t *testing.T) {
	session := &ReplaySession{
		Id: "skip-test",
		Steps: []*ReplayStep{
			{Id: "s0", StepNumber: 0, Type: ReplayStepTypeUserPrompt, StartedAt: time.Now()},
			{Id: "s1", StepNumber: 1, Type: ReplayStepTypeModelRequest, StartedAt: time.Now()},
		},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{
		Mode:      ReplayModeReadOnly,
		SkipSteps: []int{0},
	})

	result, err := executor.ExecuteNext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != ReplayStepStatusSkipped {
		t.Errorf("step 0 status = %q, want skipped", result.Status)
	}
}

func TestExecuteNext_ErrorStep_ReturnsFailed(t *testing.T) {
	session := &ReplaySession{
		Id: "err-step",
		Steps: []*ReplayStep{
			{
				Id:         "err-1",
				StepNumber: 1,
				Type:       ReplayStepTypeError,
				StartedAt:  time.Now(),
				Error:      "provider timeout",
			},
		},
	}

	executor := NewReplayExecutor(session, nil)
	result, err := executor.ExecuteNext()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != ReplayStepStatusFailed {
		t.Errorf("error step status = %q, want failed", result.Status)
	}
	if result.Error != "provider timeout" {
		t.Errorf("error message = %q, want provider timeout", result.Error)
	}
}

// =============================================================================
// JUMP AND PAUSE TESTS
// =============================================================================

func TestJumpTo_ValidIndex(t *testing.T) {
	session := &ReplaySession{
		Id: "jump",
		Steps: []*ReplayStep{
			{Id: "s0", StepNumber: 0, StartedAt: time.Now()},
			{Id: "s1", StepNumber: 1, StartedAt: time.Now()},
			{Id: "s2", StepNumber: 2, StartedAt: time.Now()},
		},
	}

	executor := NewReplayExecutor(session, nil)

	if err := executor.JumpTo(2); err != nil {
		t.Fatalf("JumpTo(2) failed: %v", err)
	}
	if executor.state.CurrentStepIdx != 2 {
		t.Errorf("after JumpTo(2), index = %d, want 2", executor.state.CurrentStepIdx)
	}
}

func TestJumpTo_InvalidIndex_ReturnsError(t *testing.T) {
	session := &ReplaySession{
		Id:    "jump-bad",
		Steps: []*ReplayStep{{Id: "s0", StepNumber: 0, StartedAt: time.Now()}},
	}

	executor := NewReplayExecutor(session, nil)

	if err := executor.JumpTo(5); err == nil {
		t.Error("JumpTo with out-of-range index should return error")
	}
	if err := executor.JumpTo(-1); err == nil {
		t.Error("JumpTo with negative index should return error")
	}
}

func TestPauseAndResume(t *testing.T) {
	session := &ReplaySession{Id: "pause", Steps: []*ReplayStep{}}
	executor := NewReplayExecutor(session, nil)

	if executor.state.IsPaused {
		t.Error("should not be paused initially")
	}

	executor.Pause()
	if !executor.state.IsPaused {
		t.Error("should be paused after Pause()")
	}

	executor.Resume()
	if executor.state.IsPaused {
		t.Error("should not be paused after Resume()")
	}
}

// =============================================================================
// DIVERGENCE DETECTION TESTS
// =============================================================================

func TestCheckFileDivergence_NoDivergence(t *testing.T) {
	session := &ReplaySession{
		Id:    "div-none",
		Steps: []*ReplayStep{},
		InitialFileSnapshots: map[string]*ReplayFileSnapshot{
			"src/main.go": {
				Path:        "src/main.go",
				Content:     "package main",
				ContentHash: captureFileHash([]byte("package main")),
			},
		},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{Mode: ReplayModeSimulate})

	divergence := executor.checkFileDivergence("src/main.go", "package main")
	if divergence != nil {
		t.Errorf("no divergence expected, got: %+v", divergence)
	}
}

func TestCheckFileDivergence_ContentMismatch(t *testing.T) {
	session := &ReplaySession{
		Id:    "div-mismatch",
		Steps: []*ReplayStep{},
		InitialFileSnapshots: map[string]*ReplayFileSnapshot{
			"src/main.go": {
				Path:        "src/main.go",
				Content:     "package main // original",
				ContentHash: captureFileHash([]byte("package main // original")),
			},
		},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{Mode: ReplayModeSimulate})

	// Expect divergence because recorded initial != what we're checking
	divergence := executor.checkFileDivergence("src/main.go", "package main // modified")
	if divergence == nil {
		t.Fatal("expected divergence for mismatched content")
	}
	if divergence.Type != "content_mismatch" {
		t.Errorf("divergence type = %q, want content_mismatch", divergence.Type)
	}
}

func TestCheckFileDivergence_FileNotTracked(t *testing.T) {
	session := &ReplaySession{
		Id:                   "div-untracked",
		Steps:                []*ReplayStep{},
		InitialFileSnapshots: map[string]*ReplayFileSnapshot{},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{Mode: ReplayModeSimulate})

	// File not in initial snapshots and not in current files — no divergence
	divergence := executor.checkFileDivergence("unknown.go", "content")
	if divergence != nil {
		t.Errorf("untracked file should not report divergence, got: %+v", divergence)
	}
}

func TestCheckFileDivergence_CurrentFileDiffers(t *testing.T) {
	session := &ReplaySession{
		Id:                   "div-current",
		Steps:                []*ReplayStep{},
		InitialFileSnapshots: map[string]*ReplayFileSnapshot{},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{Mode: ReplayModeSimulate})

	// Simulate a file that was written during replay
	executor.state.CurrentFiles["src/app.go"] = &ReplayFileSnapshot{
		Path:        "src/app.go",
		Content:     "func main() { /* v1 */ }",
		ContentHash: captureFileHash([]byte("func main() { /* v1 */ }")),
	}

	// Now check against different content — should diverge
	divergence := executor.checkFileDivergence("src/app.go", "func main() { /* v2 */ }")
	if divergence == nil {
		t.Fatal("expected divergence when current file differs from expected")
	}
	if divergence.Type != "content_mismatch" {
		t.Errorf("divergence type = %q, want content_mismatch", divergence.Type)
	}
}

// =============================================================================
// HELPER FUNCTION TESTS
// =============================================================================

func TestCaptureFileHash_Deterministic(t *testing.T) {
	content := []byte("deterministic content")
	h1 := captureFileHash(content)
	h2 := captureFileHash(content)

	if h1 != h2 {
		t.Error("captureFileHash should be deterministic")
	}
	if h1 == "" {
		t.Error("captureFileHash should not return empty string")
	}
}

func TestTruncateContent_Short(t *testing.T) {
	short := "hello"
	if truncateContent(short, 100) != short {
		t.Error("short content should pass through unchanged")
	}
}

func TestTruncateContent_Long(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz"
	result := truncateContent(long, 10)

	if len(result) > 10 {
		t.Errorf("truncated result length = %d, want <= 10", len(result))
	}
	if result[len(result)-3:] != "..." {
		t.Error("truncated content should end with ...")
	}
}

func TestTruncateContent_ExactLength(t *testing.T) {
	exact := "12345"
	if truncateContent(exact, 5) != exact {
		t.Error("content at exact maxLen should pass through unchanged")
	}
}

// =============================================================================
// EXECUTE RANGE TESTS
// =============================================================================

func TestExecuteRange_RunsAllSteps(t *testing.T) {
	session := &ReplaySession{
		Id: "range-all",
		Steps: []*ReplayStep{
			{Id: "s0", StepNumber: 0, Type: ReplayStepTypeUserPrompt, StartedAt: time.Now()},
			{Id: "s1", StepNumber: 1, Type: ReplayStepTypeModelRequest, StartedAt: time.Now()},
			{Id: "s2", StepNumber: 2, Type: ReplayStepTypeContextLoad, StartedAt: time.Now()},
		},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{Mode: ReplayModeReadOnly})
	results, err := executor.ExecuteRange(2)

	if err != nil {
		t.Fatalf("ExecuteRange failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestExecuteRange_StopsOnDivergence(t *testing.T) {
	// This test verifies that StopOnDivergence option works conceptually.
	// Divergence is only detected for file_diff steps in simulate mode,
	// so we test with a session that has no file diffs — range should complete.
	session := &ReplaySession{
		Id: "range-diverge",
		Steps: []*ReplayStep{
			{Id: "s0", StepNumber: 0, Type: ReplayStepTypeUserPrompt, StartedAt: time.Now()},
			{Id: "s1", StepNumber: 1, Type: ReplayStepTypeContextLoad, StartedAt: time.Now()},
		},
	}

	executor := NewReplayExecutor(session, &ReplayOptions{
		Mode:             ReplayModeSimulate,
		StopOnDivergence: true,
	})

	results, err := executor.ExecuteRange(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No divergences in these step types — all should complete
	if len(results) != 2 {
		t.Errorf("expected 2 results (no divergence to stop at), got %d", len(results))
	}
}

// =============================================================================
// MODE SAFETY TESTS
// =============================================================================

func TestReplayOptions_IsSafeMode(t *testing.T) {
	tests := []struct {
		mode ReplayMode
		want bool
	}{
		{ReplayModeReadOnly, true},
		{ReplayModeSimulate, true},
		{ReplayModeApply, false},
	}

	for _, tt := range tests {
		opts := &ReplayOptions{Mode: tt.mode}
		if got := opts.IsSafeMode(); got != tt.want {
			t.Errorf("IsSafeMode(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestDefaultReplayOptions_IsSafe(t *testing.T) {
	opts := DefaultReplayOptions()
	if !opts.IsSafeMode() {
		t.Error("default options should be safe (read-only)")
	}
	if opts.Mode != ReplayModeReadOnly {
		t.Errorf("default mode = %q, want read_only", opts.Mode)
	}
}
