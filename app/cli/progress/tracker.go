package progress

import (
	"fmt"
	"io"
	"sync"
	"time"

	shared "plandex-shared"
)

// Tracker manages progress state and rendering for a plan execution.
type Tracker struct {
	mu       sync.RWMutex
	report   *shared.ProgressReport
	renderer *Renderer
	stepSeq  int

	// Channels for async updates
	updateCh chan shared.ProgressMessage
	doneCh   chan struct{}

	// Animation state
	spinnerFrame int
	ticker       *time.Ticker

	// Stall detection
	stallThreshold time.Duration
	lastActivity   time.Time

	// Callbacks
	onStall     func(step *shared.ProgressStep)
	onPhaseEnd  func(phase shared.ProgressPhase)
}

// TrackerConfig configures a new progress tracker.
type TrackerConfig struct {
	PlanID         string
	Branch         string
	Output         io.Writer
	Verbose        bool
	StallThreshold time.Duration
	OnStall        func(step *shared.ProgressStep)
	OnPhaseEnd     func(phase shared.ProgressPhase)
}

// NewTracker creates a new progress tracker.
func NewTracker(cfg TrackerConfig) *Tracker {
	if cfg.StallThreshold == 0 {
		cfg.StallThreshold = 60 * time.Second
	}

	report := &shared.ProgressReport{
		PlanID:    cfg.PlanID,
		Branch:    cfg.Branch,
		Phase:     shared.PhaseInitializing,
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
		Steps:     make([]shared.ProgressStep, 0),
		CanCancel: true,
	}

	return &Tracker{
		report:         report,
		renderer:       NewRenderer(cfg.Output, cfg.Verbose),
		updateCh:       make(chan shared.ProgressMessage, 100),
		doneCh:         make(chan struct{}),
		stallThreshold: cfg.StallThreshold,
		lastActivity:   time.Now(),
		onStall:        cfg.OnStall,
		onPhaseEnd:     cfg.OnPhaseEnd,
	}
}

// Start begins the progress tracking loop.
func (t *Tracker) Start() {
	t.ticker = time.NewTicker(100 * time.Millisecond)

	go func() {
		for {
			select {
			case <-t.doneCh:
				return
			case msg := <-t.updateCh:
				t.processMessage(msg)
			case <-t.ticker.C:
				t.tick()
			}
		}
	}()
}

// Stop ends the progress tracking loop.
func (t *Tracker) Stop() {
	close(t.doneCh)
	if t.ticker != nil {
		t.ticker.Stop()
	}
}

// SetPhase transitions to a new execution phase.
func (t *Tracker) SetPhase(phase shared.ProgressPhase, label string) {
	t.mu.Lock()
	oldPhase := t.report.Phase
	t.report.Phase = phase
	t.report.PhaseLabel = label
	t.report.UpdatedAt = time.Now()
	t.lastActivity = time.Now()
	t.mu.Unlock()

	if t.onPhaseEnd != nil && oldPhase != phase {
		t.onPhaseEnd(oldPhase)
	}

	t.render()
}

// StartStep begins tracking a new step.
func (t *Tracker) StartStep(kind shared.StepKind, label string, detail string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stepSeq++
	id := fmt.Sprintf("step-%d", t.stepSeq)

	step := shared.ProgressStep{
		ID:        id,
		Kind:      kind,
		State:     shared.StepStateRunning,
		Label:     label,
		Detail:    detail,
		StartedAt: time.Now(),
		Progress:  -1, // Indeterminate
	}

	t.report.Steps = append(t.report.Steps, step)
	t.report.CurrentStepID = id
	t.report.UpdateCounts()
	t.lastActivity = time.Now()

	return id
}

// UpdateStep modifies a step's state or progress.
func (t *Tracker) UpdateStep(id string, updates StepUpdates) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.report.Steps {
		if t.report.Steps[i].ID == id {
			step := &t.report.Steps[i]

			if updates.State != "" {
				step.State = updates.State
				if step.State.IsTerminal() {
					step.CompletedAt = time.Now()
				}
			}
			if updates.Progress >= 0 {
				step.Progress = updates.Progress
			}
			if updates.ProgressMsg != "" {
				step.ProgressMsg = updates.ProgressMsg
			}
			if updates.Detail != "" {
				step.Detail = updates.Detail
			}
			if updates.Tokens > 0 {
				step.TokensProcessed = updates.Tokens
			}
			if updates.Bytes > 0 {
				step.BytesProcessed = updates.Bytes
			}
			if updates.Error != "" {
				step.Error = updates.Error
			}

			t.report.UpdateCounts()
			t.lastActivity = time.Now()
			break
		}
	}
}

// CompleteStep marks a step as completed successfully.
func (t *Tracker) CompleteStep(id string) {
	t.UpdateStep(id, StepUpdates{State: shared.StepStateCompleted})
}

// FailStep marks a step as failed.
func (t *Tracker) FailStep(id string, err string) {
	t.UpdateStep(id, StepUpdates{State: shared.StepStateFailed, Error: err})
}

// SkipStep marks a step as skipped.
func (t *Tracker) SkipStep(id string) {
	t.UpdateStep(id, StepUpdates{State: shared.StepStateSkipped})
}

// SetWaiting marks the current step as waiting on external resource.
func (t *Tracker) SetWaiting(id string, waitingFor string) {
	t.UpdateStep(id, StepUpdates{
		State:       shared.StepStateWaiting,
		ProgressMsg: waitingFor,
	})
}

// AddWarning adds a warning message to the report.
func (t *Tracker) AddWarning(msg string) {
	t.mu.Lock()
	t.report.Warnings = append(t.report.Warnings, msg)
	t.mu.Unlock()
	t.render()
}

// GetReport returns a copy of the current progress report.
func (t *Tracker) GetReport() shared.ProgressReport {
	t.mu.RLock()
	defer t.mu.RUnlock()

	report := *t.report
	report.Duration = time.Since(report.StartedAt)
	return report
}

// SendMessage queues a progress message for processing.
func (t *Tracker) SendMessage(msg shared.ProgressMessage) {
	select {
	case t.updateCh <- msg:
	default:
		// Channel full, drop message (best-effort)
	}
}

// StepUpdates contains optional updates to apply to a step.
type StepUpdates struct {
	State       shared.StepState
	Progress    float64
	ProgressMsg string
	Detail      string
	Tokens      int
	Bytes       int
	Error       string
}

func (t *Tracker) processMessage(msg shared.ProgressMessage) {
	switch msg.Type {
	case shared.ProgressMsgStepStart:
		if msg.Step != nil {
			t.mu.Lock()
			t.report.Steps = append(t.report.Steps, *msg.Step)
			t.report.CurrentStepID = msg.Step.ID
			t.report.UpdateCounts()
			t.lastActivity = time.Now()
			t.mu.Unlock()
		}

	case shared.ProgressMsgStepUpdate:
		if msg.Step != nil {
			t.mu.Lock()
			for i := range t.report.Steps {
				if t.report.Steps[i].ID == msg.Step.ID {
					t.report.Steps[i] = *msg.Step
					break
				}
			}
			t.report.UpdateCounts()
			t.lastActivity = time.Now()
			t.mu.Unlock()
		}

	case shared.ProgressMsgStepEnd:
		if msg.Step != nil {
			t.UpdateStep(msg.Step.ID, StepUpdates{State: msg.Step.State})
		}

	case shared.ProgressMsgPhaseChange:
		t.SetPhase(msg.Phase, string(msg.Phase))

	case shared.ProgressMsgFullReport:
		if msg.Report != nil {
			t.mu.Lock()
			t.report = msg.Report
			t.lastActivity = time.Now()
			t.mu.Unlock()
		}
	}

	t.render()

	// Log for non-TTY
	if !t.renderer.isTTY && t.renderer.verbose {
		t.renderer.RenderStepEvent(&msg)
	}
}

func (t *Tracker) tick() {
	t.mu.Lock()
	t.spinnerFrame++

	// Check for stalls
	current := t.report.CurrentStep()
	if current != nil && current.IsStalled() {
		current.State = shared.StepStateStalled
		t.report.UpdateCounts()
		t.report.SetSuggestedAction()

		if t.onStall != nil {
			go t.onStall(current)
		}
	}

	// Update duration
	t.report.Duration = time.Since(t.report.StartedAt)
	t.mu.Unlock()

	t.render()
}

func (t *Tracker) render() {
	t.mu.RLock()
	report := *t.report
	frame := t.spinnerFrame
	t.mu.RUnlock()

	report.SetSuggestedAction()
	t.renderer.RenderReport(&report, frame)
}

// Convenience functions for common step types

// TrackLLMCall tracks an LLM API call.
func (t *Tracker) TrackLLMCall(model string) string {
	return t.StartStep(shared.StepKindLLMCall, "Calling LLM", model)
}

// TrackFileBuild tracks file generation.
func (t *Tracker) TrackFileBuild(path string) string {
	return t.StartStep(shared.StepKindFileBuild, "Building file", path)
}

// TrackFileWrite tracks file write operation.
func (t *Tracker) TrackFileWrite(path string) string {
	return t.StartStep(shared.StepKindFileWrite, "Writing file", path)
}

// TrackToolExec tracks external tool execution.
func (t *Tracker) TrackToolExec(tool string) string {
	return t.StartStep(shared.StepKindToolExec, "Running tool", tool)
}

// TrackContext tracks context loading.
func (t *Tracker) TrackContext(files int) string {
	return t.StartStep(shared.StepKindContext, "Loading context", fmt.Sprintf("%d files", files))
}

// TrackValidation tracks validation step.
func (t *Tracker) TrackValidation(what string) string {
	return t.StartStep(shared.StepKindValidation, "Validating", what)
}

// TrackUserInput tracks waiting for user input.
func (t *Tracker) TrackUserInput(prompt string) string {
	id := t.StartStep(shared.StepKindUserInput, "Waiting for input", prompt)
	t.SetWaiting(id, "user response")
	return id
}
