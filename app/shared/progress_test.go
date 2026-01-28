package shared

import (
	"testing"
	"time"
)

func TestNewProgress_InitialState(t *testing.T) {
	p := NewProgress()

	if p.ActivePhase != PhaseConnect {
		t.Errorf("expected initial phase PhaseConnect, got %s", p.ActivePhase)
	}
	if len(p.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(p.Steps))
	}
	if p.Finished {
		t.Error("expected Finished=false initially")
	}
	if p.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestAddStep_AppendsWithCorrectDefaults(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseModel, "model request", "gpt-4o")

	if s.Phase != PhaseModel {
		t.Errorf("expected phase PhaseModel, got %s", s.Phase)
	}
	if s.Label != "model request" {
		t.Errorf("expected label 'model request', got %s", s.Label)
	}
	if s.Detail != "gpt-4o" {
		t.Errorf("expected detail 'gpt-4o', got %s", s.Detail)
	}
	if s.Status != StepPending {
		t.Errorf("expected status StepPending, got %s", s.Status)
	}
	if s.Confidence != ConfidenceBestEffort {
		t.Errorf("expected confidence BestEffort, got %d", s.Confidence)
	}
	if len(p.Steps) != 1 {
		t.Errorf("expected 1 step in progress, got %d", len(p.Steps))
	}
}

func TestStartStep_SetsRunningAndTimestamp(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseContext, "loading", "auth.go")

	before := time.Now()
	p.StartStep(s)
	after := time.Now()

	if s.Status != StepRunning {
		t.Errorf("expected StepRunning after StartStep, got %s", s.Status)
	}
	if s.Confidence != ConfidenceBestEffort {
		t.Errorf("expected BestEffort confidence while running, got %d", s.Confidence)
	}
	if s.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if s.StartedAt.Before(before) || s.StartedAt.After(after) {
		t.Error("StartedAt is outside the expected time window")
	}
	if p.ActivePhase != PhaseContext {
		t.Errorf("expected ActivePhase=PhaseContext, got %s", p.ActivePhase)
	}
}

func TestCompleteStep_GuaranteedState(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseBuild, "build file", "main.go")
	p.StartStep(s)

	p.CompleteStep(s)

	if s.Status != StepCompleted {
		t.Errorf("expected StepCompleted, got %s", s.Status)
	}
	if s.Confidence != ConfidenceGuaranteed {
		t.Errorf("expected Guaranteed confidence after completion, got %d", s.Confidence)
	}
	if s.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be set")
	}
	if s.DurationMs <= 0 {
		// DurationMs may be 0 if Start and Complete happen in the same nanosecond;
		// just ensure it's not negative.
		if s.DurationMs < 0 {
			t.Errorf("expected non-negative DurationMs, got %d", s.DurationMs)
		}
	}
}

func TestFailStep_GuaranteedFailure(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseModel, "model call", "")
	p.StartStep(s)

	p.FailStep(s, "API timeout after 30s")

	if s.Status != StepFailed {
		t.Errorf("expected StepFailed, got %s", s.Status)
	}
	if s.Confidence != ConfidenceGuaranteed {
		t.Errorf("expected Guaranteed confidence after failure, got %d", s.Confidence)
	}
	if s.Error != "API timeout after 30s" {
		t.Errorf("expected error message preserved, got %s", s.Error)
	}
	if p.Error != "API timeout after 30s" {
		t.Errorf("expected progress-level error set, got %s", p.Error)
	}
}

func TestSkipStep_MarkedWithReason(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseApply, "run tests", "original detail")

	p.SkipStep(s, "user requested skip")

	if s.Status != StepSkipped {
		t.Errorf("expected StepSkipped, got %s", s.Status)
	}
	if s.Confidence != ConfidenceGuaranteed {
		t.Errorf("expected Guaranteed confidence after skip, got %d", s.Confidence)
	}
	// Reason goes into Error so that the pre-existing Detail is preserved.
	if s.Error != "user requested skip" {
		t.Errorf("expected Error to hold skip reason, got %s", s.Error)
	}
	if s.Detail != "original detail" {
		t.Errorf("expected Detail preserved after skip, got %s", s.Detail)
	}
}

func TestMarkStalled_OnlyAffectsRunning(t *testing.T) {
	p := NewProgress()

	completed := p.AddStep(PhaseConnect, "connect", "")
	p.StartStep(completed)
	p.CompleteStep(completed)

	running := p.AddStep(PhaseModel, "model", "")
	p.StartStep(running)

	pending := p.AddStep(PhaseBuild, "build", "")

	p.MarkStalled(PhaseModel)

	if completed.Status != StepCompleted {
		t.Error("completed step should not be affected by MarkStalled")
	}
	if running.Status != StepStalled {
		t.Errorf("running step should be stalled, got %s", running.Status)
	}
	if pending.Status != StepPending {
		t.Error("pending step should not be affected by MarkStalled")
	}
}

func TestRecordHeartbeat_UpdatesTimestamp(t *testing.T) {
	p := NewProgress()

	if p.LastHeartbeat != nil {
		t.Error("expected no heartbeat initially")
	}

	before := time.Now()
	p.RecordHeartbeat()
	after := time.Now()

	if p.LastHeartbeat == nil {
		t.Fatal("expected LastHeartbeat to be set")
	}
	if p.LastHeartbeat.Before(before) || p.LastHeartbeat.After(after) {
		t.Error("LastHeartbeat outside expected window")
	}
}

func TestActiveSteps_ReturnsRunningAndStalled(t *testing.T) {
	p := NewProgress()

	s1 := p.AddStep(PhaseModel, "a", "")
	p.StartStep(s1)

	s2 := p.AddStep(PhaseModel, "b", "")
	p.StartStep(s2)
	s2.Status = StepStalled // simulate stall

	s3 := p.AddStep(PhaseBuild, "c", "")
	p.StartStep(s3)
	p.CompleteStep(s3)

	active := p.ActiveSteps()
	if len(active) != 2 {
		t.Errorf("expected 2 active steps, got %d", len(active))
	}
}

func TestPhaseSteps_FiltersCorrectly(t *testing.T) {
	p := NewProgress()
	p.AddStep(PhaseConnect, "c1", "")
	p.AddStep(PhaseModel, "m1", "")
	p.AddStep(PhaseModel, "m2", "")
	p.AddStep(PhaseBuild, "b1", "")

	modelSteps := p.PhaseSteps(PhaseModel)
	if len(modelSteps) != 2 {
		t.Errorf("expected 2 model steps, got %d", len(modelSteps))
	}
	for _, s := range modelSteps {
		if s.Phase != PhaseModel {
			t.Errorf("expected all returned steps to be PhaseModel, got %s", s.Phase)
		}
	}
}

func TestSummary_FormatsCorrectly(t *testing.T) {
	p := NewProgress()

	s1 := p.AddStep(PhaseConnect, "connect", "")
	p.StartStep(s1)
	p.CompleteStep(s1)

	s2 := p.AddStep(PhaseModel, "model", "")
	p.StartStep(s2)

	summary := p.Summary()

	if summary == "" {
		t.Error("expected non-empty summary")
	}
	if len(summary) < 5 {
		t.Error("summary too short to be meaningful")
	}
}

func TestSummary_StalledReportedSeparately(t *testing.T) {
	p := NewProgress()

	s1 := p.AddStep(PhaseConnect, "connect", "")
	p.StartStep(s1)
	p.CompleteStep(s1)

	s2 := p.AddStep(PhaseModel, "model", "")
	p.StartStep(s2)
	s2.Status = StepStalled // simulate stall

	summary := p.Summary()

	// Stalled step must appear as "1 stalled", NOT "1 running"
	if !containsSubstring(summary, "1 stalled") {
		t.Errorf("expected '1 stalled' in summary, got: %s", summary)
	}
	if containsSubstring(summary, "running") {
		t.Errorf("expected no 'running' when step is stalled, got: %s", summary)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

func TestStepStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   StepStatus
		terminal bool
	}{
		{StepPending, false},
		{StepRunning, false},
		{StepCompleted, true},
		{StepFailed, true},
		{StepSkipped, true},
		{StepStalled, false},
	}

	for _, tt := range tests {
		if tt.status.IsTerminal() != tt.terminal {
			t.Errorf("IsTerminal(%s) = %v, want %v", tt.status, tt.status.IsTerminal(), tt.terminal)
		}
	}
}

func TestStepStatus_IsGuaranteed(t *testing.T) {
	tests := []struct {
		status     StepStatus
		guaranteed bool
	}{
		{StepPending, false},
		{StepRunning, false},
		{StepCompleted, true},
		{StepFailed, true},
		{StepSkipped, true},
		{StepStalled, false},
	}

	for _, tt := range tests {
		if tt.status.IsGuaranteed() != tt.guaranteed {
			t.Errorf("IsGuaranteed(%s) = %v, want %v", tt.status, tt.status.IsGuaranteed(), tt.guaranteed)
		}
	}
}

func TestStep_Duration_Running(t *testing.T) {
	s := &Step{}
	now := time.Now().Add(-2 * time.Second)
	s.StartedAt = &now
	s.Status = StepRunning

	d := s.Duration()
	if d < time.Second {
		t.Errorf("expected duration >= 1s for a step started 2s ago, got %v", d)
	}
}

func TestStep_Duration_Completed(t *testing.T) {
	start := time.Now().Add(-500 * time.Millisecond)
	end := time.Now()
	s := &Step{
		StartedAt:  &start,
		FinishedAt: &end,
		Status:     StepCompleted,
	}

	d := s.Duration()
	if d < 400*time.Millisecond || d > 600*time.Millisecond {
		t.Errorf("expected ~500ms duration, got %v", d)
	}
}

func TestFormatProgressDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "<1s"},
		{500 * time.Millisecond, "<1s"},
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{2*time.Minute + 30*time.Second, "2m30s"},
		{5 * time.Minute, "5m"},
		{1*time.Hour + 5*time.Minute, "1h5m"},
	}

	for _, tt := range tests {
		result := formatProgressDuration(tt.input)
		if result != tt.expected {
			t.Errorf("formatProgressDuration(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestPhaseID_PhaseLabel(t *testing.T) {
	tests := []struct {
		phase PhaseID
		label string
	}{
		{PhaseConnect, "connect"},
		{PhaseContext, "context"},
		{PhaseModel, "model"},
		{PhaseBuild, "build"},
		{PhaseApply, "apply"},
		{PhaseFinalize, "finalize"},
		{PhaseID("unknown"), "unknown"},
	}

	for _, tt := range tests {
		if tt.phase.PhaseLabel() != tt.label {
			t.Errorf("PhaseLabel(%s) = %q, want %q", tt.phase, tt.phase.PhaseLabel(), tt.label)
		}
	}
}

func TestMultipleStepsPerPhase(t *testing.T) {
	p := NewProgress()

	// Simulate multiple build steps (one per file)
	paths := []string{"main.go", "util.go", "auth.go"}
	steps := make([]*Step, len(paths))

	for i, path := range paths {
		steps[i] = p.AddStep(PhaseBuild, "build", path)
		p.StartStep(steps[i])
	}

	// Complete them out of order
	p.CompleteStep(steps[2])
	p.CompleteStep(steps[0])

	// steps[1] still running
	active := p.ActiveSteps()
	if len(active) != 1 {
		t.Errorf("expected 1 active step, got %d", len(active))
	}
	if active[0].Detail != "util.go" {
		t.Errorf("expected active step to be util.go, got %s", active[0].Detail)
	}

	completed := p.CompletedSteps()
	if len(completed) != 2 {
		t.Errorf("expected 2 completed steps, got %d", len(completed))
	}
}

// --- Idempotency & double-call safety ----------------------------------------

func TestCompleteStep_Idempotent(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseModel, "model", "")
	p.StartStep(s)
	p.CompleteStep(s)

	firstFinishedAt := *s.FinishedAt

	// Second complete must not panic and must not regress state
	p.CompleteStep(s)

	if s.Status != StepCompleted {
		t.Errorf("expected StepCompleted after double complete, got %s", s.Status)
	}
	if s.Confidence != ConfidenceGuaranteed {
		t.Error("expected Guaranteed confidence after double complete")
	}
	if s.FinishedAt.Before(firstFinishedAt) {
		t.Error("FinishedAt should not regress on re-complete")
	}
}

func TestFailStep_Idempotent(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseModel, "model", "")
	p.StartStep(s)
	p.FailStep(s, "first error")

	// Second fail with different message must not panic; last message wins
	p.FailStep(s, "second error")

	if s.Status != StepFailed {
		t.Errorf("expected StepFailed after double fail, got %s", s.Status)
	}
	if s.Error != "second error" {
		t.Errorf("expected last error message preserved, got %q", s.Error)
	}
}

func TestStartStep_AlreadyRunning(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseModel, "model", "")
	p.StartStep(s)
	firstStartedAt := *s.StartedAt

	// Re-start must not panic; StartedAt is refreshed
	p.StartStep(s)

	if s.Status != StepRunning {
		t.Errorf("expected StepRunning after re-start, got %s", s.Status)
	}
	if s.StartedAt.Before(firstStartedAt) {
		t.Error("StartedAt should not regress on re-start")
	}
}

// --- Empty / nil edge cases ---------------------------------------------------

func TestSummary_EmptyProgress(t *testing.T) {
	p := NewProgress()
	summary := p.Summary()

	if summary == "" {
		t.Error("expected non-empty summary for empty progress")
	}
	if !containsSubstring(summary, "starting") {
		t.Errorf("expected 'starting' in empty summary, got: %s", summary)
	}
}

func TestSummary_AllPending(t *testing.T) {
	p := NewProgress()
	p.AddStep(PhaseModel, "a", "")
	p.AddStep(PhaseBuild, "b", "")

	summary := p.Summary()
	if !containsSubstring(summary, "2 pending") {
		t.Errorf("expected '2 pending' in summary, got: %s", summary)
	}
}

func TestActiveSteps_EmptyProgress(t *testing.T) {
	p := NewProgress()
	active := p.ActiveSteps()
	if len(active) != 0 {
		t.Errorf("expected 0 active steps on empty progress, got %d", len(active))
	}
}

func TestCompletedSteps_EmptyProgress(t *testing.T) {
	p := NewProgress()
	completed := p.CompletedSteps()
	if len(completed) != 0 {
		t.Errorf("expected 0 completed steps on empty progress, got %d", len(completed))
	}
}

func TestFailedSteps_EmptyProgress(t *testing.T) {
	p := NewProgress()
	failed := p.FailedSteps()
	if len(failed) != 0 {
		t.Errorf("expected 0 failed steps on empty progress, got %d", len(failed))
	}
}

func TestPhaseSteps_NonexistentPhase(t *testing.T) {
	p := NewProgress()
	p.AddStep(PhaseModel, "m", "")

	steps := p.PhaseSteps(PhaseBuild)
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for non-existent phase, got %d", len(steps))
	}
}

// --- Duration edge cases -------------------------------------------------------

func TestStep_Duration_NilStartedAt(t *testing.T) {
	s := &Step{Status: StepPending}
	d := s.Duration()
	if d != 0 {
		t.Errorf("expected 0 duration for step with nil StartedAt, got %v", d)
	}
}

func TestStep_Duration_NilFinishedAt_Completed(t *testing.T) {
	// Status says completed but FinishedAt was never set — falls back to live elapsed
	now := time.Now().Add(-1 * time.Second)
	s := &Step{
		StartedAt: &now,
		Status:    StepCompleted,
		// FinishedAt intentionally nil
	}
	d := s.Duration()
	if d < 500*time.Millisecond {
		t.Errorf("expected live duration fallback for nil FinishedAt, got %v", d)
	}
}

// --- Unusual state transitions -------------------------------------------------

func TestSkipStep_FromPending(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseApply, "apply", "run tests")

	// Skip without ever starting
	p.SkipStep(s, "not needed")

	if s.Status != StepSkipped {
		t.Errorf("expected StepSkipped from pending, got %s", s.Status)
	}
	if s.Confidence != ConfidenceGuaranteed {
		t.Error("expected Guaranteed confidence after skip")
	}
	if s.Error != "not needed" {
		t.Errorf("expected skip reason in Error, got %q", s.Error)
	}
	if s.Detail != "run tests" {
		t.Errorf("expected Detail preserved, got %q", s.Detail)
	}
}

func TestFailStep_FromPending(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseModel, "model", "")

	// Fail without starting — StartedAt is nil
	p.FailStep(s, "pre-flight check failed")

	if s.Status != StepFailed {
		t.Errorf("expected StepFailed from pending, got %s", s.Status)
	}
	if s.Error != "pre-flight check failed" {
		t.Errorf("expected error message, got %q", s.Error)
	}
	// DurationMs must be 0 because StartedAt was nil
	if s.DurationMs != 0 {
		t.Errorf("expected DurationMs=0 when StartedAt is nil, got %d", s.DurationMs)
	}
}

func TestCompleteStep_FromPending(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseConnect, "connect", "")

	// Complete without starting — StartedAt is nil
	p.CompleteStep(s)

	if s.Status != StepCompleted {
		t.Errorf("expected StepCompleted from pending, got %s", s.Status)
	}
	if s.DurationMs != 0 {
		t.Errorf("expected DurationMs=0 when StartedAt is nil, got %d", s.DurationMs)
	}
}

// --- MarkStalled edge cases ----------------------------------------------------

func TestMarkStalled_NoStepsInPhase(t *testing.T) {
	p := NewProgress()
	p.AddStep(PhaseModel, "model", "")

	// Stall a phase that has no steps — must be a no-op
	p.MarkStalled(PhaseBuild)

	if p.Steps[0].Status != StepPending {
		t.Errorf("expected model step still pending, got %s", p.Steps[0].Status)
	}
}

func TestMarkStalled_OnlyCompletedInPhase(t *testing.T) {
	p := NewProgress()
	s := p.AddStep(PhaseModel, "model", "")
	p.StartStep(s)
	p.CompleteStep(s)

	// Stall model phase — only step is already completed
	p.MarkStalled(PhaseModel)

	if s.Status != StepCompleted {
		t.Errorf("expected completed step unaffected by MarkStalled, got %s", s.Status)
	}
}

// --- Multiple errors — last wins -----------------------------------------------

func TestFailStep_MultipleErrors_LastWins(t *testing.T) {
	p := NewProgress()

	s1 := p.AddStep(PhaseModel, "step1", "")
	p.StartStep(s1)
	p.FailStep(s1, "first error")

	s2 := p.AddStep(PhaseBuild, "step2", "")
	p.StartStep(s2)
	p.FailStep(s2, "second error")

	// Progress.Error reflects the last FailStep call
	if p.Error != "second error" {
		t.Errorf("expected Progress.Error='second error', got %q", p.Error)
	}
}

// --- AddStep after finished ----------------------------------------------------

func TestAddStep_AfterFinished(t *testing.T) {
	p := NewProgress()
	p.Finished = true

	// Must not panic — Progress is a simple data holder
	s := p.AddStep(PhaseFinalize, "cleanup", "")
	if s == nil {
		t.Fatal("AddStep returned nil after Finished=true")
	}
	if len(p.Steps) != 1 {
		t.Errorf("expected 1 step after AddStep on finished progress, got %d", len(p.Steps))
	}
}

// --- RecordHeartbeat ordering --------------------------------------------------

func TestRecordHeartbeat_MultipleOrdering(t *testing.T) {
	p := NewProgress()
	p.RecordHeartbeat()
	first := *p.LastHeartbeat

	time.Sleep(1 * time.Millisecond)
	p.RecordHeartbeat()
	second := *p.LastHeartbeat

	if !second.After(first) {
		t.Error("expected second heartbeat to be after first")
	}
}
