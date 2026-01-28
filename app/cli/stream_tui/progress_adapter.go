package streamtui

import (
	"fmt"
	"sync"
	"time"

	shared "plandex-shared"
)

// =============================================================================
// PROGRESS ADAPTER
//
// Translates the existing StreamMessage protocol into shared.Progress state
// updates. This adapter sits between the stream consumer (update.go) and the
// progress model, so the renderer stays decoupled from wire-format details.
//
// It is also responsible for heartbeat tracking and stall detection: if no
// message arrives within HeartbeatTimeout, any running steps are marked stalled.
// =============================================================================

// HeartbeatTimeout is how long to wait for server activity before marking
// running steps as stalled.
const HeartbeatTimeout = 15 * time.Second

// ProgressAdapter maintains a shared.Progress and provides methods that
// mirror the stream message lifecycle.  All public methods are safe for
// concurrent use: OnMessage from the Bubble Tea goroutine and Progress()
// from the View goroutine may run in parallel with the stall-timer callback.
type ProgressAdapter struct {
	mu       sync.RWMutex
	progress *shared.Progress

	// Current step pointers for phases that track a single active operation.
	// Multiple build steps can be active simultaneously via buildSteps map.
	connectStep  *shared.Step
	contextStep  *shared.Step
	modelStep    *shared.Step
	finalizeStep *shared.Step

	// Per-file build steps, keyed by file path.
	buildSteps map[string]*shared.Step

	// Stall detection timer. Reset on every message received.
	stallTimer *time.Timer
	stallDone  chan struct{} // closed when adapter is shut down
}

// NewProgressAdapter creates a new adapter with a fresh Progress snapshot.
func NewProgressAdapter() *ProgressAdapter {
	a := &ProgressAdapter{
		progress:   shared.NewProgress(),
		buildSteps: make(map[string]*shared.Step),
		stallDone:  make(chan struct{}),
	}
	// Seed the connect step immediately — the first thing any run does.
	a.connectStep = a.progress.AddStep(shared.PhaseConnect, "connecting", "establishing server connection")
	a.progress.StartStep(a.connectStep)
	a.resetStallTimer()
	return a
}

// Progress returns a read-locked snapshot of the current progress.
// The returned pointer is safe to read without further synchronisation until
// the next call to OnMessage.
func (a *ProgressAdapter) Progress() *shared.Progress {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.progress
}

// Shutdown stops the stall detection goroutine. Must be called when the
// stream is closed (finished, error, or user abort).
func (a *ProgressAdapter) Shutdown() {
	select {
	case <-a.stallDone:
		// already closed
	default:
		close(a.stallDone)
	}
	if a.stallTimer != nil {
		a.stallTimer.Stop()
	}
}

// resetStallTimer restarts the heartbeat watchdog. Any running step will be
// marked stalled if no further message arrives within HeartbeatTimeout.
// The callback acquires the write lock so it cannot race with OnMessage or
// Progress().
func (a *ProgressAdapter) resetStallTimer() {
	if a.stallTimer != nil {
		a.stallTimer.Stop()
	}
	a.stallTimer = time.AfterFunc(HeartbeatTimeout, func() {
		select {
		case <-a.stallDone:
			return
		default:
			a.mu.Lock()
			defer a.mu.Unlock()
			for _, s := range a.progress.Steps {
				if s.Status == shared.StepRunning {
					s.Status = shared.StepStalled
				}
			}
		}
	})
}

// OnMessage is the single entry point: routes a StreamMessage to the
// appropriate handler and resets the stall timer.  The write lock is held
// for the entire dispatch so the stall-timer goroutine cannot interleave.
//
// Multi messages are dispatched inline via onMessageLocked (not recursively
// through the public OnMessage) to avoid acquiring the lock twice and to
// prevent each sub-message from being processed a second time.
func (a *ProgressAdapter) OnMessage(msg *shared.StreamMessage) {
	if msg == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onMessageLocked(msg)
}

// onMessageLocked is the lock-held dispatch.  Callers must hold a.mu.Lock().
func (a *ProgressAdapter) onMessageLocked(msg *shared.StreamMessage) {
	a.resetStallTimer()
	a.progress.RecordHeartbeat()

	switch msg.Type {
	case shared.StreamMessageStart:
		a.onStart(msg)
	case shared.StreamMessageConnectActive:
		a.onConnectActive(msg)
	case shared.StreamMessageLoadContext:
		a.onLoadContext(msg)
	case shared.StreamMessageDescribing:
		a.onDescribing(msg)
	case shared.StreamMessageReply:
		a.onReply(msg)
	case shared.StreamMessageRepliesFinished:
		a.onRepliesFinished(msg)
	case shared.StreamMessageBuildInfo:
		a.onBuildInfo(msg)
	case shared.StreamMessageFinished:
		a.onFinished(msg)
	case shared.StreamMessageAborted:
		a.onAborted(msg)
	case shared.StreamMessageError:
		a.onError(msg)
	case shared.StreamMessageHeartbeat:
		// Heartbeat alone — stall timer already reset above
	case shared.StreamMessageMulti:
		for i := range msg.StreamMessages {
			a.onMessageLocked(&msg.StreamMessages[i])
		}
	}
}

// --- Individual message handlers ------------------------------------------

func (a *ProgressAdapter) onStart(msg *shared.StreamMessage) {
	// The server acknowledged receipt. Connection is established.
	if a.connectStep != nil && a.connectStep.Status != shared.StepCompleted {
		a.progress.CompleteStep(a.connectStep)
	}
}

func (a *ProgressAdapter) onConnectActive(msg *shared.StreamMessage) {
	// Connected to an already-active plan (resume scenario).
	if a.connectStep != nil && a.connectStep.Status != shared.StepCompleted {
		a.connectStep.Detail = "reconnected to active plan"
		a.progress.CompleteStep(a.connectStep)
	}
}

func (a *ProgressAdapter) onLoadContext(msg *shared.StreamMessage) {
	if a.connectStep != nil && a.connectStep.Status != shared.StepCompleted {
		a.progress.CompleteStep(a.connectStep)
	}

	if a.contextStep == nil {
		a.contextStep = a.progress.AddStep(shared.PhaseContext, "loading context", "")
		a.progress.StartStep(a.contextStep)
	}

	if len(msg.LoadContextFiles) > 0 {
		a.contextStep.Detail = fmt.Sprintf("%d file(s)", len(msg.LoadContextFiles))
	}
}

func (a *ProgressAdapter) onDescribing(msg *shared.StreamMessage) {
	// "Describing" means the model is generating a plan description before
	// the main reply. Treat as the start of the model phase.
	if a.contextStep != nil && a.contextStep.Status == shared.StepRunning {
		a.progress.CompleteStep(a.contextStep)
	}

	if a.modelStep == nil {
		a.modelStep = a.progress.AddStep(shared.PhaseModel, "model", "generating description")
		a.progress.StartStep(a.modelStep)
	} else {
		a.modelStep.Detail = "generating description"
	}
}

func (a *ProgressAdapter) onReply(msg *shared.StreamMessage) {
	if msg.ReplyChunk == "" {
		return
	}

	// Ensure context step is closed if it was open
	if a.contextStep != nil && a.contextStep.Status == shared.StepRunning {
		a.progress.CompleteStep(a.contextStep)
	}

	if a.modelStep == nil {
		a.modelStep = a.progress.AddStep(shared.PhaseModel, "model", "streaming reply")
		a.progress.StartStep(a.modelStep)
	} else if a.modelStep.Status != shared.StepRunning {
		// Re-entered model phase (e.g. after missing-file prompt).
		// Clear previous timing so the renderer does not display a stale
		// duration from the prior run of this step.
		a.modelStep.Status = shared.StepRunning
		now := time.Now()
		a.modelStep.StartedAt = &now
		a.modelStep.FinishedAt = nil
		a.modelStep.DurationMs = 0
		a.modelStep.Detail = "streaming reply (continued)"
		a.progress.ActivePhase = shared.PhaseModel
	} else {
		a.modelStep.Detail = "streaming reply"
	}
}

func (a *ProgressAdapter) onRepliesFinished(msg *shared.StreamMessage) {
	if a.modelStep != nil && a.modelStep.Status == shared.StepRunning {
		a.modelStep.Detail = "reply complete"
		a.progress.CompleteStep(a.modelStep)
	}
}

func (a *ProgressAdapter) onBuildInfo(msg *shared.StreamMessage) {
	if msg.BuildInfo == nil {
		return
	}

	path := msg.BuildInfo.Path
	step, exists := a.buildSteps[path]

	if !exists {
		label := "build"
		step = a.progress.AddStep(shared.PhaseBuild, label, path)
		a.progress.StartStep(step)
		a.buildSteps[path] = step
		a.progress.ActivePhase = shared.PhaseBuild
	}

	if msg.BuildInfo.Removed {
		step.Detail = path + " (removed)"
		a.progress.CompleteStep(step)
		return
	}

	if msg.BuildInfo.Finished {
		step.Detail = path
		a.progress.CompleteStep(step)
	} else {
		tokens := msg.BuildInfo.NumTokens
		if tokens > 0 {
			step.Detail = fmt.Sprintf("%s — %d tokens", path, tokens)
		}
	}
}

func (a *ProgressAdapter) onFinished(msg *shared.StreamMessage) {
	a.closeAllRunning()

	a.finalizeStep = a.progress.AddStep(shared.PhaseFinalize, "finalize", "run complete")
	a.progress.StartStep(a.finalizeStep)
	a.progress.CompleteStep(a.finalizeStep)

	a.progress.Finished = true
	a.Shutdown()
}

func (a *ProgressAdapter) onAborted(msg *shared.StreamMessage) {
	a.closeAllRunning()
	a.progress.Finished = true
	a.progress.Error = "stopped by user"
	a.Shutdown()
}

func (a *ProgressAdapter) onError(msg *shared.StreamMessage) {
	errMsg := "unknown error"
	if msg.Error != nil {
		errMsg = msg.Error.Msg
	}

	// Fail whichever step is currently running
	for _, s := range a.progress.Steps {
		if s.Status == shared.StepRunning || s.Status == shared.StepStalled {
			a.progress.FailStep(s, errMsg)
		}
	}

	a.progress.Finished = true
	a.progress.Error = errMsg
	a.Shutdown()
}

// closeAllRunning completes any steps still in Running or Stalled state
// without an error — used on clean termination paths.
func (a *ProgressAdapter) closeAllRunning() {
	for _, s := range a.progress.Steps {
		if s.Status == shared.StepRunning || s.Status == shared.StepStalled {
			a.progress.CompleteStep(s)
		}
	}
}
