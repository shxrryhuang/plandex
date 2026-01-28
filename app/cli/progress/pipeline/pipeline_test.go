package pipeline

import (
	"bytes"
	"sync"
	"testing"
	"time"

	shared "plandex-shared"
)

func TestPipelineBasic(t *testing.T) {
	config := DefaultConfig()
	p := New(config)

	p.Start()
	defer p.Stop()

	// Test phase setting
	p.SetPhase(shared.PhasePlanning, "Planning")
	report := p.GetReport()
	if report.Phase != shared.PhasePlanning {
		t.Errorf("expected phase %s, got %s", shared.PhasePlanning, report.Phase)
	}

	// Test step creation
	stepID := p.StartStep(shared.StepKindLLMCall, "Test LLM", "model-1")
	step, ok := p.GetStep(stepID)
	if !ok {
		t.Error("step not found")
	}
	if step.State != shared.StepStateRunning {
		t.Errorf("expected state %s, got %s", shared.StepStateRunning, step.State)
	}

	// Test step update
	p.UpdateStep(stepID, StepUpdates{Tokens: 100})
	step, _ = p.GetStep(stepID)
	if step.TokensProcessed != 100 {
		t.Errorf("expected 100 tokens, got %d", step.TokensProcessed)
	}

	// Test token accumulation
	p.UpdateStep(stepID, StepUpdates{Tokens: 50})
	step, _ = p.GetStep(stepID)
	if step.TokensProcessed != 150 {
		t.Errorf("expected 150 tokens (accumulated), got %d", step.TokensProcessed)
	}

	// Test completion
	p.CompleteStep(stepID)
	step, _ = p.GetStep(stepID)
	if step.State != shared.StepStateCompleted {
		t.Errorf("expected state %s, got %s", shared.StepStateCompleted, step.State)
	}
}

func TestPipelineFailure(t *testing.T) {
	config := DefaultConfig()
	p := New(config)

	p.Start()
	defer p.Stop()

	stepID := p.StartStep(shared.StepKindFileBuild, "Build", "test.go")
	p.FailStep(stepID, "syntax error")

	step, _ := p.GetStep(stepID)
	if step.State != shared.StepStateFailed {
		t.Errorf("expected state %s, got %s", shared.StepStateFailed, step.State)
	}
	if step.Error != "syntax error" {
		t.Errorf("expected error 'syntax error', got '%s'", step.Error)
	}
}

func TestPipelineSkip(t *testing.T) {
	config := DefaultConfig()
	p := New(config)

	p.Start()
	defer p.Stop()

	stepID := p.StartStep(shared.StepKindFileBuild, "Build", "deprecated.go")
	p.SkipStep(stepID)

	step, _ := p.GetStep(stepID)
	if step.State != shared.StepStateSkipped {
		t.Errorf("expected state %s, got %s", shared.StepStateSkipped, step.State)
	}
}

func TestPipelineWait(t *testing.T) {
	config := DefaultConfig()
	p := New(config)

	p.Start()
	defer p.Stop()

	stepID := p.StartStep(shared.StepKindUserInput, "Input", "config.json")
	p.WaitStep(stepID, "Waiting for file")

	step, _ := p.GetStep(stepID)
	if step.State != shared.StepStateWaiting {
		t.Errorf("expected state %s, got %s", shared.StepStateWaiting, step.State)
	}
}

func TestPipelineCallbacks(t *testing.T) {
	var phaseChanges []shared.ProgressPhase
	var stepStarts []string
	var stepEnds []string
	var mu sync.Mutex

	config := DefaultConfig()
	config.OnPhaseChange = func(phase shared.ProgressPhase, label string) {
		mu.Lock()
		phaseChanges = append(phaseChanges, phase)
		mu.Unlock()
	}
	config.OnStepStart = func(step *shared.Step) {
		mu.Lock()
		stepStarts = append(stepStarts, step.ID)
		mu.Unlock()
	}
	config.OnStepEnd = func(step *shared.Step) {
		mu.Lock()
		stepEnds = append(stepEnds, step.ID)
		mu.Unlock()
	}

	p := New(config)
	p.Start()
	defer p.Stop()

	p.SetPhase(shared.PhasePlanning, "Planning")
	stepID := p.StartStep(shared.StepKindLLMCall, "LLM", "test")
	p.CompleteStep(stepID)

	mu.Lock()
	defer mu.Unlock()

	if len(phaseChanges) != 1 || phaseChanges[0] != shared.PhasePlanning {
		t.Errorf("phase change callback not called correctly")
	}
	if len(stepStarts) != 1 {
		t.Errorf("step start callback not called")
	}
	if len(stepEnds) != 1 {
		t.Errorf("step end callback not called")
	}
}

func TestMockStreamNormal(t *testing.T) {
	config := DefaultConfig()
	p := New(config)
	p.Start()
	defer p.Stop()

	mockConfig := DefaultMockConfig()
	mockConfig.Scenario = ScenarioQuickTask
	mockConfig.TimeScale = 0.01 // Very fast for testing

	mock := NewMockStream(p, mockConfig)
	err := mock.Run()
	if err != nil {
		t.Errorf("mock stream failed: %v", err)
	}

	report := p.GetReport()
	if report.Phase != shared.PhaseCompleted {
		t.Errorf("expected phase %s, got %s", shared.PhaseCompleted, report.Phase)
	}

	// Check we have some completed steps
	completedCount := 0
	for _, s := range report.Steps {
		if s.State == shared.StepStateCompleted {
			completedCount++
		}
	}
	if completedCount == 0 {
		t.Error("expected some completed steps")
	}
}

func TestMockStreamFailure(t *testing.T) {
	config := DefaultConfig()
	p := New(config)
	p.Start()
	defer p.Stop()

	mockConfig := DefaultMockConfig()
	mockConfig.Scenario = ScenarioFailure
	mockConfig.TimeScale = 0.01

	mock := NewMockStream(p, mockConfig)
	err := mock.Run()
	if err != nil {
		t.Errorf("mock stream failed unexpectedly: %v", err)
	}

	report := p.GetReport()
	if report.Phase != shared.PhaseFailed {
		t.Errorf("expected phase %s, got %s", shared.PhaseFailed, report.Phase)
	}

	// Check we have a failed step
	failedCount := 0
	for _, s := range report.Steps {
		if s.State == shared.StepStateFailed {
			failedCount++
		}
	}
	if failedCount == 0 {
		t.Error("expected a failed step")
	}
}

func TestRunnerOutput(t *testing.T) {
	var buf bytes.Buffer

	config := DefaultConfig()
	p := New(config)

	runnerConfig := RunnerConfig{
		Output: &buf,
		IsTTY:  false, // Use log format for easier testing
		Width:  80,
	}

	runner := NewRunner(p, runnerConfig)

	mockConfig := DefaultMockConfig()
	mockConfig.Scenario = ScenarioQuickTask
	mockConfig.TimeScale = 0.01

	p.config.OnPhaseChange = runner.onPhaseChange
	p.config.OnStepStart = runner.onStepStart
	p.config.OnStepEnd = runner.onStepEnd
	p.config.OnComplete = runner.onComplete

	p.Start()
	defer p.Stop()

	mock := NewMockStream(p, mockConfig)
	err := mock.Run()
	if err != nil {
		t.Errorf("runner failed: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected some output")
	}

	// Check for expected log format elements
	if !containsAny(output, "START", "PHASE") {
		t.Error("expected START or PHASE in log output")
	}
	if !containsAny(output, "COMPLETED", "COMPLETE") {
		t.Error("expected COMPLETED in log output")
	}
}

func TestConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	p := New(config)
	p.Start()
	defer p.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			stepID := p.StartStep(shared.StepKindFileBuild, "Build", "file.go")
			p.UpdateStep(stepID, StepUpdates{Tokens: 100})
			time.Sleep(time.Millisecond)
			p.CompleteStep(stepID)
		}(i)
	}
	wg.Wait()

	report := p.GetReport()
	if len(report.Steps) != 10 {
		t.Errorf("expected 10 steps, got %d", len(report.Steps))
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
