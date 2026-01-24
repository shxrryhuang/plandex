package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"time"
)

// =============================================================================
// RESUME ALGORITHM
// =============================================================================
//
// This algorithm safely resumes execution from the last known good checkpoint.
//
// Key principles:
//   1. VERIFY before resuming - ensure current state matches checkpoint
//   2. NEVER skip validation - divergence must be handled explicitly
//   3. FAIL SAFE - when in doubt, don't resume
//   4. AUDIT TRAIL - record all resume decisions
//
// =============================================================================

// ResumeResult contains the outcome of a resume attempt
type ResumeResult struct {
	Success          bool               `json:"success"`
	ResumedFrom      string             `json:"resumedFrom,omitempty"`    // Checkpoint name
	ResumedAtEntry   int                `json:"resumedAtEntry,omitempty"` // Entry index
	Divergences      []ResumeDivergence `json:"divergences,omitempty"`
	RepairActions    []RepairAction     `json:"repairActions,omitempty"`
	Error            string             `json:"error,omitempty"`
	ValidationReport *ValidationReport  `json:"validationReport,omitempty"`
}

// ResumeDivergence describes a difference between expected and actual state
type ResumeDivergence struct {
	Type         string `json:"type"` // "file_missing", "file_changed", "file_extra", "hash_mismatch"
	Path         string `json:"path"`
	Expected     string `json:"expected,omitempty"` // Expected hash or state
	Actual       string `json:"actual,omitempty"`   // Actual hash or state
	Severity     string `json:"severity"`           // "error", "warning", "info"
	Recoverable  bool   `json:"recoverable"`
	SuggestedFix string `json:"suggestedFix,omitempty"`
}

// RepairAction describes an action taken to repair state before resume
type RepairAction struct {
	Type        string    `json:"type"` // "restore_file", "delete_file", "skip_entry"
	Path        string    `json:"path,omitempty"`
	Description string    `json:"description"`
	PerformedAt time.Time `json:"performedAt"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

// ValidationReport summarizes pre-resume validation
type ValidationReport struct {
	CheckpointName   string            `json:"checkpointName"`
	CheckpointEntry  int               `json:"checkpointEntry"`
	FilesValidated   int               `json:"filesValidated"`
	FilesMatched     int               `json:"filesMatched"`
	FilesDiverged    int               `json:"filesDiverged"`
	JournalIntegrity bool              `json:"journalIntegrity"`
	SafeToResume     bool              `json:"safeToResume"`
	ValidationTime   time.Time         `json:"validationTime"`
	FileStates       map[string]string `json:"fileStates"` // path -> status
}

// ResumeOptions configures resume behavior
type ResumeOptions struct {
	// Checkpoint selection
	CheckpointName      string `json:"checkpointName,omitempty"` // Specific checkpoint to resume from
	UseLatestCheckpoint bool   `json:"useLatestCheckpoint"`      // Use most recent checkpoint

	// Validation behavior
	StrictValidation bool `json:"strictValidation"` // Fail on any divergence
	AllowRepair      bool `json:"allowRepair"`      // Attempt to repair divergences
	ValidateAllFiles bool `json:"validateAllFiles"` // Validate all tracked files, not just checkpoint files

	// Resume behavior
	SkipDivergedEntries bool `json:"skipDivergedEntries"` // Skip entries that would fail due to divergence
	DryRun              bool `json:"dryRun"`              // Validate but don't actually resume

	// File handling
	RestoreFromCheckpoint bool `json:"restoreFromCheckpoint"` // Restore files to checkpoint state before resuming
	BackupBeforeResume    bool `json:"backupBeforeResume"`    // Create backup before any changes
}

// DefaultResumeOptions returns safe default options
func DefaultResumeOptions() *ResumeOptions {
	return &ResumeOptions{
		UseLatestCheckpoint:   true,
		StrictValidation:      true,
		AllowRepair:           false,
		ValidateAllFiles:      true,
		SkipDivergedEntries:   false,
		DryRun:                false,
		RestoreFromCheckpoint: false,
		BackupBeforeResume:    true,
	}
}

// =============================================================================
// RESUME ALGORITHM IMPLEMENTATION
// =============================================================================

// ResumeFromCheckpoint attempts to safely resume from a checkpoint
func ResumeFromCheckpoint(journal *RunJournal, options *ResumeOptions) (*ResumeResult, error) {
	if options == nil {
		options = DefaultResumeOptions()
	}

	result := &ResumeResult{
		Divergences:   []ResumeDivergence{},
		RepairActions: []RepairAction{},
	}

	// =========================================================================
	// STEP 1: Find the checkpoint to resume from
	// =========================================================================
	checkpoint, err := selectCheckpoint(journal, options)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to select checkpoint: %v", err)
		return result, err
	}

	result.ResumedFrom = checkpoint.Name
	result.ResumedAtEntry = checkpoint.EntryIndex

	// =========================================================================
	// STEP 2: Validate journal integrity up to checkpoint
	// =========================================================================
	if !validateJournalIntegrity(journal, checkpoint) {
		result.Error = "Journal integrity check failed - entries may be corrupted"
		return result, fmt.Errorf("journal integrity check failed")
	}

	// =========================================================================
	// STEP 3: Validate current file state against checkpoint
	// =========================================================================
	report, divergences := validateFileState(journal, checkpoint, options)
	result.ValidationReport = report
	result.Divergences = divergences

	// =========================================================================
	// STEP 4: Handle divergences
	// =========================================================================
	if len(divergences) > 0 {
		if options.StrictValidation {
			// Strict mode: any divergence is fatal
			result.Error = fmt.Sprintf("Found %d divergences in strict mode", len(divergences))
			return result, fmt.Errorf("divergences detected in strict mode")
		}

		if options.AllowRepair {
			// Attempt to repair divergences
			repairActions := attemptRepair(journal, checkpoint, divergences, options)
			result.RepairActions = repairActions

			// Check if repair succeeded
			for _, action := range repairActions {
				if !action.Success {
					result.Error = fmt.Sprintf("Repair failed: %s", action.Error)
					return result, fmt.Errorf("repair failed: %s", action.Error)
				}
			}
		} else if !options.SkipDivergedEntries {
			// Can't repair, can't skip - fail
			result.Error = "Divergences detected and repair not allowed"
			return result, fmt.Errorf("divergences detected, repair not allowed")
		}
	}

	// =========================================================================
	// STEP 5: Dry run check
	// =========================================================================
	if options.DryRun {
		result.Success = true
		result.ValidationReport.SafeToResume = true
		return result, nil
	}

	// =========================================================================
	// STEP 6: Create backup if requested
	// =========================================================================
	if options.BackupBeforeResume {
		if err := createPreResumeBackup(journal, checkpoint); err != nil {
			result.Error = fmt.Sprintf("Failed to create backup: %v", err)
			return result, err
		}
	}

	// =========================================================================
	// STEP 7: Restore files to checkpoint state if requested
	// =========================================================================
	if options.RestoreFromCheckpoint {
		if err := restoreFilesToCheckpoint(journal, checkpoint); err != nil {
			result.Error = fmt.Sprintf("Failed to restore files: %v", err)
			return result, err
		}
	}

	// =========================================================================
	// STEP 8: Update journal state to resume position
	// =========================================================================
	if err := journal.ResumeFromEntry(checkpoint.EntryIndex); err != nil {
		result.Error = fmt.Sprintf("Failed to update journal state: %v", err)
		return result, err
	}

	result.Success = true
	result.ValidationReport.SafeToResume = true
	return result, nil
}

// =============================================================================
// CHECKPOINT SELECTION
// =============================================================================

func selectCheckpoint(journal *RunJournal, options *ResumeOptions) (*Checkpoint, error) {
	if len(journal.Checkpoints) == 0 {
		return nil, fmt.Errorf("no checkpoints available")
	}

	// If specific checkpoint requested
	if options.CheckpointName != "" {
		cp, exists := journal.Checkpoints[options.CheckpointName]
		if !exists {
			return nil, fmt.Errorf("checkpoint not found: %s", options.CheckpointName)
		}
		return cp, nil
	}

	// Find latest checkpoint
	if options.UseLatestCheckpoint {
		return findLatestCheckpoint(journal)
	}

	// Find last "good" checkpoint (one where all prior entries completed successfully)
	return findLastGoodCheckpoint(journal)
}

func findLatestCheckpoint(journal *RunJournal) (*Checkpoint, error) {
	var latest *Checkpoint
	var latestTime time.Time

	for _, cp := range journal.Checkpoints {
		if latest == nil || cp.CreatedAt.After(latestTime) {
			latest = cp
			latestTime = cp.CreatedAt
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("no checkpoints found")
	}
	return latest, nil
}

func findLastGoodCheckpoint(journal *RunJournal) (*Checkpoint, error) {
	// Sort checkpoints by entry index (descending)
	var checkpoints []*Checkpoint
	for _, cp := range journal.Checkpoints {
		checkpoints = append(checkpoints, cp)
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].EntryIndex > checkpoints[j].EntryIndex
	})

	// Find the latest checkpoint where all entries up to it completed successfully
	for _, cp := range checkpoints {
		if isCheckpointGood(journal, cp) {
			return cp, nil
		}
	}

	return nil, fmt.Errorf("no good checkpoint found")
}

func isCheckpointGood(journal *RunJournal, checkpoint *Checkpoint) bool {
	// All entries up to checkpoint must be completed (not failed/pending)
	for i := 0; i < checkpoint.EntryIndex && i < len(journal.Entries); i++ {
		entry := journal.Entries[i]
		if entry.Status == EntryStatusFailed {
			return false
		}
		if entry.Status == EntryStatusPending || entry.Status == EntryStatusRunning {
			return false
		}
	}
	return true
}

// =============================================================================
// JOURNAL INTEGRITY VALIDATION
// =============================================================================

func validateJournalIntegrity(journal *RunJournal, checkpoint *Checkpoint) bool {
	// Verify hash chain up to checkpoint
	computedHash := journal.ComputeHashUpTo(checkpoint.EntryIndex)
	return computedHash == checkpoint.JournalHash
}

// =============================================================================
// FILE STATE VALIDATION
// =============================================================================

func validateFileState(journal *RunJournal, checkpoint *Checkpoint, options *ResumeOptions) (*ValidationReport, []ResumeDivergence) {
	report := &ValidationReport{
		CheckpointName:  checkpoint.Name,
		CheckpointEntry: checkpoint.EntryIndex,
		ValidationTime:  time.Now(),
		FileStates:      make(map[string]string),
	}

	var divergences []ResumeDivergence

	// Determine which files to validate
	filesToValidate := checkpoint.FileStates
	if options.ValidateAllFiles {
		// Also include files tracked in journal
		for path := range journal.FileStates {
			if _, exists := filesToValidate[path]; !exists {
				filesToValidate[path] = ""
			}
		}
	}

	report.FilesValidated = len(filesToValidate)

	// Validate each file
	for path, expectedHash := range filesToValidate {
		actualHash, exists, err := getFileHash(path)

		if err != nil {
			divergences = append(divergences, ResumeDivergence{
				Type:         "file_error",
				Path:         path,
				Expected:     expectedHash,
				Actual:       fmt.Sprintf("error: %v", err),
				Severity:     "error",
				Recoverable:  false,
				SuggestedFix: "Check file permissions and disk space",
			})
			report.FileStates[path] = "error"
			continue
		}

		if !exists && expectedHash != "" {
			// File should exist but doesn't
			divergences = append(divergences, ResumeDivergence{
				Type:         "file_missing",
				Path:         path,
				Expected:     expectedHash,
				Actual:       "",
				Severity:     "error",
				Recoverable:  true,
				SuggestedFix: "Restore file from checkpoint or backup",
			})
			report.FileStates[path] = "missing"
			report.FilesDiverged++
			continue
		}

		if exists && expectedHash == "" {
			// File exists but shouldn't (wasn't tracked at checkpoint)
			divergences = append(divergences, ResumeDivergence{
				Type:         "file_extra",
				Path:         path,
				Expected:     "",
				Actual:       actualHash,
				Severity:     "warning",
				Recoverable:  true,
				SuggestedFix: "File was created after checkpoint - consider if it should be kept",
			})
			report.FileStates[path] = "extra"
			report.FilesDiverged++
			continue
		}

		if actualHash != expectedHash {
			// File exists but content differs
			divergences = append(divergences, ResumeDivergence{
				Type:         "hash_mismatch",
				Path:         path,
				Expected:     expectedHash,
				Actual:       actualHash,
				Severity:     "error",
				Recoverable:  true,
				SuggestedFix: "File was modified - restore from checkpoint or accept current state",
			})
			report.FileStates[path] = "modified"
			report.FilesDiverged++
			continue
		}

		// File matches expected state
		report.FilesMatched++
		report.FileStates[path] = "ok"
	}

	report.JournalIntegrity = true // Already validated above
	report.SafeToResume = len(divergences) == 0

	return report, divergences
}

func getFileHash(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	h := sha256.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil)), true, nil
}

// =============================================================================
// DIVERGENCE REPAIR
// =============================================================================

func attemptRepair(journal *RunJournal, checkpoint *Checkpoint, divergences []ResumeDivergence, options *ResumeOptions) []RepairAction {
	var actions []RepairAction

	for _, div := range divergences {
		if !div.Recoverable {
			actions = append(actions, RepairAction{
				Type:        "skip",
				Path:        div.Path,
				Description: fmt.Sprintf("Cannot repair: %s", div.Type),
				PerformedAt: time.Now(),
				Success:     false,
				Error:       "Not recoverable",
			})
			continue
		}

		switch div.Type {
		case "file_missing":
			action := repairMissingFile(journal, checkpoint, div.Path)
			actions = append(actions, action)

		case "hash_mismatch":
			if options.RestoreFromCheckpoint {
				action := restoreFileFromCheckpoint(journal, checkpoint, div.Path)
				actions = append(actions, action)
			} else {
				actions = append(actions, RepairAction{
					Type:        "accept_current",
					Path:        div.Path,
					Description: "Accepting current file state (RestoreFromCheckpoint=false)",
					PerformedAt: time.Now(),
					Success:     true,
				})
			}

		case "file_extra":
			// Extra files are usually fine to keep
			actions = append(actions, RepairAction{
				Type:        "keep_extra",
				Path:        div.Path,
				Description: "Keeping extra file (created after checkpoint)",
				PerformedAt: time.Now(),
				Success:     true,
			})
		}
	}

	return actions
}

func repairMissingFile(journal *RunJournal, checkpoint *Checkpoint, path string) RepairAction {
	action := RepairAction{
		Type:        "restore_file",
		Path:        path,
		Description: "Restoring missing file from checkpoint",
		PerformedAt: time.Now(),
	}

	// Try to find content in checkpoint's FileContents
	if checkpoint.FileContents != nil {
		if content, exists := checkpoint.FileContents[path]; exists {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				action.Success = false
				action.Error = err.Error()
			} else {
				action.Success = true
			}
			return action
		}
	}

	// Try to find in journal's file states
	if fileState := journal.GetFileState(path); fileState != nil {
		// We only have hash, not content - can't restore
		action.Success = false
		action.Error = "File content not available in checkpoint"
	} else {
		action.Success = false
		action.Error = "File not tracked in journal"
	}

	return action
}

func restoreFileFromCheckpoint(journal *RunJournal, checkpoint *Checkpoint, path string) RepairAction {
	action := RepairAction{
		Type:        "restore_file",
		Path:        path,
		Description: "Restoring file to checkpoint state",
		PerformedAt: time.Now(),
	}

	if checkpoint.FileContents == nil {
		action.Success = false
		action.Error = "Checkpoint does not contain file contents"
		return action
	}

	content, exists := checkpoint.FileContents[path]
	if !exists {
		action.Success = false
		action.Error = "File content not found in checkpoint"
		return action
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		action.Success = false
		action.Error = err.Error()
	} else {
		action.Success = true
	}

	return action
}

// =============================================================================
// BACKUP AND RESTORE
// =============================================================================

func createPreResumeBackup(journal *RunJournal, checkpoint *Checkpoint) error {
	backupName := fmt.Sprintf("pre_resume_%s_%d", checkpoint.Name, time.Now().Unix())
	journal.CreateCheckpoint(backupName, "Automatic backup before resume", true)
	return nil
}

func restoreFilesToCheckpoint(journal *RunJournal, checkpoint *Checkpoint) error {
	if checkpoint.FileContents == nil {
		return fmt.Errorf("checkpoint does not contain file contents for restoration")
	}

	for path, content := range checkpoint.FileContents {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to restore %s: %w", path, err)
		}
	}

	return nil
}

// =============================================================================
// SAFE RESUME ALGORITHM (HIGH-LEVEL)
// =============================================================================
//
// The algorithm follows these steps:
//
// 1. SELECT CHECKPOINT
//    ├─ Use specified checkpoint name, OR
//    ├─ Use latest checkpoint, OR
//    └─ Find last "good" checkpoint (all prior entries succeeded)
//
// 2. VALIDATE JOURNAL INTEGRITY
//    └─ Verify hash chain from start to checkpoint
//       └─ If corrupted → ABORT
//
// 3. VALIDATE FILE STATE
//    ├─ For each file in checkpoint:
//    │   ├─ File exists? Content matches hash?
//    │   └─ Record divergences
//    └─ If strict mode and divergences → ABORT
//
// 4. HANDLE DIVERGENCES (if any)
//    ├─ If repair allowed:
//    │   ├─ Missing file → Restore from checkpoint
//    │   ├─ Modified file → Restore or accept current
//    │   └─ Extra file → Keep (warning only)
//    └─ If repair failed → ABORT
//
// 5. CREATE BACKUP (optional)
//    └─ Checkpoint current state before any changes
//
// 6. RESTORE FILES (optional)
//    └─ Overwrite current files with checkpoint state
//
// 7. UPDATE JOURNAL STATE
//    ├─ Set current entry to checkpoint position
//    ├─ Mark status as "recording"
//    └─ Increment resume count
//
// 8. RESUME EXECUTION
//    └─ Continue from next pending entry
//
// =============================================================================

// =============================================================================
// USAGE EXAMPLE
// =============================================================================
//
// // Load journal from disk
// journal, _ := FromJSON(journalData)
//
// // Configure resume options
// options := &ResumeOptions{
//     UseLatestCheckpoint:   true,
//     StrictValidation:      false,  // Allow some divergence
//     AllowRepair:           true,   // Try to fix issues
//     BackupBeforeResume:    true,   // Safety first
//     RestoreFromCheckpoint: false,  // Keep current file state
// }
//
// // Attempt resume
// result, err := ResumeFromCheckpoint(journal, options)
// if err != nil {
//     log.Printf("Resume failed: %v", err)
//     log.Printf("Divergences: %+v", result.Divergences)
//     return
// }
//
// log.Printf("Resumed from checkpoint: %s (entry %d)",
//     result.ResumedFrom, result.ResumedAtEntry)
//
// // Continue execution
// for journal.HasMoreEntries() {
//     entry := journal.GetNextPendingEntry()
//     // ... execute entry
// }
//
// =============================================================================
