// Package pipeline provides a standalone pipeline for testing and demonstrating
// the progress reporting system without affecting the original stream_tui code.
package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	shared "plandex-shared"
)

// PipelineConfig configures the progress pipeline
type PipelineConfig struct {
	// Output configuration
	IsTTY       bool
	Width       int
	EnableColor bool

	// Callbacks
	OnPhaseChange func(phase shared.ProgressPhase, label string)
	OnStepStart   func(step *shared.ProgressStep)
	OnStepUpdate  func(step *shared.ProgressStep)
	OnStepEnd     func(step *shared.ProgressStep)
	OnStall       func(step *shared.ProgressStep)
	OnComplete    func(report *shared.ProgressReport)
	OnError       func(err error)

	// Timing configuration
	StallThreshold time.Duration
}

// DefaultConfig returns a default pipeline configuration
func DefaultConfig() PipelineConfig {
	return PipelineConfig{
		IsTTY:          true,
		Width:          80,
		EnableColor:    true,
		StallThreshold: 60 * time.Second,
	}
}

// Pipeline manages progress tracking for a plan execution
type Pipeline struct {
	config PipelineConfig
	report *shared.ProgressReport
	mu     sync.RWMutex

	// Step tracking
	stepSeq  int
	stepByID map[string]*shared.ProgressStep

	// Phase tracking (local, not in shared struct)
	phaseStartedAt time.Time
	errorMsg       string

	// Stall detection
	ctx        context.Context
	cancel     context.CancelFunc
	stallCheck *time.Ticker

	// State
	running bool
}

// New creates a new progress pipeline
func New(config PipelineConfig) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())

	return &Pipeline{
		config:   config,
		stepByID: make(map[string]*shared.ProgressStep),
		ctx:      ctx,
		cancel:   cancel,
		report: &shared.ProgressReport{
			Phase:     shared.ProgressPhaseInitializing,
			StartedAt: time.Now(),
			Steps:     []shared.ProgressStep{},
		},
	}
}

// Start begins the pipeline
func (p *Pipeline) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return
	}
	p.running = true

	// Clean up any existing ticker
	if p.stallCheck != nil {
		p.stallCheck.Stop()
	}

	// Start stall detection
	p.stallCheck = time.NewTicker(time.Second)
	go p.stallDetectionLoop()
}

// Stop halts the pipeline
func (p *Pipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}
	p.running = false

	// Cancel context first to stop goroutine
	p.cancel()

	// Then stop and clear ticker
	if p.stallCheck != nil {
		p.stallCheck.Stop()
		p.stallCheck = nil
	}
}

// SetPhase updates the current execution phase
func (p *Pipeline) SetPhase(phase shared.ProgressPhase, label string) {
	p.mu.Lock()
	p.report.Phase = phase
	p.report.PhaseLabel = label
	p.phaseStartedAt = time.Now()
	p.report.UpdatedAt = time.Now()
	p.mu.Unlock()

	if p.config.OnPhaseChange != nil {
		p.config.OnPhaseChange(phase, label)
	}
}

// StartStep begins tracking a new step
func (p *Pipeline) StartStep(kind shared.StepKind, label, detail string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stepSeq++
	id := fmt.Sprintf("step-%d-%s", p.stepSeq, uuid.New().String()[:8])

	step := shared.ProgressStep{
		ID:        id,
		Kind:      kind,
		State:     shared.StepStateRunning,
		Label:     label,
		Detail:    detail,
		StartedAt: time.Now(),
		Progress:  -1, // Indeterminate
	}

	p.report.Steps = append(p.report.Steps, step)
	p.stepByID[id] = &p.report.Steps[len(p.report.Steps)-1]

	if p.config.OnStepStart != nil {
		p.config.OnStepStart(&step)
	}

	return id
}

// UpdateStep updates a step's state and metrics
func (p *Pipeline) UpdateStep(id string, updates StepUpdates) {
	p.mu.Lock()
	defer p.mu.Unlock()

	step, ok := p.stepByID[id]
	if !ok {
		return
	}

	if updates.State != "" {
		step.State = updates.State
	}
	if updates.Progress >= 0 {
		step.Progress = updates.Progress
	}
	if updates.Tokens > 0 {
		step.TokensProcessed += updates.Tokens // Accumulate, don't replace
	}
	if updates.Detail != "" {
		step.Detail = updates.Detail
	}

	if p.config.OnStepUpdate != nil {
		p.config.OnStepUpdate(step)
	}
}

// CompleteStep marks a step as completed
func (p *Pipeline) CompleteStep(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	step, ok := p.stepByID[id]
	if !ok {
		return
	}

	step.State = shared.StepStateCompleted
	step.CompletedAt = time.Now()
	step.Progress = 1.0

	if p.config.OnStepEnd != nil {
		p.config.OnStepEnd(step)
	}
}

// FailStep marks a step as failed
func (p *Pipeline) FailStep(id string, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	step, ok := p.stepByID[id]
	if !ok {
		return
	}

	step.State = shared.StepStateFailed
	step.CompletedAt = time.Now()
	step.Error = errMsg

	if p.config.OnStepEnd != nil {
		p.config.OnStepEnd(step)
	}
}

// SkipStep marks a step as skipped
func (p *Pipeline) SkipStep(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	step, ok := p.stepByID[id]
	if !ok {
		return
	}

	step.State = shared.StepStateSkipped
	step.CompletedAt = time.Now()

	if p.config.OnStepEnd != nil {
		p.config.OnStepEnd(step)
	}
}

// WaitStep marks a step as waiting for external resource
func (p *Pipeline) WaitStep(id string, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	step, ok := p.stepByID[id]
	if !ok {
		return
	}

	step.State = shared.StepStateWaiting
	if reason != "" {
		step.Detail = reason
	}

	if p.config.OnStepUpdate != nil {
		p.config.OnStepUpdate(step)
	}
}

// GetReport returns a copy of the current progress report
func (p *Pipeline) GetReport() shared.ProgressReport {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy
	report := *p.report
	report.Steps = make([]shared.ProgressStep, len(p.report.Steps))
	copy(report.Steps, p.report.Steps)
	return report
}

// GetStep returns a copy of a specific step
func (p *Pipeline) GetStep(id string) (shared.ProgressStep, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	step, ok := p.stepByID[id]
	if !ok {
		return shared.ProgressStep{}, false
	}
	return *step, true
}

// Complete marks the entire pipeline as completed
func (p *Pipeline) Complete() {
	p.SetPhase(shared.ProgressPhaseCompleted, "Completed")
	p.mu.Lock()
	p.report.CompletedAt = time.Now()
	report := *p.report
	p.mu.Unlock()

	if p.config.OnComplete != nil {
		p.config.OnComplete(&report)
	}
}

// Fail marks the entire pipeline as failed
func (p *Pipeline) Fail(err error) {
	p.SetPhase(shared.ProgressPhaseFailed, "Failed")
	p.mu.Lock()
	p.report.CompletedAt = time.Now()
	if err != nil {
		p.errorMsg = err.Error()
	}
	p.mu.Unlock()

	if p.config.OnError != nil {
		p.config.OnError(err)
	}
}

// stallDetectionLoop checks for stalled steps
func (p *Pipeline) stallDetectionLoop() {
	for {
		p.mu.RLock()
		ticker := p.stallCheck
		p.mu.RUnlock()

		if ticker == nil {
			return
		}

		select {
		case <-p.ctx.Done():
			return
		case _, ok := <-ticker.C:
			if !ok {
				return // Ticker was closed
			}
			p.checkForStalls()
		}
	}
}

// checkForStalls examines running steps for stalls
func (p *Pipeline) checkForStalls() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for _, step := range p.report.Steps {
		if step.State != shared.StepStateRunning {
			continue
		}

		threshold := p.getStallThreshold(step.Kind)
		if threshold == 0 {
			continue // Never stalls (e.g., user_input)
		}

		elapsed := now.Sub(step.StartedAt)
		if elapsed > threshold && step.State != shared.StepStateStalled {
			// Mark as stalled
			if s, ok := p.stepByID[step.ID]; ok {
				s.State = shared.StepStateStalled

				if p.config.OnStall != nil {
					p.config.OnStall(s)
				}
			}
		}
	}
}

// getStallThreshold returns the stall threshold for a step kind
func (p *Pipeline) getStallThreshold(kind shared.StepKind) time.Duration {
	// Use configured threshold or kind-specific defaults
	if p.config.StallThreshold > 0 {
		return p.config.StallThreshold
	}

	switch kind {
	case shared.StepKindLLMCall:
		return 60 * time.Second
	case shared.StepKindContext:
		return 10 * time.Second
	case shared.StepKindFileBuild:
		return 120 * time.Second
	case shared.StepKindFileWrite:
		return 4 * time.Second
	case shared.StepKindToolExec:
		return 60 * time.Second
	case shared.StepKindValidation:
		return 30 * time.Second
	case shared.StepKindUserInput:
		return 0 // Never stalls
	default:
		return 60 * time.Second
	}
}

// StepUpdates contains optional updates for a step
type StepUpdates struct {
	State    shared.StepState
	Progress float64
	Tokens   int
	Detail   string
}
