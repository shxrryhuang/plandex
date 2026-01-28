package progress

import (
	"fmt"

	shared "plandex-shared"
)

// StreamAdapter translates existing StreamMessage events into progress updates.
// This bridges the current streaming protocol with the new progress tracking system.
type StreamAdapter struct {
	tracker      *Tracker
	currentPhase shared.ProgressPhase
	buildSteps   map[string]string // path -> stepID
	llmStepID    string
	contextStepID string
}

// NewStreamAdapter creates an adapter for translating stream messages.
func NewStreamAdapter(tracker *Tracker) *StreamAdapter {
	return &StreamAdapter{
		tracker:    tracker,
		buildSteps: make(map[string]string),
	}
}

// HandleMessage processes a StreamMessage and updates progress accordingly.
func (a *StreamAdapter) HandleMessage(msg *shared.StreamMessage) {
	switch msg.Type {
	case shared.StreamMessageStart:
		a.handleStart(msg)

	case shared.StreamMessageConnectActive:
		a.handleReconnect(msg)

	case shared.StreamMessageReply:
		a.handleReply(msg)

	case shared.StreamMessageDescribing:
		a.handleDescribing(msg)

	case shared.StreamMessageRepliesFinished:
		a.handleRepliesFinished()

	case shared.StreamMessageBuildInfo:
		a.handleBuildInfo(msg)

	case shared.StreamMessagePromptMissingFile:
		a.handleMissingFile(msg)

	case shared.StreamMessageLoadContext:
		a.handleLoadContext(msg)

	case shared.StreamMessageFinished:
		a.handleFinished()

	case shared.StreamMessageError:
		a.handleError(msg)

	case shared.StreamMessageAborted:
		a.handleAborted()

	case shared.StreamMessageHeartbeat:
		// Heartbeats are handled by the tracker's tick loop
		// No action needed here

	case shared.StreamMessageMulti:
		// Process batched messages
		for _, subMsg := range msg.StreamMessages {
			subMsgCopy := subMsg
			a.HandleMessage(&subMsgCopy)
		}
	}
}

func (a *StreamAdapter) handleStart(msg *shared.StreamMessage) {
	// Starting fresh or reconnecting
	if msg.InitBuildOnly {
		a.setPhase(shared.PhaseBuilding, "Building files")
	} else {
		a.setPhase(shared.PhaseInitializing, "Initializing")
	}

	// If there are init replies, we're resuming
	if len(msg.InitReplies) > 0 {
		a.setPhase(shared.PhasePlanning, "Resuming planning")
	}
}

func (a *StreamAdapter) handleReconnect(msg *shared.StreamMessage) {
	// Reconnecting to an active plan
	a.tracker.AddWarning("Reconnected to running plan")

	if msg.InitBuildOnly {
		a.setPhase(shared.PhaseBuilding, "Building files")
	}
}

func (a *StreamAdapter) handleReply(msg *shared.StreamMessage) {
	// LLM is responding
	if a.currentPhase != shared.PhasePlanning {
		a.setPhase(shared.PhasePlanning, "Planning task")
	}

	// Start or update LLM step
	if a.llmStepID == "" {
		a.llmStepID = a.tracker.TrackLLMCall("streaming")
	}

	// Update with chunk info (could track token count here)
	if len(msg.ReplyChunk) > 0 {
		// Estimate tokens from chunk length (rough approximation)
		estimatedTokens := len(msg.ReplyChunk) / 4
		a.tracker.UpdateStep(a.llmStepID, StepUpdates{
			Tokens: estimatedTokens,
		})
	}
}

func (a *StreamAdapter) handleDescribing(msg *shared.StreamMessage) {
	// LLM is describing changes
	if a.currentPhase != shared.PhaseDescribing {
		// Complete the planning LLM step
		if a.llmStepID != "" {
			a.tracker.CompleteStep(a.llmStepID)
			a.llmStepID = ""
		}
		a.setPhase(shared.PhaseDescribing, "Describing changes")
	}

	// Could track description progress here
	if msg.Description != nil {
		// Description contains info about proposed changes
	}
}

func (a *StreamAdapter) handleRepliesFinished() {
	// LLM finished responding
	if a.llmStepID != "" {
		a.tracker.CompleteStep(a.llmStepID)
		a.llmStepID = ""
	}
}

func (a *StreamAdapter) handleBuildInfo(msg *shared.StreamMessage) {
	if msg.BuildInfo == nil {
		return
	}

	bi := msg.BuildInfo

	// Transition to building phase if needed
	if a.currentPhase != shared.PhaseBuilding {
		a.setPhase(shared.PhaseBuilding, "Building files")
	}

	// Get or create step for this file
	stepID, exists := a.buildSteps[bi.Path]
	if !exists {
		// Determine label based on path
		label := "Building file"
		detail := bi.Path
		if bi.Path == "_apply.sh" {
			label = "Building commands"
			detail = "apply script"
		}

		stepID = a.tracker.StartStep(shared.StepKindFileBuild, label, detail)
		a.buildSteps[bi.Path] = stepID
	}

	// Update step state
	if bi.Finished {
		if bi.Removed {
			a.tracker.SkipStep(stepID)
		} else {
			a.tracker.UpdateStep(stepID, StepUpdates{
				Tokens: bi.NumTokens,
			})
			a.tracker.CompleteStep(stepID)
		}
	} else {
		// Still building - update token count
		a.tracker.UpdateStep(stepID, StepUpdates{
			Tokens: bi.NumTokens,
		})
	}
}

func (a *StreamAdapter) handleMissingFile(msg *shared.StreamMessage) {
	// Waiting for user decision on missing file
	stepID := a.tracker.TrackUserInput(fmt.Sprintf("missing file: %s", msg.MissingFilePath))
	a.tracker.SetWaiting(stepID, "user decision")
}

func (a *StreamAdapter) handleLoadContext(msg *shared.StreamMessage) {
	if len(msg.LoadContextFiles) == 0 {
		return
	}

	// Auto-loading context files
	if a.contextStepID == "" {
		a.contextStepID = a.tracker.TrackContext(len(msg.LoadContextFiles))
	} else {
		// Update with new file count
		a.tracker.UpdateStep(a.contextStepID, StepUpdates{
			Detail: fmt.Sprintf("%d files", len(msg.LoadContextFiles)),
		})
	}
}

func (a *StreamAdapter) handleFinished() {
	// Execution completed successfully
	a.setPhase(shared.PhaseCompleted, "Completed")

	// Complete any remaining steps
	if a.llmStepID != "" {
		a.tracker.CompleteStep(a.llmStepID)
	}
	if a.contextStepID != "" {
		a.tracker.CompleteStep(a.contextStepID)
	}
}

func (a *StreamAdapter) handleError(msg *shared.StreamMessage) {
	// Execution failed
	a.setPhase(shared.PhaseFailed, "Failed")

	errMsg := "unknown error"
	if msg.Error != nil {
		errMsg = msg.Error.Msg
	}

	// Fail the current step
	if a.llmStepID != "" {
		a.tracker.FailStep(a.llmStepID, errMsg)
	}

	// Mark any running build steps as failed
	for _, stepID := range a.buildSteps {
		report := a.tracker.GetReport()
		for _, step := range report.Steps {
			if step.ID == stepID && !step.State.IsTerminal() {
				a.tracker.FailStep(stepID, "interrupted")
			}
		}
	}

	a.tracker.AddWarning(fmt.Sprintf("Error: %s", errMsg))
}

func (a *StreamAdapter) handleAborted() {
	// User stopped execution
	a.setPhase(shared.PhaseStopped, "Stopped")

	// Mark running steps as skipped
	if a.llmStepID != "" {
		a.tracker.SkipStep(a.llmStepID)
	}
	for _, stepID := range a.buildSteps {
		report := a.tracker.GetReport()
		for _, step := range report.Steps {
			if step.ID == stepID && !step.State.IsTerminal() {
				a.tracker.SkipStep(stepID)
			}
		}
	}
}

func (a *StreamAdapter) setPhase(phase shared.ProgressPhase, label string) {
	if a.currentPhase != phase {
		a.currentPhase = phase
		a.tracker.SetPhase(phase, label)
	}
}

// CompleteContextLoad marks context loading as complete.
func (a *StreamAdapter) CompleteContextLoad() {
	if a.contextStepID != "" {
		a.tracker.CompleteStep(a.contextStepID)
		a.contextStepID = ""
	}
}

// SetLLMModel updates the LLM step with model information.
func (a *StreamAdapter) SetLLMModel(model string) {
	if a.llmStepID != "" {
		a.tracker.UpdateStep(a.llmStepID, StepUpdates{
			Detail: model,
		})
	}
}

// AddCustomStep adds a custom step to tracking.
func (a *StreamAdapter) AddCustomStep(kind shared.StepKind, label, detail string) string {
	return a.tracker.StartStep(kind, label, detail)
}

// MarkStepComplete marks a custom step as complete.
func (a *StreamAdapter) MarkStepComplete(stepID string) {
	a.tracker.CompleteStep(stepID)
}

// MarkStepFailed marks a custom step as failed.
func (a *StreamAdapter) MarkStepFailed(stepID string, err string) {
	a.tracker.FailStep(stepID, err)
}
