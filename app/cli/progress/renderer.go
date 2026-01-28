package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	shared "plandex-shared"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// Renderer handles progress output for both TTY and non-TTY environments.
type Renderer struct {
	out       io.Writer
	isTTY     bool
	width     int
	verbose   bool
	lastLines int // For clearing previous output in TTY mode
}

// NewRenderer creates a new progress renderer.
func NewRenderer(out io.Writer, verbose bool) *Renderer {
	isTTY := false
	width := 80

	if f, ok := out.(*os.File); ok {
		isTTY = term.IsTerminal(int(f.Fd()))
		if isTTY {
			if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
				width = w
			}
		}
	}

	return &Renderer{
		out:     out,
		isTTY:   isTTY,
		width:   width,
		verbose: verbose,
	}
}

// Icons and symbols for different states and kinds
var (
	// State icons - clearly distinguish guaranteed vs best-effort
	iconPending   = "○"  // Empty circle - not started
	iconRunning   = "◐"  // Half circle - in progress (best-effort)
	iconWaiting   = "◔"  // Quarter circle - waiting (best-effort)
	iconStalled   = "⚠"  // Warning - may need attention
	iconCompleted = "●"  // Filled circle - done (guaranteed)
	iconFailed    = "✗"  // X mark - failed (guaranteed)
	iconSkipped   = "⊘"  // Circle with slash - skipped (guaranteed)

	// Kind icons
	iconLLM        = "🤖"
	iconFile       = "📄"
	iconTool       = "🔧"
	iconValidation = "✓"
	iconContext    = "📚"
	iconNetwork    = "🌐"
	iconUserInput  = "⌨"
	iconInternal   = "⚙"

	// Phase icons
	iconPhaseInit     = "🚀"
	iconPhasePlan     = "🧠"
	iconPhaseDescribe = "📝"
	iconPhaseBuild    = "🏗"
	iconPhaseApply    = "📦"
	iconPhaseValidate = "🔍"
	iconPhaseComplete = "✅"
	iconPhaseFailed   = "❌"
	iconPhaseStopped  = "⏹"
)

// Colors
var (
	colorPending   = color.New(color.FgWhite, color.Faint)
	colorRunning   = color.New(color.FgCyan)
	colorWaiting   = color.New(color.FgYellow)
	colorStalled   = color.New(color.FgRed, color.Bold)
	colorCompleted = color.New(color.FgGreen)
	colorFailed    = color.New(color.FgRed)
	colorSkipped   = color.New(color.FgWhite, color.Faint)
	colorPhase     = color.New(color.FgHiWhite, color.Bold)
	colorLabel     = color.New(color.FgWhite)
	colorDetail    = color.New(color.FgWhite, color.Faint)
	colorTime      = color.New(color.FgHiBlack)
	colorWarning   = color.New(color.FgYellow, color.Bold)
	colorHelp      = color.New(color.FgHiBlack)
)

// Spinner frames for TTY animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// RenderReport outputs the full progress report.
func (r *Renderer) RenderReport(report *shared.ProgressReport, spinnerFrame int) {
	if r.isTTY {
		r.renderTTY(report, spinnerFrame)
	} else {
		r.renderNonTTY(report)
	}
}

// renderTTY renders progress with ANSI escape codes for dynamic updates.
func (r *Renderer) renderTTY(report *shared.ProgressReport, spinnerFrame int) {
	var b strings.Builder

	// Clear previous output
	if r.lastLines > 0 {
		b.WriteString(fmt.Sprintf("\033[%dA\033[J", r.lastLines))
	}

	lines := 0

	// Phase header
	phaseIcon := r.getPhaseIcon(report.Phase)
	phaseLine := fmt.Sprintf("%s %s", phaseIcon, colorPhase.Sprint(report.PhaseLabel))
	if elapsed := r.formatDuration(report.Duration); elapsed != "" {
		phaseLine += colorTime.Sprintf(" [%s]", elapsed)
	}
	b.WriteString(phaseLine + "\n")
	lines++

	// Progress bar
	if report.TotalSteps > 0 {
		progressLine := r.renderProgressBar(report.CompletedSteps, report.TotalSteps, r.width-4)
		b.WriteString(progressLine + "\n")
		lines++
	}

	// Current step with spinner
	if current := report.CurrentStep(); current != nil {
		spinner := spinnerFrames[spinnerFrame%len(spinnerFrames)]
		stepLine := r.renderStep(current, spinner, true)
		b.WriteString(stepLine + "\n")
		lines++

		// Show substeps if present
		for _, child := range current.Children {
			childLine := "  " + r.renderStep(&child, spinner, false)
			b.WriteString(childLine + "\n")
			lines++
		}
	}

	// Recent completed steps (last 3)
	recentCompleted := r.getRecentCompletedSteps(report.Steps, 3)
	for _, step := range recentCompleted {
		stepLine := r.renderStep(&step, "", false)
		b.WriteString(stepLine + "\n")
		lines++
	}

	// Warnings
	for _, warning := range report.Warnings {
		b.WriteString(colorWarning.Sprintf("⚠ %s\n", warning))
		lines++
	}

	// Stall warnings
	if len(report.StalledIDs) > 0 {
		b.WriteString(colorStalled.Sprint("⚠ Operation may be stalled\n"))
		lines++
	}

	// Suggested action
	if report.SuggestedAction != "" {
		b.WriteString(colorHelp.Sprintf("💡 %s\n", report.SuggestedAction))
		lines++
	}

	// Help line
	helpLine := r.renderHelpLine(report)
	b.WriteString(helpLine)
	lines++

	r.lastLines = lines
	fmt.Fprint(r.out, b.String())
}

// renderNonTTY renders progress as structured log lines for non-interactive output.
func (r *Renderer) renderNonTTY(report *shared.ProgressReport) {
	timestamp := time.Now().Format("15:04:05")

	// Only output on state changes (not every heartbeat)
	current := report.CurrentStep()
	if current == nil {
		return
	}

	// Format: [TIME] [STATE] PHASE > STEP (detail)
	stateTag := r.getStateTag(current.State)
	kindIcon := r.getKindIcon(current.Kind)

	line := fmt.Sprintf("[%s] [%s] %s > %s %s",
		timestamp,
		stateTag,
		report.Phase,
		kindIcon,
		current.Label,
	)

	if current.Detail != "" {
		line += fmt.Sprintf(" (%s)", current.Detail)
	}

	if current.Progress >= 0 && current.Progress <= 1 {
		line += fmt.Sprintf(" [%.0f%%]", current.Progress*100)
	}

	if current.Error != "" {
		line += fmt.Sprintf(" ERROR: %s", current.Error)
	}

	fmt.Fprintln(r.out, line)
}

// RenderStepEvent outputs a single step event (for logging).
func (r *Renderer) RenderStepEvent(msg *shared.ProgressMessage) {
	if msg.Step == nil {
		return
	}

	timestamp := msg.Timestamp.Format("15:04:05.000")
	step := msg.Step

	var eventType string
	switch msg.Type {
	case shared.ProgressMsgStepStart:
		eventType = "START"
	case shared.ProgressMsgStepUpdate:
		eventType = "UPDATE"
	case shared.ProgressMsgStepEnd:
		eventType = "END"
	default:
		eventType = string(msg.Type)
	}

	stateMarker := ""
	if step.State.IsGuaranteed() {
		stateMarker = " [CONFIRMED]"
	} else if step.State.IsBestEffort() {
		stateMarker = " [SIGNAL]"
	}

	line := fmt.Sprintf("[%s] %s %s: %s%s",
		timestamp,
		eventType,
		step.State,
		step.Label,
		stateMarker,
	)

	if step.Detail != "" {
		line += fmt.Sprintf(" | %s", step.Detail)
	}

	if step.Duration() > 0 {
		line += fmt.Sprintf(" | %s", r.formatDuration(step.Duration()))
	}

	fmt.Fprintln(r.out, line)
}

// renderProgressBar creates an ASCII progress bar.
func (r *Renderer) renderProgressBar(completed, total, width int) string {
	if width < 10 {
		width = 10
	}

	barWidth := width - 10 // Leave room for percentage and brackets
	if barWidth < 5 {
		barWidth = 5
	}

	progress := float64(completed) / float64(total)
	filled := int(progress * float64(barWidth))

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	pct := fmt.Sprintf("%3.0f%%", progress*100)

	return fmt.Sprintf("  [%s] %s", bar, pct)
}

// renderStep formats a single step for display.
func (r *Renderer) renderStep(step *shared.ProgressStep, spinner string, isCurrent bool) string {
	stateIcon := r.getStateIcon(step.State)
	kindIcon := r.getKindIcon(step.Kind)
	stateColor := r.getStateColor(step.State)

	// Use spinner for running steps in TTY mode
	if spinner != "" && (step.State == shared.StepStateRunning || step.State == shared.StepStateWaiting) {
		stateIcon = spinner
	}

	var b strings.Builder
	b.WriteString(stateColor.Sprint(stateIcon))
	b.WriteString(" ")
	b.WriteString(kindIcon)
	b.WriteString(" ")

	if isCurrent {
		b.WriteString(colorLabel.Sprint(step.Label))
	} else {
		b.WriteString(colorDetail.Sprint(step.Label))
	}

	// Add detail
	if step.Detail != "" {
		b.WriteString(colorDetail.Sprintf(" (%s)", step.Detail))
	}

	// Add progress for running steps
	if step.State == shared.StepStateRunning && step.Progress > 0 {
		b.WriteString(colorDetail.Sprintf(" %.0f%%", step.Progress*100))
	}

	// Add token count if available
	if step.TokensProcessed > 0 {
		b.WriteString(colorDetail.Sprintf(" %d🪙", step.TokensProcessed))
	}

	// Add duration for completed or long-running steps
	if step.State.IsTerminal() || step.Duration() > 5*time.Second {
		if dur := r.formatDuration(step.Duration()); dur != "" {
			b.WriteString(colorTime.Sprintf(" %s", dur))
		}
	}

	// Add error
	if step.Error != "" {
		b.WriteString(colorFailed.Sprintf(" [%s]", step.Error))
	}

	return b.String()
}

// renderHelpLine shows available actions.
func (r *Renderer) renderHelpLine(report *shared.ProgressReport) string {
	var actions []string

	if report.CanCancel {
		actions = append(actions, "(s)top")
	}

	actions = append(actions, "(b)ackground")

	if report.Phase != shared.PhaseCompleted && report.Phase != shared.PhaseFailed {
		actions = append(actions, "(j/k) scroll")
	}

	return colorHelp.Sprint(strings.Join(actions, " • "))
}

func (r *Renderer) getStateIcon(state shared.StepState) string {
	switch state {
	case shared.StepStatePending:
		return iconPending
	case shared.StepStateRunning:
		return iconRunning
	case shared.StepStateWaiting:
		return iconWaiting
	case shared.StepStateStalled:
		return iconStalled
	case shared.StepStateCompleted:
		return iconCompleted
	case shared.StepStateFailed:
		return iconFailed
	case shared.StepStateSkipped:
		return iconSkipped
	default:
		return "?"
	}
}

func (r *Renderer) getStateColor(state shared.StepState) *color.Color {
	switch state {
	case shared.StepStatePending:
		return colorPending
	case shared.StepStateRunning:
		return colorRunning
	case shared.StepStateWaiting:
		return colorWaiting
	case shared.StepStateStalled:
		return colorStalled
	case shared.StepStateCompleted:
		return colorCompleted
	case shared.StepStateFailed:
		return colorFailed
	case shared.StepStateSkipped:
		return colorSkipped
	default:
		return colorLabel
	}
}

func (r *Renderer) getStateTag(state shared.StepState) string {
	switch state {
	case shared.StepStatePending:
		return "PEND"
	case shared.StepStateRunning:
		return "RUN "
	case shared.StepStateWaiting:
		return "WAIT"
	case shared.StepStateStalled:
		return "STAL"
	case shared.StepStateCompleted:
		return "DONE"
	case shared.StepStateFailed:
		return "FAIL"
	case shared.StepStateSkipped:
		return "SKIP"
	default:
		return "????"
	}
}

func (r *Renderer) getKindIcon(kind shared.StepKind) string {
	switch kind {
	case shared.StepKindLLMCall:
		return iconLLM
	case shared.StepKindFileRead:
		return iconFile
	case shared.StepKindFileWrite:
		return iconFile
	case shared.StepKindFileBuild:
		return iconFile
	case shared.StepKindToolExec:
		return iconTool
	case shared.StepKindValidation:
		return iconValidation
	case shared.StepKindContext:
		return iconContext
	case shared.StepKindNetwork:
		return iconNetwork
	case shared.StepKindUserInput:
		return iconUserInput
	case shared.StepKindInternal:
		return iconInternal
	default:
		return "·"
	}
}

func (r *Renderer) getPhaseIcon(phase shared.ProgressPhase) string {
	switch phase {
	case shared.PhaseInitializing:
		return iconPhaseInit
	case shared.PhasePlanning:
		return iconPhasePlan
	case shared.PhaseDescribing:
		return iconPhaseDescribe
	case shared.PhaseBuilding:
		return iconPhaseBuild
	case shared.PhaseApplying:
		return iconPhaseApply
	case shared.PhaseValidating:
		return iconPhaseValidate
	case shared.PhaseCompleted:
		return iconPhaseComplete
	case shared.PhaseFailed:
		return iconPhaseFailed
	case shared.PhaseStopped:
		return iconPhaseStopped
	default:
		return "·"
	}
}

func (r *Renderer) formatDuration(d time.Duration) string {
	if d < time.Second {
		return ""
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func (r *Renderer) getRecentCompletedSteps(steps []shared.ProgressStep, n int) []shared.ProgressStep {
	var completed []shared.ProgressStep
	for i := len(steps) - 1; i >= 0 && len(completed) < n; i-- {
		if steps[i].State.IsTerminal() {
			completed = append(completed, steps[i])
		}
	}
	// Reverse to show oldest first
	for i, j := 0, len(completed)-1; i < j; i, j = i+1, j-1 {
		completed[i], completed[j] = completed[j], completed[i]
	}
	return completed
}
