package streamtui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"plandex-cli/term"

	shared "plandex-shared"

	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
)

// Progress view styling
var (
	progressPhaseStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fff"))
	progressTimeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	progressRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0ff"))
	progressDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f0"))
	progressFailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f00"))
	progressWaitStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0"))
	progressStallStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f00")).Bold(true)
	progressPendStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	progressHelpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	progressWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0")).Bold(true)
)

var borderColor = lipgloss.Color("#444")
var helpTextColor = lipgloss.Color("#ddd")

func (m streamUIModel) View() string {

	if m.promptingMissingFile {
		return m.renderMissingFilePrompt()
	}

	views := []string{}
	if !m.buildOnly {
		views = append(views, m.renderMainView())
	}

	// Show progress view if enabled, otherwise show classic processing/build views
	if m.showProgressView && m.progressReport != nil {
		views = append(views, m.renderProgressView())
	} else {
		if m.processing || m.starting {
			views = append(views, m.renderProcessing())
		}
		if m.building {
			views = append(views, m.renderBuild())
		}
	}

	views = append(views, m.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

func (m streamUIModel) renderMainView() string {
	return m.mainViewport.View()
}

func (m streamUIModel) renderHelp() string {
	style := lipgloss.NewStyle().Width(m.width).Foreground(lipgloss.Color(helpTextColor)).BorderStyle(lipgloss.NormalBorder()).BorderTop(true).BorderForeground(lipgloss.Color(borderColor))

	if m.buildOnly {
		s := " (s)top"
		if m.canSendToBg {
			s += " • (b)ackground"
		}
		s += " • (p)rogress"
		return style.Render(s)
	} else {
		s := " (s)top"
		if m.canSendToBg {
			s += " • (b)ackground"
		}
		s += " • (j/k) scroll • (d/u) page • (g/G) start/end • (p)rogress"
		return style.Render(s)
	}
}

func (m streamUIModel) renderProcessing() string {
	if m.starting || m.processing {
		return "\n " + m.spinner.View()
	} else {
		return ""
	}
}

func (m streamUIModel) renderBuild() string {
	return m.doRenderBuild(false)
}

func (m streamUIModel) renderStaticBuild() string {
	return m.doRenderBuild(true)
}

func (m streamUIModel) doRenderBuild(outputStatic bool) string {
	if !m.building && !outputStatic {
		return ""
	}

	if outputStatic && len(m.finishedByPath) == 0 && len(m.tokensByPath) == 0 {
		return ""
	}

	var style lipgloss.Style
	if m.buildOnly {
		style = lipgloss.NewStyle().Width(m.width)
	} else {
		style = lipgloss.NewStyle().Width(m.width).BorderStyle(lipgloss.NormalBorder()).BorderTop(true).BorderForeground(lipgloss.Color(borderColor))
	}

	if !outputStatic && m.buildViewCollapsed {
		// Render collapsed view
		inProgress := 0
		total := len(m.tokensByPath)
		for path := range m.tokensByPath {
			if path == "_apply.sh" {
				total--
				continue
			}
			if !m.finishedByPath[path] {
				inProgress++
			}
		}

		_, hasApplyScript := m.tokensByPath["_apply.sh"]
		applyScriptFinished := m.finishedByPath["_apply.sh"]

		lbl := "file"
		if total > 1 {
			lbl = "files"
		}

		var summary string
		if total > 0 {
			summary = fmt.Sprintf(" 📄 %d %s", total, lbl)
		}
		if inProgress > 0 {
			summary += fmt.Sprintf(" • 📝 editing %d %s", inProgress, m.buildSpinner.View())
		}
		if hasApplyScript {
			if total > 0 {
				summary += " •"
			}
			if applyScriptFinished {
				summary += " 🚀 wrote commands"
			} else {
				summary += fmt.Sprintf(" 🚀 editing commands %s", m.buildSpinner.View())
			}
		}
		head := m.getBuildHeader(outputStatic)
		return style.Render(lipgloss.JoinVertical(lipgloss.Left, head, summary))
	}

	resRows := m.getRows(outputStatic)

	res := style.Render(strings.Join(resRows, "\n"))

	return res
}

func (m streamUIModel) didBuild() bool {
	return !(m.stopped || m.err != nil || m.apiErr != nil)
}

func (m streamUIModel) getBuildHeader(static bool) string {
	lbl := "Building plan "
	bgColor := color.BgGreen
	if static {
		if !m.didBuild() {
			lbl = "Build incomplete "
			bgColor = color.BgRed
		} else {
			lbl = "Built plan "
		}
	}

	head := color.New(bgColor, color.FgHiWhite, color.Bold).Sprint(" 🏗  ") + color.New(bgColor, color.FgHiWhite).Sprint(lbl)

	// Add collapse/expand hint
	var hint string
	if !static {
		hint = "(↓) collapse"
		if m.buildViewCollapsed {
			hint = "(↑) expand"
		}
	}
	padding := m.width - lipgloss.Width(head) - lipgloss.Width(hint) - 1 // 1 for space
	if padding > 0 {
		head += strings.Repeat(" ", padding) + hint
	}

	return head
}

func (m streamUIModel) getRows(static bool) []string {
	built := m.didBuild() && static
	head := m.getBuildHeader(static)

	// Gather file paths, _apply.sh last
	filePaths := make([]string, 0, len(m.tokensByPath))
	for filePath := range m.tokensByPath {
		if filePath == "_apply.sh" {
			continue
		}
		filePaths = append(filePaths, filePath)
	}
	sort.Strings(filePaths)
	if _, ok := m.tokensByPath["_apply.sh"]; ok {
		filePaths = append(filePaths, "_apply.sh")
	}

	var rows [][]string
	lineWidth := 0
	lineNum := -1
	rowIdx := 0

	for _, filePath := range filePaths {
		tokens := m.tokensByPath[filePath]
		finished := m.finished || m.finishedByPath[filePath] || built
		removed := m.removedByPath[filePath]

		// Basic block label
		icon := "📄"
		label := filePath
		if filePath == "_apply.sh" {
			icon = "🚀"
			label = "commands"
		}
		block := fmt.Sprintf("%s %s", icon, label)

		// Mark removed/finished/tokens
		switch {
		case removed:
			block += " ❌"
		case finished:
			block += " ✅"
		case tokens > 0:
			block += fmt.Sprintf(" %d 🪙", tokens)
		default:
			block += " " + m.buildSpinner.View()
		}

		// Truncate if needed
		blockWidth := lipgloss.Width(block)
		if blockWidth > m.width {
			maxWidth := m.width - lipgloss.Width("⋯")
			if maxWidth < 4 {
				block = string([]rune(block)[0:1]) + "⋯"
			} else {
				half := maxWidth / 2
				runes := []rune(block)
				block = string(runes[:half]) + "⋯" + string(runes[len(runes)-half:])
			}
		}

		// Build the "prefix + block" text tentatively:
		prefix := ""
		if rowIdx > 0 {
			prefix = " | "
		}
		candidate := prefix + block
		candidateWidth := lipgloss.Width(candidate)

		// Check if we have no row or it won't fit with the prefix
		if lineNum == -1 || lineWidth+candidateWidth > m.width {
			// Start a new row
			rows = append(rows, []string{})
			lineNum++
			rowIdx = 0
			lineWidth = 0

			// In a new row, there's no prefix
			candidate = block
			candidateWidth = lipgloss.Width(candidate)
		}

		rows[lineNum] = append(rows[lineNum], candidate)
		lineWidth += candidateWidth
		rowIdx++
	}

	// If empty row left at the end, strip it
	if len(rows) > 0 && len(rows[len(rows)-1]) == 0 {
		rows = rows[:len(rows)-1]
	}

	// Final output lines
	resRows := make([]string, len(rows)+1)
	resRows[0] = head
	for i, row := range rows {
		resRows[i+1] = lipgloss.JoinHorizontal(lipgloss.Left, row...)
	}

	return resRows
}

func (m streamUIModel) renderMissingFilePrompt() string {
	style := lipgloss.NewStyle().Padding(1).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(borderColor)).Width(m.width - 2).Height(m.height - 2)

	prompt := "📄 " + color.New(color.Bold, term.ColorHiYellow).Sprint(m.missingFilePath) + " isn't in context."

	prompt += "\n\n"

	desc := "This file exists in your project, but isn't loaded into context. Unless you load it into context or skip generating it, Plandex will fully overwrite the existing file rather than applying updates."

	words := strings.Split(desc, " ")
	for i, word := range words {
		words[i] = color.New(color.FgWhite).Sprint(word)
	}

	prompt += strings.Join(words, " ")

	prompt += "\n\n" + color.New(term.ColorHiMagenta, color.Bold).Sprintln("🧐 What do you want to do?")

	for i, opt := range missingFileSelectOpts {
		if i == m.missingFileSelectedIdx {
			prompt += color.New(term.ColorHiCyan, color.Bold).Sprint(" > " + opt)
		} else {
			prompt += "   " + opt
		}

		if opt == MissingFileLoadLabel {
			prompt += fmt.Sprintf(" | %d 🪙", m.missingFileTokens)
		}

		prompt += "\n"
	}

	return style.Render(prompt)
}

// renderProgressView renders the new progress tracking view
func (m streamUIModel) renderProgressView() string {
	if m.progressReport == nil {
		return ""
	}

	report := m.progressReport
	var b strings.Builder

	// Phase header with duration
	phaseIcon := m.getPhaseIcon(report.Phase)
	duration := time.Since(report.StartedAt)
	durationStr := formatDuration(duration)

	b.WriteString(progressPhaseStyle.Render(fmt.Sprintf(" %s %s", phaseIcon, report.PhaseLabel)))
	if durationStr != "" {
		b.WriteString(progressTimeStyle.Render(fmt.Sprintf(" [%s]", durationStr)))
	}
	b.WriteString("\n")

	// Progress bar if we have steps
	if report.TotalSteps > 0 {
		barWidth := m.width - 15
		if barWidth < 10 {
			barWidth = 10
		}
		progress := float64(report.CompletedSteps) / float64(report.TotalSteps)
		filled := int(progress * float64(barWidth))
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		pct := fmt.Sprintf("%3.0f%%", progress*100)
		b.WriteString(fmt.Sprintf("  [%s] %s\n", bar, pct))
	}

	// Current step with spinner
	if report.CurrentStepID != "" {
		for _, step := range report.Steps {
			if step.ID == report.CurrentStepID {
				b.WriteString(m.renderProgressStep(&step, true))
				break
			}
		}
	}

	// Recent completed steps (last 3)
	completed := m.getRecentCompletedSteps(3)
	for _, step := range completed {
		b.WriteString(m.renderProgressStep(&step, false))
	}

	// Warnings
	for _, warning := range report.Warnings {
		b.WriteString(progressWarnStyle.Render(fmt.Sprintf("⚠ %s\n", warning)))
	}

	// Stall warnings
	if len(report.StalledIDs) > 0 {
		b.WriteString(progressStallStyle.Render("⚠ Operation may be stalled\n"))
	}

	// Suggested action
	if report.SuggestedAction != "" {
		b.WriteString(progressHelpStyle.Render(fmt.Sprintf("💡 %s\n", report.SuggestedAction)))
	}

	style := lipgloss.NewStyle().Width(m.width).BorderStyle(lipgloss.NormalBorder()).BorderTop(true).BorderForeground(lipgloss.Color(borderColor))
	return style.Render(b.String())
}

// renderProgressStep renders a single step in the progress view
func (m streamUIModel) renderProgressStep(step *shared.Step, isCurrent bool) string {
	var b strings.Builder

	// State icon with spinner for running steps
	stateIcon := m.getStateIcon(step.State)
	if isCurrent && (step.State == shared.StepStateRunning || step.State == shared.StepStateWaiting) {
		stateIcon = m.spinner.View()
	}

	// Get style based on state
	var stateStyle lipgloss.Style
	switch step.State {
	case shared.StepStateRunning:
		stateStyle = progressRunningStyle
	case shared.StepStateCompleted:
		stateStyle = progressDoneStyle
	case shared.StepStateFailed:
		stateStyle = progressFailStyle
	case shared.StepStateWaiting:
		stateStyle = progressWaitStyle
	case shared.StepStateStalled:
		stateStyle = progressStallStyle
	default:
		stateStyle = progressPendStyle
	}

	// Kind icon
	kindIcon := m.getKindIcon(step.Kind)

	b.WriteString(stateStyle.Render(stateIcon))
	b.WriteString(" ")
	b.WriteString(kindIcon)
	b.WriteString(" ")
	b.WriteString(step.Label)

	if step.Detail != "" {
		b.WriteString(progressTimeStyle.Render(fmt.Sprintf(" (%s)", step.Detail)))
	}

	// Token count
	if step.TokensProcessed > 0 {
		b.WriteString(progressTimeStyle.Render(fmt.Sprintf(" %d🪙", step.TokensProcessed)))
	}

	// Duration for completed steps
	if step.State.IsTerminal() || step.Duration() > 5*time.Second {
		if dur := formatDuration(step.Duration()); dur != "" {
			b.WriteString(progressTimeStyle.Render(fmt.Sprintf(" %s", dur)))
		}
	}

	// Error
	if step.Error != "" {
		b.WriteString(progressFailStyle.Render(fmt.Sprintf(" [%s]", step.Error)))
	}

	b.WriteString("\n")
	return b.String()
}

// getRecentCompletedSteps returns the most recent completed steps
func (m streamUIModel) getRecentCompletedSteps(n int) []shared.Step {
	if m.progressReport == nil {
		return nil
	}

	var completed []shared.Step
	for i := len(m.progressReport.Steps) - 1; i >= 0 && len(completed) < n; i-- {
		step := m.progressReport.Steps[i]
		if step.State.IsTerminal() && step.ID != m.progressReport.CurrentStepID {
			completed = append(completed, step)
		}
	}

	// Reverse to show oldest first
	for i, j := 0, len(completed)-1; i < j; i, j = i+1, j-1 {
		completed[i], completed[j] = completed[j], completed[i]
	}
	return completed
}

// getPhaseIcon returns an icon for the execution phase
func (m streamUIModel) getPhaseIcon(phase shared.ProgressPhase) string {
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
		return "⏹"
	default:
		return "·"
	}
}

// getStateIcon returns an icon for the step state
func (m streamUIModel) getStateIcon(state shared.StepState) string {
	switch state {
	case shared.StepStatePending:
		return "○"
	case shared.StepStateRunning:
		return "◐"
	case shared.StepStateWaiting:
		return "◔"
	case shared.StepStateStalled:
		return "⚠"
	case shared.StepStateCompleted:
		return "●"
	case shared.StepStateFailed:
		return "✗"
	case shared.StepStateSkipped:
		return "⊘"
	default:
		return "?"
	}
}

// getKindIcon returns an icon for the step kind
func (m streamUIModel) getKindIcon(kind shared.StepKind) string {
	switch kind {
	case shared.StepKindLLMCall:
		return "🤖"
	case shared.StepKindFileRead, shared.StepKindFileWrite, shared.StepKindFileBuild:
		return "📄"
	case shared.StepKindToolExec:
		return "🔧"
	case shared.StepKindValidation:
		return "✓"
	case shared.StepKindContext:
		return "📚"
	case shared.StepKindNetwork:
		return "🌐"
	case shared.StepKindUserInput:
		return "⌨"
	case shared.StepKindInternal:
		return "⚙"
	default:
		return "·"
	}
}

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
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
