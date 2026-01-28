package term

import (
	"strings"
	"testing"
	"time"

	shared "plandex-shared"
)

func TestRenderer_PhaseBar_AllPending(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	lines := r.Lines(p)
	bar := lines[0]

	// Active phase (connect) should have marker
	if !strings.Contains(bar, "[▶ connect]") {
		t.Errorf("expected active connect marker in phase bar, got: %s", bar)
	}
	// Others should be pending (dim brackets)
	if !strings.Contains(bar, "[  model]") {
		t.Errorf("expected pending model in phase bar, got: %s", bar)
	}
}

func TestRenderer_PhaseBar_CompletedPhases(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	// Complete the connect phase
	s := p.AddStep(shared.PhaseConnect, "connect", "")
	p.StartStep(s)
	p.CompleteStep(s)
	p.ActivePhase = shared.PhaseModel

	lines := r.Lines(p)
	bar := lines[0]

	if !strings.Contains(bar, "[✓ connect]") {
		t.Errorf("expected completed connect marker, got: %s", bar)
	}
	if !strings.Contains(bar, "[▶ model]") {
		t.Errorf("expected active model marker, got: %s", bar)
	}
}

func TestRenderer_StepLine_Running(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	s := p.AddStep(shared.PhaseModel, "model", "streaming reply")
	p.StartStep(s)

	lines := r.Lines(p)

	found := false
	for _, line := range lines {
		if strings.Contains(line, "model") && strings.Contains(line, "streaming reply") && strings.Contains(line, "running") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a running model step line, got lines: %v", lines)
	}
}

func TestRenderer_StepLine_Failed(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	s := p.AddStep(shared.PhaseModel, "model", "")
	p.StartStep(s)
	p.FailStep(s, "connection reset")

	lines := r.Lines(p)

	found := false
	for _, line := range lines {
		if strings.Contains(line, "failed") && strings.Contains(line, "model") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a failed step line, got lines: %v", lines)
	}
}

func TestRenderer_StepLine_Stalled(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	s := p.AddStep(shared.PhaseModel, "model", "waiting for API")
	p.StartStep(s)
	s.Status = shared.StepStalled

	lines := r.Lines(p)

	foundStall := false
	foundWarning := false
	for _, line := range lines {
		if strings.Contains(line, "stalled") {
			foundStall = true
		}
		if strings.Contains(line, "heartbeat") {
			foundWarning = true
		}
	}
	if !foundStall {
		t.Errorf("expected stalled step line, got: %v", lines)
	}
	if !foundWarning {
		t.Errorf("expected heartbeat warning line, got: %v", lines)
	}
}

func TestRenderer_SummaryLine_Normal(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	s1 := p.AddStep(shared.PhaseConnect, "connect", "")
	p.StartStep(s1)
	p.CompleteStep(s1)

	s2 := p.AddStep(shared.PhaseModel, "model", "")
	p.StartStep(s2)

	lines := r.Lines(p)
	summary := lines[len(lines)-1]

	if !strings.Contains(summary, "1 done") {
		t.Errorf("expected '1 done' in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "1 running") {
		t.Errorf("expected '1 running' in summary, got: %s", summary)
	}
}

func TestRenderer_SummaryLine_Finished(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	s := p.AddStep(shared.PhaseConnect, "connect", "")
	p.StartStep(s)
	p.CompleteStep(s)
	p.Finished = true

	lines := r.Lines(p)
	summary := lines[len(lines)-1]

	if !strings.Contains(summary, "1 done") {
		t.Errorf("expected '1 done' in finished summary, got: %s", summary)
	}
}

func TestRenderer_SummaryLine_Error(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	s := p.AddStep(shared.PhaseModel, "model", "")
	p.StartStep(s)
	p.FailStep(s, "rate limited")
	p.Finished = true

	lines := r.Lines(p)
	summary := lines[len(lines)-1]

	if !strings.Contains(summary, "rate limited") {
		t.Errorf("expected error message in summary, got: %s", summary)
	}
}

func TestRenderer_NoColorOutput(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	p := shared.NewProgress()
	p.AddStep(shared.PhaseConnect, "connect", "test")

	lines := r.Lines(p)
	for _, line := range lines {
		if strings.Contains(line, "\033[") {
			t.Errorf("expected no ANSI codes in non-color mode, got: %q", line)
		}
	}
}

func TestRenderer_ColorOutput(t *testing.T) {
	r := NewProgressRendererForTest(true, 100)
	p := shared.NewProgress()
	s := p.AddStep(shared.PhaseConnect, "connect", "")
	p.StartStep(s)
	p.CompleteStep(s)
	p.ActivePhase = shared.PhaseModel

	lines := r.Lines(p)
	hasColor := false
	for _, line := range lines {
		if strings.Contains(line, "\033[") {
			hasColor = true
			break
		}
	}
	if !hasColor {
		t.Error("expected ANSI color codes in color mode")
	}
}

func TestRenderer_LabelTruncation(t *testing.T) {
	r := NewProgressRendererForTest(false, 40) // Narrow terminal

	p := shared.NewProgress()
	longLabel := strings.Repeat("x", 80)
	s := p.AddStep(shared.PhaseModel, "model", longLabel)
	p.StartStep(s)

	lines := r.Lines(p)
	for _, line := range lines {
		if len(line) > 80 {
			// The line should be truncated for narrow terminals.
			// We allow some slack for the status badge/icon.
			if !strings.Contains(line, "…") {
				t.Errorf("expected truncation marker in narrow terminal, got: %q", line)
			}
		}
	}
}

func TestRenderer_DurationStr_Seconds(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	start := time.Now().Add(-5 * time.Second)
	s := &shared.Step{
		StartedAt: &start,
		Status:    shared.StepRunning,
	}

	dur := r.durationStr(s)
	if dur != "5s" {
		t.Errorf("expected '5s', got %q", dur)
	}
}

func TestRenderer_DurationStr_Minutes(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	start := time.Now().Add(-2*time.Minute - 30*time.Second)
	end := time.Now()
	s := &shared.Step{
		StartedAt:  &start,
		FinishedAt: &end,
		Status:     shared.StepCompleted,
	}

	dur := r.durationStr(s)
	if dur != "2m30s" {
		t.Errorf("expected '2m30s', got %q", dur)
	}
}

func TestStripANSI(t *testing.T) {
	input := "\033[1m\033[32mhello\033[0m world"
	expected := "hello world"
	result := stripANSI(input)
	if result != expected {
		t.Errorf("stripANSI(%q) = %q, want %q", input, result, expected)
	}
}

func TestStripANSI_TruncatedSequence(t *testing.T) {
	// A truncated escape (no terminating letter) must not swallow
	// everything that follows after the max scan window.
	input := "\033[999 this is visible"
	result := stripANSI(input)
	// After the 32-byte window abandons the escape, "visible" must survive.
	if !strings.Contains(result, "visible") {
		t.Errorf("truncated escape ate trailing text: stripANSI(%q) = %q", input, result)
	}
}

func TestRenderer_MultipleBuilds(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	// Simulate 3 concurrent build steps
	files := []string{"main.go", "util.go", "auth.go"}
	for _, f := range files {
		s := p.AddStep(shared.PhaseBuild, "build", f)
		p.StartStep(s)
	}
	p.ActivePhase = shared.PhaseBuild

	lines := r.Lines(p)

	for _, f := range files {
		found := false
		for _, line := range lines {
			if strings.Contains(line, f) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file %s to appear in output", f)
		}
	}
}

// --- Empty / nil edge cases ---------------------------------------------------

func TestRenderer_EmptyProgress(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	p := shared.NewProgress()

	lines := r.Lines(p)
	// Must produce at least a phase bar and a summary — never empty output
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines for empty progress, got %d", len(lines))
	}
	// Phase bar must still render with active connect marker
	if !strings.Contains(lines[0], "[▶ connect]") {
		t.Errorf("expected active connect in phase bar for empty progress, got: %s", lines[0])
	}
}

func TestRenderer_UnknownActivePhase(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	p := shared.NewProgress()
	p.ActivePhase = shared.PhaseID("unknown_phase")

	// Must not panic; phase bar renders known phases with dim markers
	lines := r.Lines(p)
	if len(lines) == 0 {
		t.Error("expected non-empty output for unknown active phase")
	}
	// No phase should have the active marker since none matches
	for _, phase := range shared.AllPhases {
		marker := "[▶ " + phase.PhaseLabel() + "]"
		if strings.Contains(lines[0], marker) {
			t.Errorf("no known phase should be marked active for unknown ActivePhase, found %s", marker)
		}
	}
}

func TestRenderer_StepLine_EmptyLabelAndDetail(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	p := shared.NewProgress()
	s := p.AddStep(shared.PhaseModel, "", "")
	p.StartStep(s)

	// Must not panic on empty strings
	lines := r.Lines(p)
	found := false
	for _, line := range lines {
		if strings.Contains(line, "running") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a running step line even with empty label/detail, got: %v", lines)
	}
}

func TestRenderer_StepLine_PendingStep(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	p := shared.NewProgress()
	p.AddStep(shared.PhaseModel, "waiting", "not started yet")

	lines := r.Lines(p)
	// Pending steps are not in active or completed lists, so they
	// should NOT appear as individual step lines (only in summary count)
	for _, line := range lines {
		if strings.Contains(line, "waiting") && strings.Contains(line, "not started yet") {
			// Pending steps should not be rendered as step lines
			t.Errorf("pending step should not appear as a step line, got: %s", line)
		}
	}
	// But summary should note them
	summary := lines[len(lines)-1]
	if !strings.Contains(summary, "1 pending") {
		t.Errorf("expected '1 pending' in summary for pending step, got: %s", summary)
	}
}

// --- Duration edge cases -------------------------------------------------------

func TestRenderer_DurationStr_SubMillisecond(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	// A step that just started — duration < 1ms
	now := time.Now()
	s := &shared.Step{
		StartedAt: &now,
		Status:    shared.StepRunning,
	}
	dur := r.durationStr(s)
	// Sub-millisecond returns empty string
	if dur != "" {
		t.Errorf("expected empty duration for sub-ms step, got %q", dur)
	}
}

func TestRenderer_DurationStr_ExactlyOneHour(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	start := time.Now().Add(-1 * time.Hour)
	end := time.Now()
	s := &shared.Step{
		StartedAt:  &start,
		FinishedAt: &end,
		Status:     shared.StepCompleted,
	}
	dur := r.durationStr(s)
	if dur != "1h0m" {
		t.Errorf("expected '1h0m' for exactly one hour, got %q", dur)
	}
}

// --- Summary edge cases --------------------------------------------------------

func TestRenderer_SummaryLine_FinishedWithError(t *testing.T) {
	r := NewProgressRendererForTest(false, 100)
	p := shared.NewProgress()
	s := p.AddStep(shared.PhaseModel, "model", "")
	p.StartStep(s)
	p.FailStep(s, "quota exceeded")
	p.Finished = true
	p.Error = "quota exceeded"

	lines := r.Lines(p)
	summary := lines[len(lines)-1]

	// Error message must appear in summary footer
	if !strings.Contains(summary, "quota exceeded") {
		t.Errorf("expected error in finished summary, got: %s", summary)
	}
}

func TestRenderer_AllPhasesCompleted(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	for _, phase := range shared.AllPhases {
		s := p.AddStep(phase, string(phase), "")
		p.StartStep(s)
		p.CompleteStep(s)
	}
	p.ActivePhase = shared.PhaseFinalize

	lines := r.Lines(p)
	bar := lines[0]

	// Every phase should show the completed checkmark
	for _, phase := range shared.AllPhases {
		marker := "[✓ " + phase.PhaseLabel() + "]"
		if !strings.Contains(bar, marker) {
			t.Errorf("expected completed marker for %s, got bar: %s", phase, bar)
		}
	}
}

func TestRenderer_CompletedPhases_MixedTerminalStatuses(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	// Phase with one completed + one failed step — both terminal
	s1 := p.AddStep(shared.PhaseModel, "m1", "")
	p.StartStep(s1)
	p.CompleteStep(s1)

	s2 := p.AddStep(shared.PhaseModel, "m2", "")
	p.StartStep(s2)
	p.FailStep(s2, "oops")

	p.ActivePhase = shared.PhaseModel

	lines := r.Lines(p)
	bar := lines[0]

	// Both steps terminal → phase shows as completed in the bar
	if !strings.Contains(bar, "[✓ model]") {
		t.Errorf("expected model completed when all steps are terminal, got: %s", bar)
	}
}

// --- Width edge cases ----------------------------------------------------------

func TestRenderer_WideTerminal_NoTruncation(t *testing.T) {
	r := NewProgressRendererForTest(false, 200)
	p := shared.NewProgress()
	label := strings.Repeat("a", 50)
	s := p.AddStep(shared.PhaseModel, "model", label)
	p.StartStep(s)

	lines := r.Lines(p)
	found := false
	for _, line := range lines {
		if strings.Contains(line, label) {
			found = true
			// Should NOT be truncated
			if strings.Contains(line, "…") {
				t.Errorf("label should not be truncated at width=200, got: %s", line)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected full label in wide terminal output")
	}
}

func TestRenderer_ZeroWidth(t *testing.T) {
	// Width 0 should use fallback (80) and not panic
	r := NewProgressRendererForTest(false, 0)
	p := shared.NewProgress()
	s := p.AddStep(shared.PhaseModel, "model", "detail")
	p.StartStep(s)

	// Must not panic
	lines := r.Lines(p)
	if len(lines) == 0 {
		t.Error("expected non-empty output at zero width")
	}
}

// --- Stall rendering -----------------------------------------------------------

func TestRenderer_MultipleStalled_SingleWarning(t *testing.T) {
	r := NewProgressRendererForTest(false, 120)
	p := shared.NewProgress()

	s1 := p.AddStep(shared.PhaseModel, "m1", "")
	p.StartStep(s1)
	s1.Status = shared.StepStalled

	s2 := p.AddStep(shared.PhaseBuild, "b1", "")
	p.StartStep(s2)
	s2.Status = shared.StepStalled

	lines := r.Lines(p)

	// Only one stall warning banner, even with multiple stalled steps
	warningCount := 0
	for _, line := range lines {
		if strings.Contains(line, "heartbeat") {
			warningCount++
		}
	}
	if warningCount != 1 {
		t.Errorf("expected exactly 1 stall warning, got %d", warningCount)
	}
}

// --- stripANSI edge cases ------------------------------------------------------

func TestStripANSI_EmptyString(t *testing.T) {
	if stripANSI("") != "" {
		t.Error("stripANSI of empty string should be empty")
	}
}

func TestStripANSI_NoEscapeSequences(t *testing.T) {
	input := "plain text with no escapes"
	if stripANSI(input) != input {
		t.Errorf("stripANSI should be identity for plain text, got %q", stripANSI(input))
	}
}

func TestStripANSI_BackToBackSequences(t *testing.T) {
	// Two escape sequences with no text between them
	input := "\033[1m\033[32m"
	result := stripANSI(input)
	if result != "" {
		t.Errorf("back-to-back escapes should produce empty string, got %q", result)
	}
}
