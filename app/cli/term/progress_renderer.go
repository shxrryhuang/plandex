package term

import (
	"fmt"
	"os"
	"strings"
	"time"

	shared "plandex-shared"

	"github.com/fatih/color"
)

// =============================================================================
// PROGRESS RENDERER
//
// Renders a shared.Progress snapshot into terminal-friendly output.
//
// TTY mode  — colour, inline spinners, compact single-line phase bar.
// Non-TTY   — plain text, timestamped lines, no ANSI codes.
//             Suitable for log files, CI pipelines, `> output.log`.
//
// The renderer never blocks or mutate state. It is a pure function from
// Progress → string (or direct stdout write for streaming updates).
// =============================================================================

// StallThreshold is how long a running step can go without a heartbeat
// before it is surfaced as "stalled" to the user.
const StallThreshold = 15 * time.Second

// ProgressRendererConfig controls output behaviour.
type ProgressRendererConfig struct {
	// If false, output is plain text without ANSI escape codes.
	ColorEnabled bool
	// Maximum width of the terminal (0 = auto-detect, fallback 80).
	Width int
}

// ProgressRenderer writes progress snapshots to stdout.
type ProgressRenderer struct {
	cfg     ProgressRendererConfig
	tty     bool
	lastLen int // track line count for in-place refresh (TTY only)
}

// NewProgressRenderer creates a renderer that auto-detects TTY and colour
// support from the current environment.
func NewProgressRenderer() *ProgressRenderer {
	isTTY := isTerminal()
	return &ProgressRenderer{
		cfg: ProgressRendererConfig{
			ColorEnabled: isTTY && !color.NoColor,
			Width:        detectWidth(),
		},
		tty: isTTY,
	}
}

// NewProgressRendererForTest creates a renderer with explicit settings,
// useful for testing or piped output.
func NewProgressRendererForTest(colorEnabled bool, width int) *ProgressRenderer {
	return &ProgressRenderer{
		cfg: ProgressRendererConfig{
			ColorEnabled: colorEnabled,
			Width:        width,
		},
		tty: colorEnabled, // treat colour-on as TTY for rendering decisions
	}
}

// Render writes the current progress to stdout. In TTY mode it overwrites
// the previous progress block. In non-TTY mode it appends timestamped lines.
func (r *ProgressRenderer) Render(p *shared.Progress) {
	lines := r.Lines(p)
	if r.tty {
		r.renderTTY(lines)
	} else {
		r.renderPlain(p, lines)
	}
}

// Lines produces the rendered output as a slice of strings (one per line),
// without trailing newlines. Callers that need direct string access (e.g.
// tests or custom outputs) use this instead of Render.
func (r *ProgressRenderer) Lines(p *shared.Progress) []string {
	var out []string

	// --- Phase bar (top-level overview) ------------------------------------
	out = append(out, r.phaseBar(p))

	// --- Active steps (running / stalled) ----------------------------------
	active := p.ActiveSteps()
	if len(active) > 0 {
		for _, s := range active {
			out = append(out, r.stepLine(s, p))
		}
	}

	// --- Recent completions (last 3 to keep noise low) ---------------------
	completed := p.CompletedSteps()
	recentN := 3
	start := len(completed) - recentN
	if start < 0 {
		start = 0
	}
	for _, s := range completed[start:] {
		out = append(out, r.stepLine(s, p))
	}

	// --- Failures (always shown in full) -----------------------------------
	for _, s := range p.FailedSteps() {
		out = append(out, r.stepLine(s, p))
	}

	// --- Stall warning -----------------------------------------------------
	if r.hasStallWarning(p) {
		out = append(out, r.stallWarning(p))
	}

	// --- Summary footer ----------------------------------------------------
	out = append(out, r.summaryLine(p))

	return out
}

// renderTTY writes lines and overwrites the previous block using cursor
// movement. This keeps the progress output in-place during a run.
func (r *ProgressRenderer) renderTTY(lines []string) {
	// Move cursor up to overwrite previous output
	if r.lastLen > 0 {
		fmt.Printf("\033[%dA", r.lastLen) // move up
		fmt.Print("\033[J")               // clear from cursor to end of screen
	}

	for _, line := range lines {
		fmt.Println(line)
	}
	r.lastLen = len(lines)
}

// renderPlain writes timestamped lines without overwriting. Each call
// appends to the output stream, making logs fully replayable.
func (r *ProgressRenderer) renderPlain(p *shared.Progress, lines []string) {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	for _, line := range lines {
		plain := stripANSI(line)
		fmt.Printf("[%s] %s\n", ts, plain)
	}
}

// =============================================================================
// PHASE BAR
// Shows which phase is active and which have completed.
//
//   [✓ connect] [✓ context] [▶ model] [  build] [  apply] [  finalize]
//
// Completed phases are bold+green. Active is bold+cyan with a marker.
// Pending phases are dim.
// =============================================================================

func (r *ProgressRenderer) phaseBar(p *shared.Progress) string {
	var segments []string

	completedPhases := r.completedPhases(p)

	for _, phase := range shared.AllPhases {
		label := phase.PhaseLabel()

		switch {
		case completedPhases[phase]:
			segments = append(segments, r.colorize(fmt.Sprintf("[✓ %s]", label), ColorHiGreen, true))
		case phase == p.ActivePhase:
			segments = append(segments, r.colorize(fmt.Sprintf("[▶ %s]", label), ColorHiCyan, true))
		default:
			segments = append(segments, r.dim(fmt.Sprintf("[  %s]", label)))
		}
	}

	return strings.Join(segments, " ")
}

// completedPhases determines which phases have all their steps in a terminal
// state. A phase with zero steps is not considered completed (it hasn't run).
// A single non-terminal step is enough to keep the phase marked incomplete,
// regardless of the order steps were appended.
func (r *ProgressRenderer) completedPhases(p *shared.Progress) map[shared.PhaseID]bool {
	hasSteps := make(map[shared.PhaseID]bool)
	hasNonTerminal := make(map[shared.PhaseID]bool)

	for _, s := range p.Steps {
		hasSteps[s.Phase] = true
		if !s.Status.IsTerminal() {
			hasNonTerminal[s.Phase] = true
		}
	}

	result := make(map[shared.PhaseID]bool, len(hasSteps))
	for phase := range hasSteps {
		if !hasNonTerminal[phase] {
			result[phase] = true
		}
	}
	return result
}

// =============================================================================
// STEP LINE
// Renders a single step with status icon, label, detail, and duration.
//
//   ⬛ [running ]  model › sending prompt          5s
//   ✅ [completed]  context › loaded auth.go        120ms
//   ❌ [failed   ]  build › src/main.go             API timeout
//   ⚠️  [stalled  ]  apply › running tests          12s (no heartbeat)
// =============================================================================

func (r *ProgressRenderer) stepLine(s *shared.Step, p *shared.Progress) string {
	icon := r.statusIcon(s.Status)
	badge := r.statusBadge(s.Status)

	label := s.Label
	if s.Detail != "" {
		label += " › " + s.Detail
	}

	// Truncate label to fit width
	maxLabelWidth := r.cfg.Width - 20
	if maxLabelWidth < 20 {
		maxLabelWidth = 20
	}
	if len(label) > maxLabelWidth {
		label = label[:maxLabelWidth-1] + "…"
	}

	dur := r.durationStr(s)

	// Pad for alignment
	labelPadded := label + strings.Repeat(" ", max(0, maxLabelWidth-len(label)))

	return fmt.Sprintf("  %s %s  %s  %s", icon, badge, labelPadded, dur)
}

func (r *ProgressRenderer) statusIcon(status shared.StepStatus) string {
	switch status {
	case shared.StepCompleted:
		return r.colorize("✓", ColorHiGreen, false)
	case shared.StepFailed:
		return r.colorize("✕", ColorHiRed, false)
	case shared.StepRunning:
		return r.colorize("▸", ColorHiCyan, false)
	case shared.StepStalled:
		return r.colorize("⚠", ColorHiYellow, false)
	case shared.StepSkipped:
		return r.colorize("◌", ColorHiYellow, false)
	default:
		return r.dim("·")
	}
}

func (r *ProgressRenderer) statusBadge(status shared.StepStatus) string {
	text := fmt.Sprintf("[%-9s]", string(status))
	switch status {
	case shared.StepCompleted:
		return r.colorize(text, ColorHiGreen, false)
	case shared.StepFailed:
		return r.colorize(text, ColorHiRed, false)
	case shared.StepRunning:
		return r.colorize(text, ColorHiCyan, false)
	case shared.StepStalled:
		return r.colorize(text, ColorHiYellow, false)
	default:
		return r.dim(text)
	}
}

func (r *ProgressRenderer) durationStr(s *shared.Step) string {
	if s.Status == shared.StepPending {
		return ""
	}
	d := s.Duration()
	if d < time.Millisecond {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		sec := int(d.Seconds()) - m*60
		if sec == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, sec)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) - h*60
	return fmt.Sprintf("%dh%dm", h, m)
}

// =============================================================================
// STALL WARNING
// Surfaced when a running step has no recent heartbeat. Tells the user
// the system may be waiting on an external call.
// =============================================================================

func (r *ProgressRenderer) hasStallWarning(p *shared.Progress) bool {
	for _, s := range p.Steps {
		if s.Status == shared.StepStalled {
			return true
		}
	}
	return false
}

func (r *ProgressRenderer) stallWarning(p *shared.Progress) string {
	msg := "  No server heartbeat for >15s — the operation may be waiting on an external service."
	return r.colorize(msg, ColorHiYellow, false)
}

// =============================================================================
// SUMMARY FOOTER
// One-line status that gives a quick read on the overall run.
//   [model] 4 done | 1 running | 0 failed (8s elapsed)
// =============================================================================

func (r *ProgressRenderer) summaryLine(p *shared.Progress) string {
	summary := p.Summary()
	if p.Finished && p.Error == "" {
		return r.colorize("  "+summary, ColorHiGreen, true)
	}
	if p.Error != "" {
		return r.colorize("  "+summary+" — "+p.Error, ColorHiRed, true)
	}
	return r.dim("  " + summary)
}

// =============================================================================
// COLOUR / FORMATTING HELPERS
// =============================================================================

func (r *ProgressRenderer) colorize(s string, clr color.Attribute, bold bool) string {
	if !r.cfg.ColorEnabled {
		return s
	}
	attrs := []color.Attribute{clr}
	if bold {
		attrs = append(attrs, color.Bold)
	}
	c := color.New(attrs...)
	c.EnableColor() // override global NoColor for explicit opt-in
	return c.Sprint(s)
}

func (r *ProgressRenderer) dim(s string) string {
	if !r.cfg.ColorEnabled {
		return s
	}
	c := color.New(color.Faint)
	c.EnableColor()
	return c.Sprint(s)
}

// isTerminal checks whether stdout is a terminal.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// detectWidth returns terminal width or 80 as fallback.
func detectWidth() int {
	// Best effort; in a real implementation this would use
	// golang.org/x/term or syscall TIOCGWINSZ.
	if w := os.Getenv("COLUMNS"); w != "" {
		var width int
		if _, err := fmt.Sscanf(w, "%d", &width); err == nil && width > 0 {
			return width
		}
	}
	return 80
}

// stripANSI removes ANSI escape sequences from a string.
// Used when rendering plain (non-TTY) output.
// A malformed sequence that never terminates with a letter is abandoned after
// 32 bytes so it cannot swallow the rest of the input.
func stripANSI(s string) string {
	const maxEscLen = 32
	var b strings.Builder
	inEscape := false
	escLen := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			escLen = 0
			continue
		}
		if inEscape {
			escLen++
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
				continue
			}
			if escLen >= maxEscLen {
				// Malformed or truncated sequence — stop consuming.
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
