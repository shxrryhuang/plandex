package pipeline

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	shared "plandex-shared"
)

// MockStreamConfig configures the mock stream generator
type MockStreamConfig struct {
	// Scenario to simulate
	Scenario Scenario

	// Timing multiplier (1.0 = normal, 0.5 = fast, 2.0 = slow)
	TimeScale float64

	// Number of files to build
	FileCount int

	// Whether to simulate failures
	SimulateFailure bool
	FailAtFile      int

	// Whether to simulate stalls
	SimulateStall bool
	StallAtStep   string // "llm", "build", "context"
	StallDuration time.Duration
}

// Scenario represents a predefined test scenario
type Scenario string

const (
	ScenarioNormal      Scenario = "normal"      // Normal successful execution
	ScenarioSlowLLM     Scenario = "slow_llm"    // Slow LLM response
	ScenarioStalled     Scenario = "stalled"     // Stalled operation
	ScenarioFailure     Scenario = "failure"     // Build failure
	ScenarioUserInput   Scenario = "user_input"  // Waiting for user input
	ScenarioLargeTask   Scenario = "large_task"  // Many files to build
	ScenarioQuickTask   Scenario = "quick_task"  // Fast, small task
	ScenarioMixed       Scenario = "mixed"       // Mix of successes and failures
)

// DefaultMockConfig returns a default mock configuration
func DefaultMockConfig() MockStreamConfig {
	return MockStreamConfig{
		Scenario:   ScenarioNormal,
		TimeScale:  1.0,
		FileCount:  5,
	}
}

// MockStream generates simulated stream events for testing
type MockStream struct {
	config   MockStreamConfig
	pipeline *Pipeline
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewMockStream creates a new mock stream generator
func NewMockStream(pipeline *Pipeline, config MockStreamConfig) *MockStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &MockStream{
		config:   config,
		pipeline: pipeline,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Run executes the mock stream scenario
func (m *MockStream) Run() error {
	switch m.config.Scenario {
	case ScenarioNormal:
		return m.runNormalScenario()
	case ScenarioSlowLLM:
		return m.runSlowLLMScenario()
	case ScenarioStalled:
		return m.runStalledScenario()
	case ScenarioFailure:
		return m.runFailureScenario()
	case ScenarioUserInput:
		return m.runUserInputScenario()
	case ScenarioLargeTask:
		return m.runLargeTaskScenario()
	case ScenarioQuickTask:
		return m.runQuickTaskScenario()
	case ScenarioMixed:
		return m.runMixedScenario()
	default:
		return m.runNormalScenario()
	}
}

// Stop cancels the mock stream
func (m *MockStream) Stop() {
	m.cancel()
}

// sleep with time scaling and context cancellation
func (m *MockStream) sleep(d time.Duration) bool {
	scaled := time.Duration(float64(d) * m.config.TimeScale)
	select {
	case <-m.ctx.Done():
		return false
	case <-time.After(scaled):
		return true
	}
}

// runNormalScenario simulates a successful plan execution
func (m *MockStream) runNormalScenario() error {
	p := m.pipeline

	// Phase 1: Initializing
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "5 files")
	if !m.sleep(500 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Phase 2: Planning
	p.SetPhase(PhasePlanning, "Planning task")
	llmID := p.StartStep(StepKindLLMCall, "Calling LLM", "claude-3-opus")

	// Simulate token streaming
	for i := 0; i < 10; i++ {
		if !m.sleep(200 * time.Millisecond) {
			return m.ctx.Err()
		}
		p.UpdateStep(llmID, StepUpdates{Tokens: 100 + rand.Intn(50)})
	}
	p.CompleteStep(llmID)

	// Phase 3: Building
	p.SetPhase(PhaseBuilding, "Building files")
	files := []string{
		"src/api/handlers.go",
		"src/models/user.go",
		"src/config/config.go",
		"src/utils/helpers.go",
		"src/main.go",
	}

	for i, file := range files[:m.config.FileCount] {
		if i >= len(files) {
			break
		}
		buildID := p.StartStep(StepKindFileBuild, "Building file", file)

		// Simulate build progress
		tokens := 0
		for j := 0; j < 5; j++ {
			if !m.sleep(100 * time.Millisecond) {
				return m.ctx.Err()
			}
			newTokens := 50 + rand.Intn(100)
			tokens += newTokens
			p.UpdateStep(buildID, StepUpdates{
				Tokens:   newTokens,
				Progress: float64(j+1) / 5.0,
			})
		}
		p.CompleteStep(buildID)
	}

	// Phase 4: Applying
	p.SetPhase(PhaseApplying, "Applying changes")
	for i, file := range files[:m.config.FileCount] {
		if i >= len(files) {
			break
		}
		writeID := p.StartStep(StepKindFileWrite, "Writing file", file)
		if !m.sleep(100 * time.Millisecond) {
			return m.ctx.Err()
		}
		p.CompleteStep(writeID)
	}

	// Phase 5: Validating
	p.SetPhase(PhaseValidating, "Validating changes")
	validateID := p.StartStep(StepKindValidation, "Running syntax check", "")
	if !m.sleep(300 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(validateID)

	// Complete
	p.Complete()
	return nil
}

// runSlowLLMScenario simulates a slow LLM response
func (m *MockStream) runSlowLLMScenario() error {
	p := m.pipeline

	// Initializing
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "12 files")
	if !m.sleep(800 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Planning with slow LLM
	p.SetPhase(PhasePlanning, "Planning task")
	llmID := p.StartStep(StepKindLLMCall, "Calling LLM", "gpt-4-turbo")

	// Simulate slow token streaming
	for i := 0; i < 30; i++ {
		if !m.sleep(500 * time.Millisecond) {
			return m.ctx.Err()
		}
		p.UpdateStep(llmID, StepUpdates{Tokens: 50 + rand.Intn(30)})
	}
	p.CompleteStep(llmID)

	// Quick build
	p.SetPhase(PhaseBuilding, "Building files")
	buildID := p.StartStep(StepKindFileBuild, "Building file", "src/main.go")
	if !m.sleep(500 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(buildID)

	p.Complete()
	return nil
}

// runStalledScenario simulates a stalled operation
func (m *MockStream) runStalledScenario() error {
	p := m.pipeline

	// Initializing
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "8 files")
	if !m.sleep(500 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Planning - will stall
	p.SetPhase(PhasePlanning, "Planning task")
	llmID := p.StartStep(StepKindLLMCall, "Calling LLM", "gpt-4-turbo")

	// Some initial progress
	for i := 0; i < 3; i++ {
		if !m.sleep(200 * time.Millisecond) {
			return m.ctx.Err()
		}
		p.UpdateStep(llmID, StepUpdates{Tokens: 100})
	}

	// Stall for configured duration (or 5 seconds by default for demo)
	stallDuration := m.config.StallDuration
	if stallDuration == 0 {
		stallDuration = 5 * time.Second
	}
	if !m.sleep(stallDuration) {
		return m.ctx.Err()
	}

	// Eventually complete
	p.CompleteStep(llmID)
	p.Complete()
	return nil
}

// runFailureScenario simulates a build failure
func (m *MockStream) runFailureScenario() error {
	p := m.pipeline

	// Initializing
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "5 files")
	if !m.sleep(300 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Planning
	p.SetPhase(PhasePlanning, "Planning task")
	llmID := p.StartStep(StepKindLLMCall, "Calling LLM", "claude-3-opus")
	if !m.sleep(1 * time.Second) {
		return m.ctx.Err()
	}
	p.UpdateStep(llmID, StepUpdates{Tokens: 500})
	p.CompleteStep(llmID)

	// Building - one file will fail
	p.SetPhase(PhaseBuilding, "Building files")

	files := []string{"src/api.go", "src/broken.go", "src/models.go"}
	failAt := m.config.FailAtFile
	if failAt == 0 {
		failAt = 1 // Fail at second file by default
	}

	for i, file := range files {
		buildID := p.StartStep(StepKindFileBuild, "Building file", file)
		if !m.sleep(500 * time.Millisecond) {
			return m.ctx.Err()
		}

		if i == failAt {
			p.FailStep(buildID, "syntax error at line 42: unexpected token")
			p.Fail(fmt.Errorf("build failed: syntax error in %s", file))
			return nil
		}
		p.CompleteStep(buildID)
	}

	p.Complete()
	return nil
}

// runUserInputScenario simulates waiting for user input
func (m *MockStream) runUserInputScenario() error {
	p := m.pipeline

	// Initializing
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "3 files")
	if !m.sleep(300 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Building with user input needed
	p.SetPhase(PhaseBuilding, "Building files")

	buildID := p.StartStep(StepKindFileBuild, "Building file", "src/api.go")
	if !m.sleep(500 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(buildID)

	// Wait for user input
	inputID := p.StartStep(StepKindUserInput, "Waiting for input", "missing file: src/config.json")
	p.WaitStep(inputID, "Please provide config.json")

	// Simulate waiting
	if !m.sleep(3 * time.Second) {
		return m.ctx.Err()
	}

	// User provided input
	p.CompleteStep(inputID)

	// Continue building
	buildID2 := p.StartStep(StepKindFileBuild, "Building file", "src/config.go")
	if !m.sleep(500 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(buildID2)

	p.Complete()
	return nil
}

// runLargeTaskScenario simulates a large task with many files
func (m *MockStream) runLargeTaskScenario() error {
	p := m.pipeline
	fileCount := 20
	if m.config.FileCount > 0 {
		fileCount = m.config.FileCount
	}

	// Initializing
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "50 files")
	if !m.sleep(1 * time.Second) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Planning
	p.SetPhase(PhasePlanning, "Planning task")
	llmID := p.StartStep(StepKindLLMCall, "Calling LLM", "claude-3-opus")
	for i := 0; i < 20; i++ {
		if !m.sleep(100 * time.Millisecond) {
			return m.ctx.Err()
		}
		p.UpdateStep(llmID, StepUpdates{Tokens: 200})
	}
	p.CompleteStep(llmID)

	// Building many files
	p.SetPhase(PhaseBuilding, "Building files")
	for i := 0; i < fileCount; i++ {
		file := fmt.Sprintf("src/file_%03d.go", i+1)
		buildID := p.StartStep(StepKindFileBuild, "Building file", file)

		// Quick build per file
		if !m.sleep(200 * time.Millisecond) {
			return m.ctx.Err()
		}
		p.UpdateStep(buildID, StepUpdates{Tokens: 100 + rand.Intn(200)})
		p.CompleteStep(buildID)
	}

	p.Complete()
	return nil
}

// runQuickTaskScenario simulates a fast, simple task
func (m *MockStream) runQuickTaskScenario() error {
	p := m.pipeline

	// Quick init
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "1 file")
	if !m.sleep(100 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Quick planning
	p.SetPhase(PhasePlanning, "Planning task")
	llmID := p.StartStep(StepKindLLMCall, "Calling LLM", "claude-3-haiku")
	if !m.sleep(300 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.UpdateStep(llmID, StepUpdates{Tokens: 150})
	p.CompleteStep(llmID)

	// Quick build
	p.SetPhase(PhaseBuilding, "Building files")
	buildID := p.StartStep(StepKindFileBuild, "Building file", "src/fix.go")
	if !m.sleep(200 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(buildID)

	p.Complete()
	return nil
}

// runMixedScenario simulates mixed results
func (m *MockStream) runMixedScenario() error {
	p := m.pipeline

	// Initializing
	p.SetPhase(PhaseInitializing, "Initializing")
	contextID := p.StartStep(StepKindContext, "Loading context", "10 files")
	if !m.sleep(500 * time.Millisecond) {
		return m.ctx.Err()
	}
	p.CompleteStep(contextID)

	// Planning
	p.SetPhase(PhasePlanning, "Planning task")
	llmID := p.StartStep(StepKindLLMCall, "Calling LLM", "gpt-4")
	if !m.sleep(1 * time.Second) {
		return m.ctx.Err()
	}
	p.UpdateStep(llmID, StepUpdates{Tokens: 800})
	p.CompleteStep(llmID)

	// Building with mixed results
	p.SetPhase(PhaseBuilding, "Building files")

	files := []struct {
		name   string
		result string // "success", "fail", "skip"
	}{
		{"src/api.go", "success"},
		{"src/deprecated.go", "skip"},
		{"src/handlers.go", "success"},
		{"src/broken.go", "fail"},
		{"src/models.go", "success"},
	}

	for _, f := range files {
		buildID := p.StartStep(StepKindFileBuild, "Building file", f.name)
		if !m.sleep(400 * time.Millisecond) {
			return m.ctx.Err()
		}

		switch f.result {
		case "success":
			p.UpdateStep(buildID, StepUpdates{Tokens: 200})
			p.CompleteStep(buildID)
		case "fail":
			p.FailStep(buildID, "compilation error")
		case "skip":
			p.SkipStep(buildID)
		}
	}

	// Complete despite failures (partial success)
	p.Complete()
	return nil
}

// Helper constants for phase and step kinds (re-exported for convenience)
const (
	PhaseInitializing = shared.PhaseInitializing
	PhasePlanning     = shared.PhasePlanning
	PhaseDescribing   = shared.PhaseDescribing
	PhaseBuilding     = shared.PhaseBuilding
	PhaseApplying     = shared.PhaseApplying
	PhaseValidating   = shared.PhaseValidating
	PhaseCompleted    = shared.PhaseCompleted
	PhaseFailed       = shared.PhaseFailed
	PhaseStopped      = shared.PhaseStopped

	StepKindLLMCall    = shared.StepKindLLMCall
	StepKindContext    = shared.StepKindContext
	StepKindFileBuild  = shared.StepKindFileBuild
	StepKindFileWrite  = shared.StepKindFileWrite
	StepKindToolExec   = shared.StepKindToolExec
	StepKindUserInput  = shared.StepKindUserInput
	StepKindValidation = shared.StepKindValidation
)
