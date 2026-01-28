package shared

import (
	"fmt"
	"strings"
)

// =============================================================================
// UNRECOVERABLE ERROR CLASSIFICATION
// =============================================================================

// UnrecoverableReason identifies why recovery is impossible
type UnrecoverableReason string

const (
	// Provider-related unrecoverable reasons
	UnrecoverableQuotaExhausted   UnrecoverableReason = "quota_exhausted"
	UnrecoverableAuthInvalid      UnrecoverableReason = "auth_invalid"
	UnrecoverablePermissionDenied UnrecoverableReason = "permission_denied"
	UnrecoverableContentPolicy    UnrecoverableReason = "content_policy"
	UnrecoverableModelNotFound    UnrecoverableReason = "model_not_found"
	UnrecoverableContextTooLong   UnrecoverableReason = "context_too_long"

	// Data loss unrecoverable reasons
	UnrecoverableCheckpointLost   UnrecoverableReason = "checkpoint_lost"
	UnrecoverableJournalCorrupted UnrecoverableReason = "journal_corrupted"
	UnrecoverableSnapshotMissing  UnrecoverableReason = "snapshot_missing"
	UnrecoverableFileContentLost  UnrecoverableReason = "file_content_lost"
	UnrecoverableWALCorrupted     UnrecoverableReason = "wal_corrupted"

	// External state unrecoverable reasons
	UnrecoverableExternalModification UnrecoverableReason = "external_modification"
	UnrecoverableConcurrentAccess     UnrecoverableReason = "concurrent_access"
	UnrecoverableResourceDeleted      UnrecoverableReason = "resource_deleted"

	// System-level unrecoverable reasons
	UnrecoverableDiskFull         UnrecoverableReason = "disk_full"
	UnrecoverablePermissionError  UnrecoverableReason = "permission_error"
	UnrecoverableNetworkPartition UnrecoverableReason = "network_partition"
)

// UnrecoverableError represents an error from which automatic recovery is impossible
type UnrecoverableError struct {
	// Reason identifies the specific unrecoverable condition
	Reason UnrecoverableReason `json:"reason"`

	// Category groups related reasons
	Category UnrecoverableCategory `json:"category"`

	// Message is a human-readable description
	Message string `json:"message"`

	// TechnicalDetails provides debugging information
	TechnicalDetails string `json:"technicalDetails,omitempty"`

	// AffectedResources lists what was impacted
	AffectedResources []string `json:"affectedResources,omitempty"`

	// PartialRecoveryPossible indicates if some data can be salvaged
	PartialRecoveryPossible bool `json:"partialRecoveryPossible"`

	// PartialRecoverySteps describes what can be recovered
	PartialRecoverySteps []string `json:"partialRecoverySteps,omitempty"`

	// UserActions lists required user interventions
	UserActions []UserAction `json:"userActions"`

	// DataLossDescription explains what data was lost (if any)
	DataLossDescription string `json:"dataLossDescription,omitempty"`

	// PreventionAdvice explains how to avoid this in the future
	PreventionAdvice []string `json:"preventionAdvice,omitempty"`
}

// UnrecoverableCategory groups unrecoverable reasons
type UnrecoverableCategory string

const (
	CategoryProviderLimit  UnrecoverableCategory = "provider_limit"
	CategoryAuthentication UnrecoverableCategory = "authentication"
	CategoryDataLoss       UnrecoverableCategory = "data_loss"
	CategoryExternalState  UnrecoverableCategory = "external_state"
	CategorySystemResource UnrecoverableCategory = "system_resource"
)

// UserAction describes what the user must do
type UserAction struct {
	Description string `json:"description"`
	Priority    string `json:"priority"` // critical, high, medium, low
	Command     string `json:"command,omitempty"`
	Link        string `json:"link,omitempty"`
	Automated   bool   `json:"automated"` // Can this be done via CLI?
}

// =============================================================================
// EDGE CASE DEFINITIONS
// =============================================================================

// GetUnrecoverableEdgeCases returns all known unrecoverable scenarios
func GetUnrecoverableEdgeCases() []UnrecoverableEdgeCase {
	return []UnrecoverableEdgeCase{
		// =================================================================
		// PROVIDER LIMIT EDGE CASES
		// =================================================================
		{
			Reason:      UnrecoverableQuotaExhausted,
			Category:    CategoryProviderLimit,
			Title:       "API Quota Exhausted",
			Description: "The API quota for your account has been exhausted. This is different from rate limiting - the account has reached its spending or usage cap.",
			Scenarios: []string{
				"OpenAI: \"You exceeded your current quota\"",
				"Anthropic: Monthly usage limit reached",
				"Google: Daily quota exhausted",
				"OpenRouter: Insufficient credits",
			},
			WhyUnrecoverable: "No amount of retrying will succeed until the quota is replenished or the billing limit is increased.",
			DataAtRisk:       "None - no data loss occurs, but in-progress work cannot complete",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Transaction can be rolled back to last checkpoint",
				Steps: []string{
					"Rollback current transaction to restore file state",
					"Save journal for later resume",
					"Export partial results if any",
				},
			},
			UserActions: []UserAction{
				{Description: "Add credits to your account", Priority: "critical", Automated: false},
				{Description: "Increase spending limit in provider console", Priority: "critical", Automated: false},
				{Description: "Switch to a different provider", Priority: "high", Command: "plandex providers switch <provider>", Automated: true},
			},
			Prevention: []string{
				"Monitor usage with provider dashboards",
				"Set up billing alerts",
				"Configure provider fallbacks",
			},
		},
		{
			Reason:      UnrecoverableContextTooLong,
			Category:    CategoryProviderLimit,
			Title:       "Context Length Exceeded",
			Description: "The combined input (system prompt, conversation history, context files) exceeds the model's maximum context window.",
			Scenarios: []string{
				"Large codebase loaded into context",
				"Long conversation history accumulated",
				"Multiple large files in context simultaneously",
			},
			WhyUnrecoverable: "The model physically cannot process more tokens than its context window allows. No retry strategy will help.",
			DataAtRisk:       "None - request never started processing",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Reduce context and retry",
				Steps: []string{
					"Remove unnecessary files from context",
					"Summarize conversation history",
					"Use a model with larger context window",
				},
			},
			UserActions: []UserAction{
				{Description: "Remove files from context", Priority: "high", Command: "plandex context rm <file>", Automated: true},
				{Description: "Clear conversation history", Priority: "medium", Command: "plandex convo clear", Automated: true},
				{Description: "Switch to larger context model", Priority: "medium", Command: "plandex models set --min-context 128000", Automated: true},
			},
			Prevention: []string{
				"Use selective context loading",
				"Implement context summarization",
				"Monitor token usage before requests",
			},
		},
		{
			Reason:      UnrecoverableContentPolicy,
			Category:    CategoryProviderLimit,
			Title:       "Content Policy Violation",
			Description: "The request was rejected because it violated the provider's content policy.",
			Scenarios: []string{
				"Prompt contains restricted content",
				"Generated output triggered safety filters",
				"Azure's stricter content filtering activated",
			},
			WhyUnrecoverable: "The same content will always be rejected. Retrying with identical input will fail.",
			DataAtRisk:       "None - request rejected before processing",
			PartialRecovery:  nil, // No partial recovery
			UserActions: []UserAction{
				{Description: "Modify the prompt to remove policy-violating content", Priority: "critical", Automated: false},
				{Description: "Review provider's content policy guidelines", Priority: "high", Automated: false},
			},
			Prevention: []string{
				"Review content policies before using sensitive topics",
				"Use content pre-screening if available",
			},
		},

		// =================================================================
		// AUTHENTICATION EDGE CASES
		// =================================================================
		{
			Reason:      UnrecoverableAuthInvalid,
			Category:    CategoryAuthentication,
			Title:       "Invalid API Credentials",
			Description: "The API key or authentication token is invalid, expired, or revoked.",
			Scenarios: []string{
				"API key was rotated but not updated",
				"API key was revoked due to security incident",
				"OAuth token expired",
				"Service account credentials invalid",
			},
			WhyUnrecoverable: "Authentication cannot succeed without valid credentials. The system cannot fix credential issues.",
			DataAtRisk:       "In-progress transaction will be rolled back. No permanent data loss.",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Transaction rolled back, can resume after fixing credentials",
				Steps: []string{
					"Current transaction automatically rolled back",
					"Journal state preserved",
					"Resume from checkpoint after updating credentials",
				},
			},
			UserActions: []UserAction{
				{Description: "Update API key", Priority: "critical", Command: "plandex api-keys set <provider>", Automated: true},
				{Description: "Verify key in provider console", Priority: "high", Automated: false},
				{Description: "Check for security notifications from provider", Priority: "high", Automated: false},
			},
			Prevention: []string{
				"Use environment variables for API keys",
				"Set up key rotation reminders",
				"Monitor for security alerts",
			},
		},

		// =================================================================
		// DATA LOSS EDGE CASES
		// =================================================================
		{
			Reason:      UnrecoverableCheckpointLost,
			Category:    CategoryDataLoss,
			Title:       "Checkpoint Data Lost",
			Description: "The checkpoint data needed for recovery has been lost or corrupted.",
			Scenarios: []string{
				"Checkpoint file deleted externally",
				"Storage corruption",
				"Incomplete checkpoint write due to crash",
				"Checkpoint pruned by cleanup process",
			},
			WhyUnrecoverable: "Without checkpoint data, the system cannot restore to a known good state. The recovery point no longer exists.",
			DataAtRisk:       "All changes since last available checkpoint may be inconsistent",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Fall back to earlier checkpoint if available",
				Steps: []string{
					"List available checkpoints",
					"Select most recent valid checkpoint",
					"Accept loss of work between checkpoints",
				},
			},
			UserActions: []UserAction{
				{Description: "List available checkpoints", Priority: "critical", Command: "plandex checkpoints list", Automated: true},
				{Description: "Restore from backup if available", Priority: "high", Automated: false},
				{Description: "Manually verify file states", Priority: "high", Automated: false},
			},
			Prevention: []string{
				"Enable checkpoint redundancy",
				"Configure external backups",
				"Don't manually delete .plandex directories",
			},
		},
		{
			Reason:      UnrecoverableJournalCorrupted,
			Category:    CategoryDataLoss,
			Title:       "Journal Corrupted",
			Description: "The run journal has been corrupted and cannot be parsed or trusted.",
			Scenarios: []string{
				"Disk write failed mid-journal-update",
				"File system corruption",
				"Manual editing of journal file",
				"Incompatible version migration",
			},
			WhyUnrecoverable: "The journal is the source of truth for execution history. If corrupted, the system cannot determine what operations completed.",
			DataAtRisk:       "Execution history lost. File states may be inconsistent with expected state.",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Reconstruct from WAL if available",
				Steps: []string{
					"Attempt WAL replay",
					"Validate file states against git",
					"Create new journal from current state",
				},
			},
			UserActions: []UserAction{
				{Description: "Attempt journal recovery", Priority: "critical", Command: "plandex recover journal", Automated: true},
				{Description: "Restore journal from backup", Priority: "high", Automated: false},
				{Description: "Start fresh with current file state", Priority: "medium", Command: "plandex reset --keep-files", Automated: true},
			},
			Prevention: []string{
				"Enable WAL for crash recovery",
				"Use reliable storage",
				"Don't manually edit plandex files",
			},
		},
		{
			Reason:      UnrecoverableSnapshotMissing,
			Category:    CategoryDataLoss,
			Title:       "File Snapshot Missing",
			Description: "The original file content snapshot needed for rollback is missing. Snapshots are persisted to disk (as .snapshot + .meta.json files) before any write begins, so this condition is rare under normal operation.",
			Scenarios: []string{
				"Crash during the captureSnapshot write itself (narrow window)",
				"Manual deletion of .plandex/snapshots directory",
				"Storage failure or disk corruption after snapshot was written",
			},
			WhyUnrecoverable: "Rollback requires the original file content. Without the snapshot, the system cannot restore the file to its pre-transaction state.",
			DataAtRisk:       "Original file content cannot be recovered through the transaction system. WAL may still be intact for partial diagnosis.",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Recover from git or external backup",
				Steps: []string{
					"Check git for file history (git log -- <file>)",
					"Restore from external backup",
					"Accept current state as new baseline",
				},
			},
			UserActions: []UserAction{
				{Description: "Restore file from git", Priority: "critical", Command: "git checkout HEAD -- <file>", Automated: true},
				{Description: "Restore from backup", Priority: "high", Automated: false},
				{Description: "Accept current state", Priority: "medium", Command: "plandex accept-current-state", Automated: true},
			},
			Prevention: []string{
				"Always commit to git before plandex operations",
				"Do not manually delete .plandex/snapshots directories",
				"Use reliable storage (avoid network filesystems for project directories)",
			},
		},
		{
			Reason:      UnrecoverableFileContentLost,
			Category:    CategoryDataLoss,
			Title:       "Checkpoint File Content Lost",
			Description: "The checkpoint recorded file hashes but not file contents, and files have changed.",
			Scenarios: []string{
				"Checkpoint created without content storage to save space",
				"File modified externally after checkpoint",
				"Content storage corrupted",
			},
			WhyUnrecoverable: "The checkpoint knows what the file should look like (hash) but doesn't have the actual content to restore it.",
			DataAtRisk:       "Cannot restore files to checkpoint state",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Use git or find matching content elsewhere",
				Steps: []string{
					"Search git history for matching hash",
					"Check other checkpoints for content",
					"Manual reconstruction if logic is understood",
				},
			},
			UserActions: []UserAction{
				{Description: "Search git for file version", Priority: "high", Command: "git log --all -p -- <file> | grep -A 100 <hash>", Automated: false},
				{Description: "Manually restore file content", Priority: "medium", Automated: false},
				{Description: "Skip this file and continue", Priority: "low", Command: "plandex resume --skip-missing", Automated: true},
			},
			Prevention: []string{
				"Enable full content storage in checkpoints",
				"Commit to git before major operations",
				"Use incremental checkpoints with content",
			},
		},

		// =================================================================
		// EXTERNAL STATE EDGE CASES
		// =================================================================
		{
			Reason:      UnrecoverableExternalModification,
			Category:    CategoryExternalState,
			Title:       "External File Modification",
			Description: "Files were modified by an external process during the operation, causing state divergence.",
			Scenarios: []string{
				"IDE auto-save modified file during plandex operation",
				"Another developer pushed changes",
				"Build process modified generated files",
				"File sync service (Dropbox, etc.) caused conflict",
			},
			WhyUnrecoverable: "The system cannot determine which changes to keep - the external changes or the plandex changes. Human judgment required.",
			DataAtRisk:       "Either external changes or plandex changes will be lost depending on resolution",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Merge changes manually",
				Steps: []string{
					"View diff between versions",
					"Manually merge changes",
					"Create new checkpoint from merged state",
				},
			},
			UserActions: []UserAction{
				{Description: "View divergence details", Priority: "critical", Command: "plandex divergence show", Automated: true},
				{Description: "Accept external changes", Priority: "high", Command: "plandex resolve --accept-external", Automated: true},
				{Description: "Accept plandex changes", Priority: "high", Command: "plandex resolve --accept-plandex", Automated: true},
				{Description: "Merge manually", Priority: "high", Automated: false},
			},
			Prevention: []string{
				"Avoid editing files while plandex is running",
				"Use file locking if available",
				"Coordinate with team during plandex sessions",
			},
		},
		{
			Reason:      UnrecoverableConcurrentAccess,
			Category:    CategoryExternalState,
			Title:       "Concurrent Plandex Session",
			Description: "Another plandex session is operating on the same plan, causing conflicts.",
			Scenarios: []string{
				"Same plan open in multiple terminals",
				"Shared plan accessed by multiple users simultaneously",
				"Background process still running from previous session",
			},
			WhyUnrecoverable: "Two sessions modifying the same state will corrupt the journal and file states. One must yield.",
			DataAtRisk:       "Both sessions' changes may be partially applied, creating inconsistent state",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Stop one session and reconcile",
				Steps: []string{
					"Identify all active sessions",
					"Stop all but one session",
					"Reconcile state from surviving session",
				},
			},
			UserActions: []UserAction{
				{Description: "List active sessions", Priority: "critical", Command: "plandex sessions list", Automated: true},
				{Description: "Force stop other sessions", Priority: "high", Command: "plandex sessions kill <session-id>", Automated: true},
				{Description: "Reconcile state", Priority: "high", Command: "plandex reconcile", Automated: true},
			},
			Prevention: []string{
				"Use session locking",
				"Check for active sessions before starting",
				"Use separate plans for parallel work",
			},
		},

		// =================================================================
		// SYSTEM RESOURCE EDGE CASES
		// =================================================================
		{
			Reason:      UnrecoverableDiskFull,
			Category:    CategorySystemResource,
			Title:       "Disk Space Exhausted",
			Description: "The disk is full and the system cannot write files, snapshots, or journal entries.",
			Scenarios: []string{
				"Large files filled disk during operation",
				"Log files consumed available space",
				"Snapshot storage exceeded available space",
			},
			WhyUnrecoverable: "Without disk space, the system cannot write rollback data, making recovery unsafe. Any write operation could fail.",
			DataAtRisk:       "Current transaction state may be partially written. Rollback may fail.",
			PartialRecovery: &PartialRecoveryOption{
				Possible:    true,
				Description: "Free space and attempt recovery",
				Steps: []string{
					"Free disk space immediately",
					"Attempt to complete rollback",
					"Verify file integrity",
				},
			},
			UserActions: []UserAction{
				{Description: "Free disk space", Priority: "critical", Automated: false},
				{Description: "Clear plandex cache", Priority: "high", Command: "plandex cache clear", Automated: true},
				{Description: "Remove old checkpoints", Priority: "medium", Command: "plandex checkpoints prune --keep 5", Automated: true},
			},
			Prevention: []string{
				"Monitor disk space",
				"Configure checkpoint retention limits",
				"Use separate volume for plandex data",
			},
		},
	}
}

// UnrecoverableEdgeCase documents a specific unrecoverable scenario
type UnrecoverableEdgeCase struct {
	Reason           UnrecoverableReason
	Category         UnrecoverableCategory
	Title            string
	Description      string
	Scenarios        []string
	WhyUnrecoverable string
	DataAtRisk       string
	PartialRecovery  *PartialRecoveryOption
	UserActions      []UserAction
	Prevention       []string
}

// PartialRecoveryOption describes what can be salvaged
type PartialRecoveryOption struct {
	Possible    bool
	Description string
	Steps       []string
}

// =============================================================================
// DETECTION AND COMMUNICATION
// =============================================================================

// DetectUnrecoverableCondition checks if an error represents an unrecoverable state
func DetectUnrecoverableCondition(report *ErrorReport) *UnrecoverableError {
	if report == nil || report.RootCause == nil {
		return nil
	}

	// Check provider failures
	if report.RootCause.ProviderFailure != nil {
		return detectProviderUnrecoverable(report.RootCause.ProviderFailure)
	}

	// Check file system errors
	if report.RootCause.Category == ErrorCategoryFileSystem {
		return detectFileSystemUnrecoverable(report)
	}

	return nil
}

func detectProviderUnrecoverable(failure *ProviderFailure) *UnrecoverableError {
	if failure.Category == FailureCategoryRetryable {
		return nil // Retryable failures are recoverable
	}

	var reason UnrecoverableReason
	var category UnrecoverableCategory

	switch failure.Type {
	case FailureTypeQuotaExhausted:
		reason = UnrecoverableQuotaExhausted
		category = CategoryProviderLimit
	case FailureTypeAuthInvalid:
		reason = UnrecoverableAuthInvalid
		category = CategoryAuthentication
	case FailureTypePermissionDenied:
		reason = UnrecoverablePermissionDenied
		category = CategoryAuthentication
	case FailureTypeContentPolicy:
		reason = UnrecoverableContentPolicy
		category = CategoryProviderLimit
	case FailureTypeContextTooLong:
		reason = UnrecoverableContextTooLong
		category = CategoryProviderLimit
	case FailureTypeModelNotFound:
		reason = UnrecoverableModelNotFound
		category = CategoryProviderLimit
	default:
		return nil
	}

	edgeCase := findEdgeCase(reason)
	if edgeCase == nil {
		return nil
	}

	return &UnrecoverableError{
		Reason:                  reason,
		Category:                category,
		Message:                 failure.Message,
		TechnicalDetails:        fmt.Sprintf("HTTP %d from %s", failure.HTTPCode, failure.Provider),
		PartialRecoveryPossible: edgeCase.PartialRecovery != nil && edgeCase.PartialRecovery.Possible,
		PartialRecoverySteps:    getPartialRecoverySteps(edgeCase),
		UserActions:             edgeCase.UserActions,
		PreventionAdvice:        edgeCase.Prevention,
	}
}

func detectFileSystemUnrecoverable(report *ErrorReport) *UnrecoverableError {
	errType := report.RootCause.Type

	switch errType {
	case "disk_full":
		return &UnrecoverableError{
			Reason:                  UnrecoverableDiskFull,
			Category:                CategorySystemResource,
			Message:                 report.RootCause.Message,
			AffectedResources:       []string{report.StepContext.FilePath},
			PartialRecoveryPossible: true,
			PartialRecoverySteps: []string{
				"Free disk space",
				"Retry operation",
			},
			UserActions: []UserAction{
				{Description: "Free disk space immediately", Priority: "critical"},
			},
		}
	case "permission_denied":
		return &UnrecoverableError{
			Reason:                  UnrecoverablePermissionError,
			Category:                CategorySystemResource,
			Message:                 report.RootCause.Message,
			AffectedResources:       []string{report.StepContext.FilePath},
			PartialRecoveryPossible: true,
			PartialRecoverySteps: []string{
				"Fix file permissions",
				"Retry operation",
			},
			UserActions: []UserAction{
				{Description: "Fix file permissions", Priority: "critical", Command: fmt.Sprintf("chmod 644 %s", report.StepContext.FilePath)},
			},
		}
	}

	return nil
}

func findEdgeCase(reason UnrecoverableReason) *UnrecoverableEdgeCase {
	for _, ec := range GetUnrecoverableEdgeCases() {
		if ec.Reason == reason {
			return &ec
		}
	}
	return nil
}

func getPartialRecoverySteps(ec *UnrecoverableEdgeCase) []string {
	if ec.PartialRecovery == nil {
		return nil
	}
	return ec.PartialRecovery.Steps
}

// =============================================================================
// USER COMMUNICATION
// =============================================================================

// FormatUnrecoverableError creates a user-friendly message explaining the unrecoverable state
func (e *UnrecoverableError) Format() string {
	var sb strings.Builder

	sb.WriteString("╔═══════════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║                    UNRECOVERABLE ERROR                            ║\n")
	sb.WriteString("╚═══════════════════════════════════════════════════════════════════╝\n\n")

	// What happened
	sb.WriteString("▌ WHAT HAPPENED\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("│ %s\n", e.Message))
	if e.TechnicalDetails != "" {
		sb.WriteString(fmt.Sprintf("│ Technical: %s\n", e.TechnicalDetails))
	}
	sb.WriteString("│\n")

	// Why it can't be auto-recovered
	sb.WriteString("▌ WHY AUTOMATIC RECOVERY IS NOT POSSIBLE\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("│ Reason: %s\n", e.Reason))
	sb.WriteString(fmt.Sprintf("│ Category: %s\n", e.Category))
	sb.WriteString("│\n")

	// Data at risk
	if e.DataLossDescription != "" {
		sb.WriteString("▌ DATA AT RISK\n")
		sb.WriteString("├─────────────────────────────────────────────────────────────────────\n")
		sb.WriteString(fmt.Sprintf("│ %s\n", e.DataLossDescription))
		if len(e.AffectedResources) > 0 {
			sb.WriteString("│ Affected:\n")
			for _, r := range e.AffectedResources {
				sb.WriteString(fmt.Sprintf("│   • %s\n", r))
			}
		}
		sb.WriteString("│\n")
	}

	// Partial recovery
	if e.PartialRecoveryPossible {
		sb.WriteString("▌ PARTIAL RECOVERY AVAILABLE\n")
		sb.WriteString("├─────────────────────────────────────────────────────────────────────\n")
		sb.WriteString("│ Some recovery is possible:\n")
		for i, step := range e.PartialRecoverySteps {
			sb.WriteString(fmt.Sprintf("│   %d. %s\n", i+1, step))
		}
		sb.WriteString("│\n")
	}

	// Required actions
	sb.WriteString("▌ REQUIRED ACTIONS\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────────\n")
	for i, action := range e.UserActions {
		priority := strings.ToUpper(action.Priority)
		sb.WriteString(fmt.Sprintf("│ %d. [%s] %s\n", i+1, priority, action.Description))
		if action.Command != "" {
			sb.WriteString(fmt.Sprintf("│    └─ Run: %s\n", action.Command))
		}
		if action.Link != "" {
			sb.WriteString(fmt.Sprintf("│    └─ Visit: %s\n", action.Link))
		}
	}
	sb.WriteString("│\n")

	// Prevention
	if len(e.PreventionAdvice) > 0 {
		sb.WriteString("▌ PREVENTION FOR FUTURE\n")
		sb.WriteString("├─────────────────────────────────────────────────────────────────────\n")
		for _, advice := range e.PreventionAdvice {
			sb.WriteString(fmt.Sprintf("│ • %s\n", advice))
		}
		sb.WriteString("│\n")
	}

	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")

	return sb.String()
}

// FormatCompact returns a single-line summary
func (e *UnrecoverableError) FormatCompact() string {
	actionCount := len(e.UserActions)
	partial := ""
	if e.PartialRecoveryPossible {
		partial = " (partial recovery possible)"
	}
	return fmt.Sprintf("[UNRECOVERABLE] %s: %s - %d action(s) required%s",
		e.Reason, e.Message, actionCount, partial)
}
