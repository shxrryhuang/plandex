package progress

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	shared "plandex-shared"
)

// TestProgressExamples demonstrates the progress rendering in various scenarios.
// Run with: go test -v -run TestProgressExamples ./app/cli/progress/

func TestProgressExamples(t *testing.T) {
	t.Run("NormalExecution", testNormalExecution)
	t.Run("SlowLLMCall", testSlowLLMCall)
	t.Run("StalledOperation", testStalledOperation)
	t.Run("FailureScenario", testFailureScenario)
	t.Run("NonTTYOutput", testNonTTYOutput)
	t.Run("UserInputWaiting", testUserInputWaiting)
}

func testNormalExecution(t *testing.T) {
	fmt.Println("\n=== Normal Execution Example ===")

	report := &shared.ProgressReport{
		PlanID:     "plan-123",
		Branch:     "main",
		Phase:      shared.ProgressPhaseBuilding,
		PhaseLabel: "Building files",
		StartedAt:  time.Now().Add(-72 * time.Second),
		Steps: []shared.ProgressStep{
			{
				ID:          "step-1",
				Kind:        shared.StepKindContext,
				State:       shared.StepStateCompleted,
				Label:       "Loading context",
				Detail:      "5 files",
				StartedAt:   time.Now().Add(-72 * time.Second),
				CompletedAt: time.Now().Add(-70 * time.Second),
			},
			{
				ID:          "step-2",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateCompleted,
				Label:       "Building file",
				Detail:      "src/models/user.go",
				StartedAt:   time.Now().Add(-55 * time.Second),
				CompletedAt: time.Now().Add(-40 * time.Second),
				TokensProcessed: 423,
			},
			{
				ID:          "step-3",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateCompleted,
				Label:       "Building file",
				Detail:      "src/config/config.go",
				StartedAt:   time.Now().Add(-40 * time.Second),
				CompletedAt: time.Now().Add(-32 * time.Second),
				TokensProcessed: 256,
			},
			{
				ID:          "step-4",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateRunning,
				Label:       "Building file",
				Detail:      "src/api/handlers.go",
				StartedAt:   time.Now().Add(-20 * time.Second),
				Progress:    0.65,
				TokensProcessed: 847,
			},
		},
		CurrentStepID:  "step-4",
		TotalSteps:     4,
		CompletedSteps: 3,
		CanCancel:      true,
	}
	report.Duration = time.Since(report.StartedAt)

	renderer := NewRenderer(os.Stdout, false)
	renderer.RenderReport(report, 3)
	fmt.Println()
}

func testSlowLLMCall(t *testing.T) {
	fmt.Println("\n=== Slow LLM Call Example ===")

	report := &shared.ProgressReport{
		PlanID:     "plan-456",
		Branch:     "feature/auth",
		Phase:      shared.ProgressPhasePlanning,
		PhaseLabel: "Planning task",
		StartedAt:  time.Now().Add(-105 * time.Second),
		Steps: []shared.ProgressStep{
			{
				ID:          "step-1",
				Kind:        shared.StepKindContext,
				State:       shared.StepStateCompleted,
				Label:       "Loading context",
				Detail:      "12 files",
				StartedAt:   time.Now().Add(-105 * time.Second),
				CompletedAt: time.Now().Add(-95 * time.Second),
			},
			{
				ID:          "step-2",
				Kind:        shared.StepKindLLMCall,
				State:       shared.StepStateRunning,
				Label:       "Calling LLM",
				Detail:      "gpt-4-turbo",
				StartedAt:   time.Now().Add(-90 * time.Second),
				Progress:    -1, // Indeterminate
			},
		},
		CurrentStepID:  "step-2",
		TotalSteps:     2,
		CompletedSteps: 1,
		CanCancel:      true,
		SuggestedAction: "LLM is processing. Large tasks may take time.",
	}
	report.Duration = time.Since(report.StartedAt)

	renderer := NewRenderer(os.Stdout, false)
	renderer.RenderReport(report, 7)
	fmt.Println()
}

func testStalledOperation(t *testing.T) {
	fmt.Println("\n=== Stalled Operation Example ===")

	report := &shared.ProgressReport{
		PlanID:     "plan-789",
		Branch:     "main",
		Phase:      shared.ProgressPhasePlanning,
		PhaseLabel: "Planning task",
		StartedAt:  time.Now().Add(-150 * time.Second),
		Steps: []shared.ProgressStep{
			{
				ID:          "step-1",
				Kind:        shared.StepKindContext,
				State:       shared.StepStateCompleted,
				Label:       "Loading context",
				Detail:      "12 files",
				StartedAt:   time.Now().Add(-150 * time.Second),
				CompletedAt: time.Now().Add(-140 * time.Second),
			},
			{
				ID:          "step-2",
				Kind:        shared.StepKindLLMCall,
				State:       shared.StepStateStalled,
				Label:       "Calling LLM",
				Detail:      "gpt-4-turbo",
				StartedAt:   time.Now().Add(-135 * time.Second),
				Progress:    -1,
			},
		},
		CurrentStepID:   "step-2",
		TotalSteps:      2,
		CompletedSteps:  1,
		StalledIDs:      []string{"step-2"},
		CanCancel:       true,
		SuggestedAction: "Operation appears stalled. Consider canceling (s) if no progress.",
	}
	report.Duration = time.Since(report.StartedAt)

	renderer := NewRenderer(os.Stdout, false)
	renderer.RenderReport(report, 5)
	fmt.Println()
}

func testFailureScenario(t *testing.T) {
	fmt.Println("\n=== Failure Scenario Example ===")

	report := &shared.ProgressReport{
		PlanID:     "plan-fail",
		Branch:     "main",
		Phase:      shared.ProgressPhaseBuilding,
		PhaseLabel: "Building files",
		StartedAt:  time.Now().Add(-32 * time.Second),
		Steps: []shared.ProgressStep{
			{
				ID:          "step-1",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateCompleted,
				Label:       "Building file",
				Detail:      "src/api.go",
				StartedAt:   time.Now().Add(-32 * time.Second),
				CompletedAt: time.Now().Add(-20 * time.Second),
				TokensProcessed: 512,
			},
			{
				ID:          "step-2",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateCompleted,
				Label:       "Building file",
				Detail:      "src/models.go",
				StartedAt:   time.Now().Add(-20 * time.Second),
				CompletedAt: time.Now().Add(-12 * time.Second),
				TokensProcessed: 384,
			},
			{
				ID:          "step-3",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateFailed,
				Label:       "Building file",
				Detail:      "src/broken.go",
				StartedAt:   time.Now().Add(-12 * time.Second),
				CompletedAt: time.Now().Add(-5 * time.Second),
				Error:       "syntax error at line 42",
			},
			{
				ID:        "step-4",
				Kind:      shared.StepKindFileBuild,
				State:     shared.StepStatePending,
				Label:     "Building file",
				Detail:    "src/utils.go",
			},
		},
		CurrentStepID:  "step-3",
		TotalSteps:     4,
		CompletedSteps: 2,
		FailedSteps:    1,
		CanCancel:      true,
		CanRetry:       true,
		Warnings:       []string{"Build failed: 1 error, 2 completed, 1 remaining"},
	}
	report.Duration = time.Since(report.StartedAt)

	renderer := NewRenderer(os.Stdout, false)
	renderer.RenderReport(report, 0)
	fmt.Println()
}

func testUserInputWaiting(t *testing.T) {
	fmt.Println("\n=== User Input Waiting Example ===")

	report := &shared.ProgressReport{
		PlanID:     "plan-input",
		Branch:     "main",
		Phase:      shared.ProgressPhaseBuilding,
		PhaseLabel: "Building files",
		StartedAt:  time.Now().Add(-45 * time.Second),
		Steps: []shared.ProgressStep{
			{
				ID:          "step-1",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateCompleted,
				Label:       "Building file",
				Detail:      "src/api.go",
				StartedAt:   time.Now().Add(-45 * time.Second),
				CompletedAt: time.Now().Add(-33 * time.Second),
			},
			{
				ID:          "step-2",
				Kind:        shared.StepKindFileBuild,
				State:       shared.StepStateCompleted,
				Label:       "Building file",
				Detail:      "src/models.go",
				StartedAt:   time.Now().Add(-33 * time.Second),
				CompletedAt: time.Now().Add(-25 * time.Second),
			},
			{
				ID:          "step-3",
				Kind:        shared.StepKindUserInput,
				State:       shared.StepStateWaiting,
				Label:       "Waiting for input",
				Detail:      "missing file: src/config.json",
				StartedAt:   time.Now().Add(-10 * time.Second),
				ProgressMsg: "user response",
			},
		},
		CurrentStepID:   "step-3",
		TotalSteps:      5,
		CompletedSteps:  2,
		CanCancel:       true,
		SuggestedAction: "Waiting for your input.",
	}
	report.Duration = time.Since(report.StartedAt)

	renderer := NewRenderer(os.Stdout, false)
	renderer.RenderReport(report, 2)
	fmt.Println()
}

func testNonTTYOutput(t *testing.T) {
	fmt.Println("\n=== Non-TTY Log Output Example ===")

	var buf bytes.Buffer
	renderer := &Renderer{
		out:     &buf,
		isTTY:   false,
		width:   80,
		verbose: true,
	}

	// Simulate a sequence of progress messages
	messages := []shared.ProgressMessage{
		{
			Type:      shared.ProgressMsgStepStart,
			Timestamp: time.Now().Add(-30 * time.Second),
			Step: &shared.ProgressStep{
				ID:        "step-1",
				Kind:      shared.StepKindContext,
				State:     shared.StepStateRunning,
				Label:     "Loading context",
				Detail:    "12 files",
				StartedAt: time.Now().Add(-30 * time.Second),
			},
		},
		{
			Type:      shared.ProgressMsgStepEnd,
			Timestamp: time.Now().Add(-28 * time.Second),
			Step: &shared.ProgressStep{
				ID:          "step-1",
				Kind:        shared.StepKindContext,
				State:       shared.StepStateCompleted,
				Label:       "Loading context",
				Detail:      "12 files",
				StartedAt:   time.Now().Add(-30 * time.Second),
				CompletedAt: time.Now().Add(-28 * time.Second),
			},
		},
		{
			Type:      shared.ProgressMsgStepStart,
			Timestamp: time.Now().Add(-28 * time.Second),
			Step: &shared.ProgressStep{
				ID:        "step-2",
				Kind:      shared.StepKindLLMCall,
				State:     shared.StepStateRunning,
				Label:     "Calling LLM",
				Detail:    "gpt-4-turbo",
				StartedAt: time.Now().Add(-28 * time.Second),
			},
		},
		{
			Type:      shared.ProgressMsgStepUpdate,
			Timestamp: time.Now().Add(-15 * time.Second),
			Step: &shared.ProgressStep{
				ID:              "step-2",
				Kind:            shared.StepKindLLMCall,
				State:           shared.StepStateRunning,
				Label:           "Calling LLM",
				Detail:          "gpt-4-turbo",
				StartedAt:       time.Now().Add(-28 * time.Second),
				TokensProcessed: 1500,
			},
		},
		{
			Type:      shared.ProgressMsgStepEnd,
			Timestamp: time.Now(),
			Step: &shared.ProgressStep{
				ID:              "step-2",
				Kind:            shared.StepKindLLMCall,
				State:           shared.StepStateCompleted,
				Label:           "Calling LLM",
				Detail:          "gpt-4-turbo",
				StartedAt:       time.Now().Add(-28 * time.Second),
				CompletedAt:     time.Now(),
				TokensProcessed: 2500,
			},
		},
	}

	for _, msg := range messages {
		renderer.RenderStepEvent(&msg)
	}

	// Print the collected log output
	fmt.Println("Structured log output:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Print(buf.String())
	fmt.Println(strings.Repeat("-", 70))
}

// TestStateReliability verifies state classification
func TestStateReliability(t *testing.T) {
	tests := []struct {
		state       shared.StepState
		guaranteed  bool
		bestEffort  bool
		terminal    bool
	}{
		{shared.StepStatePending, false, false, false},
		{shared.StepStateRunning, false, true, false},
		{shared.StepStateWaiting, false, true, false},
		{shared.StepStateStalled, false, true, false},
		{shared.StepStateCompleted, true, false, true},
		{shared.StepStateFailed, true, false, true},
		{shared.StepStateSkipped, true, false, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsGuaranteed(); got != tt.guaranteed {
				t.Errorf("IsGuaranteed() = %v, want %v", got, tt.guaranteed)
			}
			if got := tt.state.IsBestEffort(); got != tt.bestEffort {
				t.Errorf("IsBestEffort() = %v, want %v", got, tt.bestEffort)
			}
			if got := tt.state.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}

// TestStallDetection verifies stall detection logic
func TestStallDetection(t *testing.T) {
	// LLM call running for 90 seconds (3x expected 30s) should be stalled
	step := &shared.ProgressStep{
		Kind:      shared.StepKindLLMCall,
		State:     shared.StepStateRunning,
		StartedAt: time.Now().Add(-90 * time.Second),
	}

	if !step.IsStalled() {
		t.Error("Expected LLM call running for 90s to be stalled")
	}

	// Same step but just started should not be stalled
	step.StartedAt = time.Now().Add(-5 * time.Second)
	if step.IsStalled() {
		t.Error("Expected recently started step to not be stalled")
	}

	// User input should never stall
	inputStep := &shared.ProgressStep{
		Kind:      shared.StepKindUserInput,
		State:     shared.StepStateWaiting,
		StartedAt: time.Now().Add(-300 * time.Second),
	}
	if inputStep.IsStalled() {
		t.Error("User input steps should never be stalled")
	}
}
