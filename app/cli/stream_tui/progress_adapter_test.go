package streamtui

import (
	"testing"
	"time"

	shared "plandex-shared"
)

func TestAdapter_InitialState(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	p := a.Progress()
	if p.ActivePhase != shared.PhaseConnect {
		t.Errorf("expected initial phase PhaseConnect, got %s", p.ActivePhase)
	}
	if len(p.Steps) != 1 {
		t.Errorf("expected 1 step (connect), got %d", len(p.Steps))
	}
	if p.Steps[0].Status != shared.StepRunning {
		t.Errorf("expected connect step to be running, got %s", p.Steps[0].Status)
	}
}

func TestAdapter_OnStart_CompletesConnect(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	p := a.Progress()
	if p.Steps[0].Status != shared.StepCompleted {
		t.Errorf("expected connect step completed after start, got %s", p.Steps[0].Status)
	}
	if p.Steps[0].Confidence != shared.ConfidenceGuaranteed {
		t.Error("expected connect step to have Guaranteed confidence after server confirmation")
	}
}

func TestAdapter_OnConnectActive_ResumeScenario(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageConnectActive,
		InitPrompt: "continue work",
	})

	p := a.Progress()
	if p.Steps[0].Status != shared.StepCompleted {
		t.Errorf("expected connect completed on reconnect, got %s", p.Steps[0].Status)
	}
	if p.Steps[0].Detail != "reconnected to active plan" {
		t.Errorf("expected reconnect detail, got %q", p.Steps[0].Detail)
	}
}

func TestAdapter_OnLoadContext_CreatesContextStep(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:             shared.StreamMessageLoadContext,
		LoadContextFiles: []string{"auth.go", "main.go"},
	})

	p := a.Progress()

	// Should have connect (completed) + context (running)
	if len(p.Steps) < 2 {
		t.Fatalf("expected at least 2 steps, got %d", len(p.Steps))
	}

	contextStep := p.Steps[1]
	if contextStep.Phase != shared.PhaseContext {
		t.Errorf("expected context phase, got %s", contextStep.Phase)
	}
	if contextStep.Status != shared.StepRunning {
		t.Errorf("expected context step running, got %s", contextStep.Status)
	}
	if contextStep.Detail != "2 file(s)" {
		t.Errorf("expected detail '2 file(s)', got %q", contextStep.Detail)
	}
}

func TestAdapter_OnDescribing_StartsModelPhase(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageDescribing})

	p := a.Progress()
	if p.ActivePhase != shared.PhaseModel {
		t.Errorf("expected PhaseModel active after describing, got %s", p.ActivePhase)
	}

	// Find the model step
	var modelStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel {
			modelStep = s
			break
		}
	}
	if modelStep == nil {
		t.Fatal("expected a model step to exist")
	}
	if modelStep.Status != shared.StepRunning {
		t.Errorf("expected model step running, got %s", modelStep.Status)
	}
	if modelStep.Detail != "generating description" {
		t.Errorf("expected detail 'generating description', got %q", modelStep.Detail)
	}
}

func TestAdapter_OnReply_TransitionsToModelPhase(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "Here's the plan...",
	})

	p := a.Progress()
	if p.ActivePhase != shared.PhaseModel {
		t.Errorf("expected PhaseModel active after reply, got %s", p.ActivePhase)
	}

	var modelStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel {
			modelStep = s
			break
		}
	}
	if modelStep == nil {
		t.Fatal("expected model step")
	}
	if modelStep.Detail != "streaming reply" {
		t.Errorf("expected 'streaming reply' detail, got %q", modelStep.Detail)
	}
}

func TestAdapter_OnReply_EmptyChunkIgnored(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	stepCountBefore := len(a.Progress().Steps)
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "",
	})

	if len(a.Progress().Steps) != stepCountBefore {
		t.Error("empty reply chunk should not create new steps")
	}
}

func TestAdapter_OnRepliesFinished_CompletesModelStep(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "content",
	})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageRepliesFinished})

	p := a.Progress()
	var modelStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel {
			modelStep = s
			break
		}
	}
	if modelStep == nil {
		t.Fatal("expected model step")
	}
	if modelStep.Status != shared.StepCompleted {
		t.Errorf("expected model step completed after replies finished, got %s", modelStep.Status)
	}
	if modelStep.Confidence != shared.ConfidenceGuaranteed {
		t.Error("expected Guaranteed confidence after server confirms replies finished")
	}
}

func TestAdapter_OnBuildInfo_CreatesBuildSteps(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	// First build message for a file
	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:      "src/main.go",
			NumTokens: 150,
		},
	})

	p := a.Progress()
	if p.ActivePhase != shared.PhaseBuild {
		t.Errorf("expected PhaseBuild active, got %s", p.ActivePhase)
	}

	// Find the build step
	var buildStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseBuild && s.Detail != "" {
			buildStep = s
			break
		}
	}
	if buildStep == nil {
		t.Fatal("expected a build step for src/main.go")
	}
	if buildStep.Status != shared.StepRunning {
		t.Errorf("expected build step running, got %s", buildStep.Status)
	}
}

func TestAdapter_OnBuildInfo_CompletesOnFinished(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:      "src/main.go",
			NumTokens: 100,
		},
	})

	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:     "src/main.go",
			Finished: true,
		},
	})

	p := a.Progress()
	var buildStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseBuild {
			buildStep = s
			break
		}
	}
	if buildStep == nil {
		t.Fatal("expected build step")
	}
	if buildStep.Status != shared.StepCompleted {
		t.Errorf("expected build step completed, got %s", buildStep.Status)
	}
}

func TestAdapter_OnBuildInfo_HandlesRemoval(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:    "old_file.go",
			Removed: true,
		},
	})

	p := a.Progress()
	var buildStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseBuild {
			buildStep = s
			break
		}
	}
	if buildStep == nil {
		t.Fatal("expected build step for removal")
	}
	if !containsStr(buildStep.Detail, "removed") {
		t.Errorf("expected detail to indicate removal, got %q", buildStep.Detail)
	}
	if buildStep.Status != shared.StepCompleted {
		t.Errorf("expected completed after removal, got %s", buildStep.Status)
	}
}

func TestAdapter_OnBuildInfo_MultipleConcurrentFiles(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	files := []string{"a.go", "b.go", "c.go"}
	for _, f := range files {
		a.OnMessage(&shared.StreamMessage{
			Type: shared.StreamMessageBuildInfo,
			BuildInfo: &shared.BuildInfo{
				Path:      f,
				NumTokens: 50,
			},
		})
	}

	p := a.Progress()
	buildCount := 0
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseBuild {
			buildCount++
		}
	}
	if buildCount != 3 {
		t.Errorf("expected 3 concurrent build steps, got %d", buildCount)
	}
}

func TestAdapter_OnFinished_ClosesAllSteps(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "text",
	})
	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:      "main.go",
			NumTokens: 100,
		},
	})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageFinished})

	p := a.Progress()
	if !p.Finished {
		t.Error("expected Finished=true after finished message")
	}
	for _, s := range p.Steps {
		if s.Status == shared.StepRunning || s.Status == shared.StepStalled {
			t.Errorf("step %s should not be running after finish, status=%s", s.Label, s.Status)
		}
	}
}

func TestAdapter_OnAborted_SetsErrorMessage(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageAborted})

	p := a.Progress()
	if !p.Finished {
		t.Error("expected Finished=true after abort")
	}
	if p.Error != "stopped by user" {
		t.Errorf("expected 'stopped by user' error, got %q", p.Error)
	}
}

func TestAdapter_OnError_FailsRunningSteps(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "partial",
	})
	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageError,
		Error: &shared.ApiError{
			Msg: "model API rate limit exceeded",
		},
	})

	p := a.Progress()
	if !p.Finished {
		t.Error("expected Finished=true after error")
	}

	hasFailedStep := false
	for _, s := range p.Steps {
		if s.Status == shared.StepFailed {
			hasFailedStep = true
			if s.Error != "model API rate limit exceeded" {
				t.Errorf("expected error message on failed step, got %q", s.Error)
			}
		}
	}
	if !hasFailedStep {
		t.Error("expected at least one failed step after error message")
	}
}

func TestAdapter_OnMulti_ProcessesAllSubmessages(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageMulti,
		StreamMessages: []shared.StreamMessage{
			{Type: shared.StreamMessageStart},
			{Type: shared.StreamMessageReply, ReplyChunk: "chunk1"},
			{Type: shared.StreamMessageReply, ReplyChunk: "chunk2"},
		},
	})

	p := a.Progress()
	if p.Steps[0].Status != shared.StepCompleted {
		t.Error("connect step should be completed after multi with start")
	}

	hasModel := false
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel {
			hasModel = true
		}
	}
	if !hasModel {
		t.Error("expected model step after multi with reply chunks")
	}
}

func TestAdapter_HeartbeatResetsStallTimer(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "working",
	})

	// Simulate heartbeat
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageHeartbeat})

	p := a.Progress()
	if p.LastHeartbeat == nil {
		t.Error("expected heartbeat timestamp to be recorded")
	}

	// The model step should still be running (not stalled) because
	// we just received a heartbeat.
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel && s.Status == shared.StepStalled {
			t.Error("step should not be stalled immediately after heartbeat")
		}
	}
}

func TestAdapter_StallDetection(t *testing.T) {
	// Override timeout for test speed
	origTimeout := HeartbeatTimeout
	// We can't easily change the const, but we can test the MarkStalled
	// path directly on the progress model.
	_ = origTimeout

	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "text",
	})

	// Manually trigger stall on model phase (simulating what the timer does)
	a.Progress().MarkStalled(shared.PhaseModel)

	p := a.Progress()
	stalledCount := 0
	for _, s := range p.Steps {
		if s.Status == shared.StepStalled {
			stalledCount++
		}
	}
	if stalledCount == 0 {
		t.Error("expected at least one stalled step after MarkStalled")
	}
}

func TestAdapter_FullLifecycle(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	// Full happy-path sequence
	messages := []shared.StreamMessage{
		{Type: shared.StreamMessageStart},
		{Type: shared.StreamMessageLoadContext, LoadContextFiles: []string{"auth.go"}},
		{Type: shared.StreamMessageDescribing},
		{Type: shared.StreamMessageReply, ReplyChunk: "I'll update auth.go to..."},
		{Type: shared.StreamMessageRepliesFinished},
		{Type: shared.StreamMessageBuildInfo, BuildInfo: &shared.BuildInfo{Path: "auth.go", NumTokens: 200}},
		{Type: shared.StreamMessageBuildInfo, BuildInfo: &shared.BuildInfo{Path: "auth.go", Finished: true}},
		{Type: shared.StreamMessageFinished},
	}

	for _, msg := range messages {
		a.OnMessage(&msg)
	}

	p := a.Progress()

	// Verify final state
	if !p.Finished {
		t.Error("expected Finished=true")
	}
	if p.Error != "" {
		t.Errorf("expected no error, got %q", p.Error)
	}

	// All steps should be in terminal state
	for _, s := range p.Steps {
		if !s.Status.IsTerminal() {
			t.Errorf("step %s (%s) should be terminal, status=%s", s.Label, s.Phase, s.Status)
		}
	}

	// Should have steps for each phase that was exercised
	phases := make(map[shared.PhaseID]bool)
	for _, s := range p.Steps {
		phases[s.Phase] = true
	}
	for _, expected := range []shared.PhaseID{shared.PhaseConnect, shared.PhaseContext, shared.PhaseModel, shared.PhaseBuild, shared.PhaseFinalize} {
		if !phases[expected] {
			t.Errorf("expected phase %s to have at least one step", expected)
		}
	}
}

func TestAdapter_Shutdown_Idempotent(t *testing.T) {
	a := NewProgressAdapter()
	// Call Shutdown multiple times — should not panic
	a.Shutdown()
	a.Shutdown()
	a.Shutdown()
}

func TestAdapter_NilBuildInfo_Ignored(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	stepsBefore := len(a.Progress().Steps)
	a.OnMessage(&shared.StreamMessage{
		Type:      shared.StreamMessageBuildInfo,
		BuildInfo: nil,
	})

	if len(a.Progress().Steps) != stepsBefore {
		t.Error("nil BuildInfo should not create any steps")
	}
}

// --- Nil / missing field safety ------------------------------------------------

func TestAdapter_NilMessage_NoPanic(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	// Nil message must be a no-op — no panic
	a.OnMessage(nil)

	p := a.Progress()
	// State unchanged: still has the initial connect step
	if len(p.Steps) != 1 {
		t.Errorf("expected 1 step after nil message, got %d", len(p.Steps))
	}
}

func TestAdapter_Error_NilErrorField(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:       shared.StreamMessageReply,
		ReplyChunk: "partial",
	})

	// Error message with nil Error field → falls back to "unknown error"
	a.OnMessage(&shared.StreamMessage{
		Type:  shared.StreamMessageError,
		Error: nil,
	})

	p := a.Progress()
	if !p.Finished {
		t.Error("expected Finished=true after error with nil Error field")
	}
	if p.Error != "unknown error" {
		t.Errorf("expected 'unknown error' fallback, got %q", p.Error)
	}
}

// --- Idempotency / double-call safety ------------------------------------------

func TestAdapter_DoubleStart_Idempotent(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	p := a.Progress()
	// Connect step still completed, no panic, no extra step
	if p.Steps[0].Status != shared.StepCompleted {
		t.Errorf("expected connect completed after double start, got %s", p.Steps[0].Status)
	}
	// Should not have spawned a second connect step
	connectCount := 0
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseConnect {
			connectCount++
		}
	}
	if connectCount != 1 {
		t.Errorf("expected exactly 1 connect step after double start, got %d", connectCount)
	}
}

// --- RepliesFinished without prior model step ---------------------------------

func TestAdapter_RepliesFinished_NoModelStep(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	// Send RepliesFinished without ever sending a Reply — model step is nil
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageRepliesFinished})

	p := a.Progress()
	// Must not panic; no model step exists so nothing to complete
	hasModel := false
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel {
			hasModel = true
		}
	}
	if hasModel {
		t.Error("expected no model step when RepliesFinished arrives without prior Reply")
	}
}

// --- BuildInfo token count updates ---------------------------------------------

func TestAdapter_BuildInfo_TokenCountUpdates(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:      "file.go",
			NumTokens: 50,
		},
	})
	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:      "file.go",
			NumTokens: 200,
		},
	})

	p := a.Progress()
	var buildStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseBuild {
			buildStep = s
			break
		}
	}
	if buildStep == nil {
		t.Fatal("expected build step")
	}
	// Detail should reflect the latest token count
	if !containsStr(buildStep.Detail, "200") {
		t.Errorf("expected detail to contain updated token count 200, got %q", buildStep.Detail)
	}
}

func TestAdapter_BuildInfo_ZeroTokens(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:      "empty.go",
			NumTokens: 0,
		},
	})

	p := a.Progress()
	var buildStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseBuild {
			buildStep = s
			break
		}
	}
	if buildStep == nil {
		t.Fatal("expected build step for zero-token file")
	}
	// Zero tokens → detail should just be the path, no token suffix
	if buildStep.Detail != "empty.go" {
		t.Errorf("expected detail='empty.go' for zero tokens, got %q", buildStep.Detail)
	}
}

// --- Finished / Aborted with no running steps ---------------------------------

func TestAdapter_Finished_NoRunningSteps(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	// Complete the connect step first
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	// Now send Finished — no other steps are running
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageFinished})

	p := a.Progress()
	if !p.Finished {
		t.Error("expected Finished=true")
	}
	if p.Error != "" {
		t.Errorf("expected no error on clean finish, got %q", p.Error)
	}
	// Finalize step should have been created and completed
	hasFinalize := false
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseFinalize && s.Status == shared.StepCompleted {
			hasFinalize = true
		}
	}
	if !hasFinalize {
		t.Error("expected a completed finalize step")
	}
}

func TestAdapter_Aborted_Immediately(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	// Abort immediately — only connect step exists (running)
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageAborted})

	p := a.Progress()
	if !p.Finished {
		t.Error("expected Finished=true after immediate abort")
	}
	if p.Error != "stopped by user" {
		t.Errorf("expected 'stopped by user', got %q", p.Error)
	}
	// Connect step should be completed (closeAllRunning)
	if p.Steps[0].Status != shared.StepCompleted {
		t.Errorf("expected connect step completed on abort, got %s", p.Steps[0].Status)
	}
}

// --- LoadContext with empty file list ------------------------------------------

func TestAdapter_LoadContext_EmptyFileList(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{
		Type:             shared.StreamMessageLoadContext,
		LoadContextFiles: []string{},
	})

	p := a.Progress()
	// Context step should exist but detail should NOT say "0 file(s)"
	var ctxStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseContext {
			ctxStep = s
			break
		}
	}
	if ctxStep == nil {
		t.Fatal("expected context step for empty LoadContext")
	}
	// Empty list means len == 0, so the "N file(s)" branch is skipped
	if ctxStep.Detail == "0 file(s)" {
		t.Errorf("expected detail to NOT say '0 file(s)' for empty list, got %q", ctxStep.Detail)
	}
}

// --- Reply re-entry after RepliesFinished ---------------------------------------

func TestAdapter_Reply_AfterRepliesFinished_ReEntry(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageReply, ReplyChunk: "first"})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageRepliesFinished})

	// Model step is now completed. Re-enter with another Reply chunk
	// (simulates missing-file prompt scenario)
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageReply, ReplyChunk: "second"})

	p := a.Progress()
	var modelStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel {
			modelStep = s
			break
		}
	}
	if modelStep == nil {
		t.Fatal("expected model step after re-entry")
	}
	if modelStep.Status != shared.StepRunning {
		t.Errorf("expected model step re-entered as running, got %s", modelStep.Status)
	}
	if !containsStr(modelStep.Detail, "continued") {
		t.Errorf("expected 'continued' in detail for re-entry, got %q", modelStep.Detail)
	}
	// DurationMs should have been reset
	if modelStep.DurationMs != 0 {
		t.Errorf("expected DurationMs=0 on re-entry, got %d", modelStep.DurationMs)
	}
}

// --- Multiple heartbeats -------------------------------------------------------

func TestAdapter_MultipleHeartbeats(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	// Record first heartbeat
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageHeartbeat})
	first := *a.Progress().LastHeartbeat

	time.Sleep(1 * time.Millisecond)

	// Record second heartbeat
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageHeartbeat})
	second := *a.Progress().LastHeartbeat

	if !second.After(first) {
		t.Error("expected second heartbeat timestamp to be after first")
	}
}

// --- Message after Shutdown ----------------------------------------------------

func TestAdapter_MessageAfterShutdown_NoPanic(t *testing.T) {
	a := NewProgressAdapter()
	a.Shutdown()

	// Sending a message after shutdown must not panic
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageHeartbeat})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	// Progress is still readable
	p := a.Progress()
	if p == nil {
		t.Error("expected non-nil progress after shutdown")
	}
}

// --- BuildInfo after Finished --------------------------------------------------

func TestAdapter_BuildInfo_AfterFinished_NoPanic(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageFinished})

	// BuildInfo after finished — must not panic
	a.OnMessage(&shared.StreamMessage{
		Type: shared.StreamMessageBuildInfo,
		BuildInfo: &shared.BuildInfo{
			Path:      "late.go",
			NumTokens: 10,
		},
	})

	p := a.Progress()
	// The step was added (Progress is a simple data holder)
	lateExists := false
	for _, s := range p.Steps {
		if s.Detail != "" && containsStr(s.Detail, "late.go") {
			lateExists = true
		}
	}
	if !lateExists {
		t.Error("expected late build step to be recorded even after Finished")
	}
}

// --- Concurrent Progress reads -------------------------------------------------

func TestAdapter_ConcurrentProgressReads(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})

	// Launch goroutines that read Progress concurrently while messages arrive
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				p := a.Progress()
				_ = p.ActivePhase
				_ = len(p.Steps)
			}
		}()
	}

	// Meanwhile send messages
	for i := 0; i < 20; i++ {
		a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageHeartbeat})
	}

	// Wait for all readers
	for i := 0; i < 10; i++ {
		<-done
	}
	// If we get here without a race detector hit, the test passes
}

// --- Describing after model already completed ----------------------------------

func TestAdapter_Describing_AfterModelCompleted_UpdatesDetail(t *testing.T) {
	a := NewProgressAdapter()
	defer a.Shutdown()

	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageStart})
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageDescribing})

	// Complete the model step
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageRepliesFinished})

	// Send another Describing — model step already exists and is completed
	a.OnMessage(&shared.StreamMessage{Type: shared.StreamMessageDescribing})

	p := a.Progress()
	var modelStep *shared.Step
	for _, s := range p.Steps {
		if s.Phase == shared.PhaseModel {
			modelStep = s
			break
		}
	}
	if modelStep == nil {
		t.Fatal("expected model step")
	}
	// Detail should be updated to "generating description"
	if modelStep.Detail != "generating description" {
		t.Errorf("expected 'generating description' after re-describing, got %q", modelStep.Detail)
	}
}

// helper
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ensure time import is used
var _ = time.Now
