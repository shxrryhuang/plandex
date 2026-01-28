package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	shared "plandex-shared"
)

// Runner executes a progress pipeline with real-time visual output
type Runner struct {
	pipeline *Pipeline
	output   io.Writer
	isTTY    bool
	width    int

	// Display state
	mu             sync.Mutex
	lastRenderTime time.Time
	spinnerFrame   int
	spinnerFrames  []string

	// Spinner control
	spinnerCtx    context.Context
	spinnerCancel context.CancelFunc
}

// RunnerConfig configures the runner
type RunnerConfig struct {
	Output io.Writer
	IsTTY  bool
	Width  int
}

// DefaultRunnerConfig returns a default runner configuration
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		Output: os.Stdout,
		IsTTY:  true,
		Width:  80,
	}
}

// NewRunner creates a new pipeline runner
func NewRunner(pipeline *Pipeline, config RunnerConfig) *Runner {
	return &Runner{
		pipeline:      pipeline,
		output:        config.Output,
		isTTY:         config.IsTTY,
		width:         config.Width,
		spinnerFrames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

// Run executes a scenario and displays progress in real-time
func (r *Runner) Run(scenario Scenario) error {
	// Configure pipeline callbacks
	r.pipeline.config.OnPhaseChange = r.onPhaseChange
	r.pipeline.config.OnStepStart = r.onStepStart
	r.pipeline.config.OnStepUpdate = r.onStepUpdate
	r.pipeline.config.OnStepEnd = r.onStepEnd
	r.pipeline.config.OnStall = r.onStall
	r.pipeline.config.OnComplete = r.onComplete
	r.pipeline.config.OnError = r.onError

	// Start pipeline
	r.pipeline.Start()
	defer r.pipeline.Stop()

	// Create and run mock stream
	mockConfig := DefaultMockConfig()
	mockConfig.Scenario = scenario
	mockConfig.TimeScale = 0.3 // Speed up for demo

	mock := NewMockStream(r.pipeline, mockConfig)

	// Start spinner update goroutine if TTY
	if r.isTTY {
		r.spinnerCtx, r.spinnerCancel = context.WithCancel(context.Background())
		go r.spinnerLoop()
		defer r.spinnerCancel() // Ensure spinner goroutine is stopped
	}

	return mock.Run()
}

// spinnerLoop updates the spinner animation
func (r *Runner) spinnerLoop() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.spinnerCtx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			r.spinnerFrame = (r.spinnerFrame + 1) % len(r.spinnerFrames)
			r.mu.Unlock()
		}
	}
}

// getSpinner returns the current spinner frame
func (r *Runner) getSpinner() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spinnerFrames[r.spinnerFrame]
}

// Callbacks

func (r *Runner) onPhaseChange(phase shared.ProgressPhase, label string) {
	if r.isTTY {
		icon := r.getPhaseIcon(phase)
		fmt.Fprintf(r.output, "\n%s %s\n",
			color.New(color.Bold).Sprint(icon+" "+label),
			color.HiBlackString("[starting]"))
	} else {
		r.logEvent("PHASE", string(phase), label, "")
	}
}

func (r *Runner) onStepStart(step *shared.Step) {
	if r.isTTY {
		icon := r.getKindIcon(step.Kind)
		spinner := color.CyanString(r.getSpinner())
		detail := ""
		if step.Detail != "" {
			detail = color.HiBlackString(" (%s)", step.Detail)
		}
		fmt.Fprintf(r.output, "%s %s %s%s\n", spinner, icon, step.Label, detail)
	} else {
		r.logEvent("START", string(step.Kind), step.Label, step.Detail)
	}
}

func (r *Runner) onStepUpdate(step *shared.Step) {
	if !r.isTTY {
		extra := ""
		if step.TokensProcessed > 0 {
			extra = fmt.Sprintf("%d tokens", step.TokensProcessed)
		}
		r.logEvent("UPDATE", string(step.Kind), step.Label, extra)
	}
	// In TTY mode, spinner handles visual updates
}

func (r *Runner) onStepEnd(step *shared.Step) {
	if r.isTTY {
		icon := r.getKindIcon(step.Kind)
		stateIcon := r.getStateIcon(step.State)
		detail := ""
		if step.Detail != "" {
			detail = color.HiBlackString(" (%s)", step.Detail)
		}

		duration := ""
		if !step.CompletedAt.IsZero() && !step.StartedAt.IsZero() {
			duration = color.HiBlackString(" %s", r.formatDuration(step.CompletedAt.Sub(step.StartedAt)))
		}

		errMsg := ""
		if step.Error != "" {
			errMsg = color.RedString(" [%s]", step.Error)
		}

		fmt.Fprintf(r.output, "%s %s %s%s%s%s\n", stateIcon, icon, step.Label, detail, duration, errMsg)
	} else {
		state := strings.ToUpper(string(step.State))
		extra := ""
		if step.Error != "" {
			extra = "ERROR: " + step.Error
		}
		r.logEvent(state, string(step.Kind), step.Label, extra)
	}
}

func (r *Runner) onStall(step *shared.Step) {
	if r.isTTY {
		icon := r.getKindIcon(step.Kind)
		stateIcon := color.New(color.FgRed, color.Bold).Sprint("⚠")
		detail := ""
		if step.Detail != "" {
			detail = color.HiBlackString(" (%s)", step.Detail)
		}
		duration := "unknown"
		if !step.StartedAt.IsZero() {
			duration = r.formatDuration(time.Since(step.StartedAt))
		}
		fmt.Fprintf(r.output, "%s %s %s%s %s\n", stateIcon, icon, step.Label, detail,
			color.New(color.FgRed, color.Bold).Sprintf("[stalled %s]", duration))
		fmt.Fprintf(r.output, "%s\n", color.New(color.FgRed, color.Bold).Sprint("⚠ Operation may be stalled"))
	} else {
		durationStr := "unknown"
		if !step.StartedAt.IsZero() {
			durationStr = time.Since(step.StartedAt).String()
		}
		r.logEvent("STALL", string(step.Kind), step.Label, fmt.Sprintf("duration: %s", durationStr))
	}
}

func (r *Runner) onComplete(report *shared.ProgressReport) {
	if r.isTTY {
		duration := "unknown"
		if !report.CompletedAt.IsZero() && !report.StartedAt.IsZero() {
			duration = r.formatDuration(report.CompletedAt.Sub(report.StartedAt))
		}
		fmt.Fprintf(r.output, "\n%s %s\n",
			color.New(color.Bold, color.FgGreen).Sprint("✅ Completed"),
			color.HiBlackString("[%s]", duration))

		// Summary
		completed := 0
		failed := 0
		skipped := 0
		for _, s := range report.Steps {
			switch s.State {
			case shared.StepStateCompleted:
				completed++
			case shared.StepStateFailed:
				failed++
			case shared.StepStateSkipped:
				skipped++
			}
		}
		fmt.Fprintf(r.output, "  %s completed", color.GreenString("%d", completed))
		if failed > 0 {
			fmt.Fprintf(r.output, ", %s failed", color.RedString("%d", failed))
		}
		if skipped > 0 {
			fmt.Fprintf(r.output, ", %s skipped", color.YellowString("%d", skipped))
		}
		fmt.Fprintln(r.output)
	} else {
		r.logEvent("COMPLETE", "", "Pipeline completed", "")
	}
}

func (r *Runner) onError(err error) {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	if r.isTTY {
		fmt.Fprintf(r.output, "\n%s %s\n",
			color.New(color.Bold, color.FgRed).Sprint("❌ Failed"),
			color.RedString("[%s]", errMsg))
	} else {
		r.logEvent("ERROR", "", "Pipeline failed", errMsg)
	}
}

// Helper methods

func (r *Runner) logEvent(state, kind, label, extra string) {
	timestamp := time.Now().Format("15:04:05")
	stateStr := fmt.Sprintf("[%-6s]", state)

	switch state {
	case "START", "UPDATE":
		stateStr = color.CyanString(stateStr)
	case "COMPLETED", "COMPLETE":
		stateStr = color.GreenString(stateStr)
	case "FAILED", "ERROR":
		stateStr = color.RedString(stateStr)
	case "STALL":
		stateStr = color.New(color.FgRed, color.Bold).Sprint(stateStr)
	case "SKIPPED":
		stateStr = color.YellowString(stateStr)
	}

	if extra != "" {
		fmt.Fprintf(r.output, "[%s] %s %s > %s | %s\n", timestamp, stateStr, kind, label, extra)
	} else {
		fmt.Fprintf(r.output, "[%s] %s %s > %s\n", timestamp, stateStr, kind, label)
	}
}

func (r *Runner) getPhaseIcon(phase shared.ProgressPhase) string {
	switch phase {
	case shared.PhaseInitializing:
		return "🚀"
	case shared.PhasePlanning:
		return "🧠"
	case shared.PhaseDescribing:
		return "📝"
	case shared.PhaseBuilding:
		return "🏗"
	case shared.PhaseApplying:
		return "📦"
	case shared.PhaseValidating:
		return "🔍"
	case shared.PhaseCompleted:
		return "✅"
	case shared.PhaseFailed:
		return "❌"
	case shared.PhaseStopped:
		return "🛑"
	default:
		return "▶"
	}
}

func (r *Runner) getKindIcon(kind shared.StepKind) string {
	switch kind {
	case shared.StepKindLLMCall:
		return "🤖"
	case shared.StepKindContext:
		return "📚"
	case shared.StepKindFileBuild:
		return "📄"
	case shared.StepKindFileWrite:
		return "💾"
	case shared.StepKindToolExec:
		return "🔧"
	case shared.StepKindUserInput:
		return "⌨"
	case shared.StepKindValidation:
		return "🔍"
	default:
		return "▪"
	}
}

func (r *Runner) getStateIcon(state shared.StepState) string {
	switch state {
	case shared.StepStateCompleted:
		return color.GreenString("●")
	case shared.StepStateFailed:
		return color.RedString("✗")
	case shared.StepStateSkipped:
		return color.YellowString("○")
	case shared.StepStateRunning:
		return color.CyanString(r.getSpinner())
	case shared.StepStateWaiting:
		return color.YellowString("◔")
	case shared.StepStateStalled:
		return color.New(color.FgRed, color.Bold).Sprint("⚠")
	case shared.StepStatePending:
		return color.HiBlackString("○")
	default:
		return "?"
	}
}

func (r *Runner) formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", minutes, seconds)
}
