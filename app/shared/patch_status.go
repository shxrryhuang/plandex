package shared

import "time"

// =============================================================================
// PATCH STATUS REPORTING
// =============================================================================
//
// PatchStatusReporter provides structured lifecycle events so callers (CLI,
// tests, logs) can observe exactly what phase each file is in during a patch
// application.  Every state transition is recorded with a timestamp so the
// caller can compute durations and build progress bars if desired.
//
// Lifecycle per file:
//
//   staged -> applying -> applied  (happy path)
//                      -> failed   (apply error, triggers transaction rollback)
//                      -> skipped  (content unchanged)
//
// Lifecycle per transaction:
//
//   staging -> applying -> committed     (all files applied)
//                       -> rolling_back -> rolled_back
//
// =============================================================================

// PatchPhase identifies where a patch application currently sits.
type PatchPhase string

const (
	PhasePreparing      PatchPhase = "preparing"    // Building operation list
	PhaseStaging        PatchPhase = "staging"      // Capturing snapshots
	PatchPhaseApplying  PatchPhase = "applying"     // Writing files
	PhaseCommitting     PatchPhase = "committing"   // Finalising transaction
	PhaseRollingBack    PatchPhase = "rolling_back" // Reverting applied ops
	PhaseDone           PatchPhase = "done"         // Terminal: committed or rolled back
)

// FileStatus is the per-file event emitted by the reporter.
type FileStatus struct {
	Path      string       `json:"path"`
	Phase     PatchPhase   `json:"phase"`
	OpType    string       `json:"opType"`    // create | modify | delete | rename
	Error     string       `json:"error,omitempty"`
	Hash      string       `json:"hash,omitempty"`     // content hash after apply
	OldHash   string       `json:"oldHash,omitempty"`  // snapshot hash before apply
	Timestamp time.Time    `json:"timestamp"`
}

// PatchEvent is a transaction-level event.
type PatchEvent struct {
	TxId      string     `json:"txId"`
	Phase     PatchPhase `json:"phase"`
	Reason    string     `json:"reason,omitempty"` // rollback reason
	FileCount int        `json:"fileCount"`
	Timestamp time.Time  `json:"timestamp"`
}

// PatchStatusReporter is the interface callers implement to receive events.
type PatchStatusReporter interface {
	// OnFileStatus is called each time a single file transitions state.
	OnFileStatus(status FileStatus)
	// OnPatchEvent is called for transaction-level lifecycle changes.
	OnPatchEvent(event PatchEvent)
}

// LoggingReporter is a simple default that collects all events in memory.
// Useful for tests and for the CLI to render after the fact.
type LoggingReporter struct {
	FileEvents  []FileStatus
	PatchEvents []PatchEvent
}

func NewLoggingReporter() *LoggingReporter {
	return &LoggingReporter{}
}

func (r *LoggingReporter) OnFileStatus(s FileStatus) {
	r.FileEvents = append(r.FileEvents, s)
}

func (r *LoggingReporter) OnPatchEvent(e PatchEvent) {
	r.PatchEvents = append(r.PatchEvents, e)
}

// Summary returns a human-readable tally of final file outcomes.
//
// Each file can emit multiple events as it moves through phases.  To get the
// final outcome we keep only the *last* event seen for each path and classify
// it:
//
//   - Error non-empty → Failed
//   - Phase == PhaseStaging (skip event) → Skipped
//   - Otherwise → Applied
func (r *LoggingReporter) Summary() PatchSummary {
	// Collect the latest event per path so earlier intermediate phases
	// (e.g. "staging" before "applying") don't inflate counts.
	latest := make(map[string]FileStatus)
	for _, e := range r.FileEvents {
		latest[e.Path] = e
	}

	var s PatchSummary
	for _, e := range latest {
		switch {
		case e.Error != "":
			s.Failed++
		case e.Phase == PhaseStaging:
			s.Skipped++
		default:
			s.Applied++
		}
	}
	return s
}

// PatchSummary provides aggregate counts after a patch run.
type PatchSummary struct {
	Applied int
	Failed  int
	Skipped int
}
