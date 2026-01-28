package shared

import (
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// ERROR REPORT - Unified error reporting with root cause, context, and recovery
// =============================================================================

// ErrorReport provides comprehensive error information for debugging and recovery
type ErrorReport struct {
	// Unique identifier for this error instance
	Id string `json:"id"`

	// Timestamp when error occurred
	Timestamp time.Time `json:"timestamp"`

	// ==========================================================================
	// ROOT CAUSE - What actually went wrong
	// ==========================================================================

	// RootCause contains the fundamental error information
	RootCause *RootCause `json:"rootCause"`

	// ==========================================================================
	// STEP CONTEXT - Where in the execution flow the error occurred
	// ==========================================================================

	// StepContext provides execution context
	StepContext *StepContext `json:"stepContext"`

	// ==========================================================================
	// RECOVERY ACTION - What can be done to fix or recover
	// ==========================================================================

	// Recovery contains recommended recovery actions
	Recovery *RecoveryAction `json:"recovery"`

	// ==========================================================================
	// ADDITIONAL METADATA
	// ==========================================================================

	// RelatedErrors contains any cascading or related errors
	RelatedErrors []*ErrorReport `json:"relatedErrors,omitempty"`

	// Tags for categorization and filtering
	Tags []string `json:"tags,omitempty"`
}

// RootCause identifies what actually went wrong
type RootCause struct {
	// Category of the error (provider, file_system, validation, internal)
	Category ErrorCategory `json:"category"`

	// Type is the specific error type within the category
	Type string `json:"type"`

	// Code is a machine-readable error code
	Code string `json:"code"`

	// Message is the human-readable error description
	Message string `json:"message"`

	// OriginalError is the raw error from the source (provider, OS, etc.)
	OriginalError string `json:"originalError,omitempty"`

	// HTTPCode if this was an HTTP error
	HTTPCode int `json:"httpCode,omitempty"`

	// Provider if this was a provider error
	Provider string `json:"provider,omitempty"`

	// ProviderFailure contains detailed provider failure info
	ProviderFailure *ProviderFailure `json:"providerFailure,omitempty"`

	// StackTrace for internal errors
	StackTrace string `json:"stackTrace,omitempty"`
}

// ErrorCategory classifies the error source
type ErrorCategory string

const (
	ErrorCategoryProvider   ErrorCategory = "provider"    // AI provider error
	ErrorCategoryFileSystem ErrorCategory = "file_system" // File operation error
	ErrorCategoryValidation ErrorCategory = "validation"  // Input/state validation error
	ErrorCategoryNetwork    ErrorCategory = "network"     // Network connectivity error
	ErrorCategoryInternal   ErrorCategory = "internal"    // Internal system error
	ErrorCategoryUser       ErrorCategory = "user"        // User input error
	ErrorCategoryResource   ErrorCategory = "resource"    // Resource limit error
)

// StepContext provides execution context for the error
type StepContext struct {
	// PlanId is the plan being executed
	PlanId string `json:"planId"`

	// Branch is the active branch
	Branch string `json:"branch"`

	// JournalEntryId is the journal entry where the error occurred
	JournalEntryId string `json:"journalEntryId,omitempty"`

	// JournalEntrySeq is the sequence number in the journal
	JournalEntrySeq int `json:"journalEntrySeq"`

	// EntryType is the type of journal entry (user_prompt, model_request, etc.)
	EntryType EntryType `json:"entryType"`

	// Phase indicates where in the step execution the error occurred
	Phase ExecutionPhase `json:"phase"`

	// Operation is the specific operation being performed
	Operation string `json:"operation"`

	// TransactionId if within a file transaction
	TransactionId string `json:"transactionId,omitempty"`

	// TransactionState at time of error
	TransactionState TransactionState `json:"transactionState,omitempty"`

	// OperationIndex within the transaction (if applicable)
	OperationIndex int `json:"operationIndex,omitempty"`

	// FilePath if error relates to a specific file
	FilePath string `json:"filePath,omitempty"`

	// ModelRequest context if error occurred during model interaction
	ModelContext *ModelContext `json:"modelContext,omitempty"`

	// PreviousSteps summarizes what succeeded before this error
	PreviousSteps []StepSummary `json:"previousSteps,omitempty"`

	// CheckpointName of the last good checkpoint before error
	LastCheckpoint string `json:"lastCheckpoint,omitempty"`
}

// ExecutionPhase indicates where in step execution the error occurred
type ExecutionPhase string

const (
	PhaseInitialization ExecutionPhase = "initialization" // Setting up the step
	PhaseValidation     ExecutionPhase = "validation"     // Validating inputs
	PhaseExecution      ExecutionPhase = "execution"      // Executing the step
	PhaseStreaming      ExecutionPhase = "streaming"      // Receiving streaming response
	PhaseProcessing     ExecutionPhase = "processing"     // Processing response
	PhaseCommit         ExecutionPhase = "commit"         // Committing changes
	PhaseCleanup        ExecutionPhase = "cleanup"        // Cleaning up resources
)

// ModelContext provides context for model-related errors
type ModelContext struct {
	// Model being used
	Model string `json:"model"`

	// Provider handling the request
	Provider string `json:"provider"`

	// TokensIn is the input token count
	TokensIn int `json:"tokensIn,omitempty"`

	// TokensOut is the output tokens received before error
	TokensOut int `json:"tokensOut,omitempty"`

	// StreamProgress is percentage of expected response received (0-100)
	StreamProgress int `json:"streamProgress,omitempty"`

	// PartialResponse if any response was received before error
	PartialResponse string `json:"partialResponse,omitempty"`

	// RequestDuration is how long the request ran before failing
	RequestDuration time.Duration `json:"requestDuration,omitempty"`

	// RetryAttempt is the current retry attempt number (1-based)
	RetryAttempt int `json:"retryAttempt,omitempty"`
}

// StepSummary provides a brief summary of a completed step
type StepSummary struct {
	Seq      int           `json:"seq"`
	Type     EntryType     `json:"type"`
	Status   string        `json:"status"`
	Duration time.Duration `json:"duration,omitempty"`
}

// RecoveryAction describes what can be done to recover from the error
type RecoveryAction struct {
	// CanAutoRecover indicates if the system can recover automatically
	CanAutoRecover bool `json:"canAutoRecover"`

	// AutoRecoveryAction is what the system will do automatically
	AutoRecoveryAction string `json:"autoRecoveryAction,omitempty"`

	// RetryStrategy if retrying is recommended
	RetryStrategy *RetryStrategy `json:"retryStrategy,omitempty"`

	// RollbackRequired indicates if rollback is needed before retry
	RollbackRequired bool `json:"rollbackRequired"`

	// RollbackTarget is what to rollback to (checkpoint name or "original")
	RollbackTarget string `json:"rollbackTarget,omitempty"`

	// ManualActions are actions the user must take
	ManualActions []ManualAction `json:"manualActions,omitempty"`

	// AlternativeApproaches are different ways to accomplish the goal
	AlternativeApproaches []string `json:"alternativeApproaches,omitempty"`

	// Documentation links for more information
	DocumentationLinks []string `json:"documentationLinks,omitempty"`
}

// ManualAction describes an action the user must take
type ManualAction struct {
	// Description of what to do
	Description string `json:"description"`

	// Priority (critical, high, medium, low)
	Priority string `json:"priority"`

	// Command if a CLI command can help
	Command string `json:"command,omitempty"`

	// Link to relevant documentation or UI
	Link string `json:"link,omitempty"`
}

// =============================================================================
// ERROR REPORT BUILDER
// =============================================================================

// NewErrorReport creates a new error report with the given root cause
func NewErrorReport(category ErrorCategory, errorType, code, message string) *ErrorReport {
	return &ErrorReport{
		Id:        generateErrorId(),
		Timestamp: time.Now(),
		RootCause: &RootCause{
			Category: category,
			Type:     errorType,
			Code:     code,
			Message:  message,
		},
		StepContext: &StepContext{},
		Recovery:    &RecoveryAction{},
	}
}

// ErrorReportFromProviderFailure creates an error report from a provider failure
func ErrorReportFromProviderFailure(failure *ProviderFailure, stepCtx *StepContext) *ErrorReport {
	report := NewErrorReport(
		ErrorCategoryProvider,
		string(failure.Type),
		string(failure.Type), // Use type as code
		failure.Message,
	)

	report.RootCause.HTTPCode = failure.HTTPCode
	report.RootCause.Provider = failure.Provider
	report.RootCause.ProviderFailure = failure
	report.RootCause.OriginalError = failure.Message

	if stepCtx != nil {
		report.StepContext = stepCtx
	}

	// Set recovery based on failure classification
	report.Recovery = buildRecoveryFromFailure(failure)

	return report
}

// FromFileError creates an error report from a file system error
func FromFileError(err error, operation, filePath string, stepCtx *StepContext) *ErrorReport {
	report := NewErrorReport(
		ErrorCategoryFileSystem,
		classifyFileError(err),
		"FILE_ERROR",
		err.Error(),
	)

	report.RootCause.OriginalError = err.Error()

	if stepCtx != nil {
		report.StepContext = stepCtx
		report.StepContext.FilePath = filePath
		report.StepContext.Operation = operation
	}

	report.Recovery = buildRecoveryFromFileError(err, filePath)

	return report
}

// FromValidationError creates an error report from a validation error
func FromValidationError(message string, stepCtx *StepContext) *ErrorReport {
	report := NewErrorReport(
		ErrorCategoryValidation,
		"validation_failed",
		"VALIDATION_ERROR",
		message,
	)

	if stepCtx != nil {
		report.StepContext = stepCtx
	}

	report.Recovery.CanAutoRecover = false
	report.Recovery.ManualActions = []ManualAction{
		{
			Description: "Review and fix the validation error",
			Priority:    "high",
		},
	}

	return report
}

// =============================================================================
// RECOVERY BUILDERS
// =============================================================================

func buildRecoveryFromFailure(failure *ProviderFailure) *RecoveryAction {
	recovery := &RecoveryAction{}
	strategy := GetRetryStrategy(failure.Type)

	if strategy.ShouldRetry {
		recovery.CanAutoRecover = true
		recovery.RetryStrategy = &strategy
		recovery.RollbackRequired = true
		recovery.RollbackTarget = "last_checkpoint"
		recovery.AutoRecoveryAction = fmt.Sprintf(
			"Will retry with %s backoff (max %d attempts). "+
				"Transaction rolled back sequentially using persisted snapshots.",
			formatBackoff(strategy),
			strategy.MaxAttempts,
		)
	} else {
		recovery.CanAutoRecover = false
		recovery.ManualActions = getManualActionsForFailure(failure)
		recovery.AlternativeApproaches = getAlternativesForFailure(failure)
	}

	return recovery
}

func getManualActionsForFailure(failure *ProviderFailure) []ManualAction {
	actions := []ManualAction{}

	switch failure.Type {
	case FailureTypeAuthInvalid:
		actions = append(actions, ManualAction{
			Description: "Verify your API key is correct and not expired",
			Priority:    "critical",
			Command:     "plandex api-keys",
		})
	case FailureTypePermissionDenied:
		actions = append(actions, ManualAction{
			Description: "Request access to the model or resource",
			Priority:    "critical",
			Link:        getProviderConsoleLink(failure.Provider),
		})
	case FailureTypeQuotaExhausted:
		actions = append(actions, ManualAction{
			Description: "Add credits or upgrade your plan",
			Priority:    "critical",
			Link:        getProviderBillingLink(failure.Provider),
		})
	case FailureTypeContextTooLong:
		actions = append(actions, ManualAction{
			Description: "Reduce context size by removing files or summarizing",
			Priority:    "high",
			Command:     "plandex context rm <file>",
		})
		actions = append(actions, ManualAction{
			Description: "Use a model with larger context window",
			Priority:    "medium",
			Command:     "plandex models available --min-context 128000",
		})
	case FailureTypeContentPolicy:
		actions = append(actions, ManualAction{
			Description: "Modify your prompt to avoid policy violations",
			Priority:    "high",
		})
	case FailureTypeModelNotFound:
		actions = append(actions, ManualAction{
			Description: "Use a valid model identifier",
			Priority:    "critical",
			Command:     "plandex models available",
		})
	}

	return actions
}

func getAlternativesForFailure(failure *ProviderFailure) []string {
	alternatives := []string{}

	switch failure.Type {
	case FailureTypeContextTooLong:
		alternatives = append(alternatives,
			"Switch to a model with larger context (e.g., claude-3-opus, gpt-4-turbo-128k)",
			"Use context summarization to reduce token count",
			"Split the task into smaller sub-tasks",
		)
	case FailureTypeQuotaExhausted:
		alternatives = append(alternatives,
			"Switch to a different provider",
			"Use a less expensive model",
			"Wait for quota reset (if daily limit)",
		)
	case FailureTypeRateLimit:
		alternatives = append(alternatives,
			"Use provider fallback to route to alternative provider",
			"Reduce request frequency",
		)
	}

	return alternatives
}

func buildRecoveryFromFileError(err error, filePath string) *RecoveryAction {
	recovery := &RecoveryAction{}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "permission denied") {
		recovery.CanAutoRecover = false
		recovery.ManualActions = []ManualAction{
			{
				Description: fmt.Sprintf("Fix file permissions for: %s", filePath),
				Priority:    "critical",
				Command:     fmt.Sprintf("chmod 644 %s", filePath),
			},
		}
	} else if strings.Contains(errStr, "no such file") || strings.Contains(errStr, "does not exist") {
		recovery.CanAutoRecover = true
		recovery.AutoRecoveryAction = "File will be created if this is a create operation"
		recovery.RollbackRequired = false
	} else if strings.Contains(errStr, "disk full") || strings.Contains(errStr, "no space") {
		recovery.CanAutoRecover = false
		recovery.ManualActions = []ManualAction{
			{
				Description: "Free up disk space",
				Priority:    "critical",
			},
		}
	} else {
		recovery.CanAutoRecover = false
		recovery.ManualActions = []ManualAction{
			{
				Description: "Investigate and resolve the file system error",
				Priority:    "high",
			},
		}
	}

	return recovery
}

// =============================================================================
// FORMATTING
// =============================================================================

// Format returns a human-readable error report
func (r *ErrorReport) Format() string {
	var sb strings.Builder

	sb.WriteString("═══════════════════════════════════════════════════════════════════\n")
	sb.WriteString("                         ERROR REPORT\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════\n\n")

	// Root Cause
	sb.WriteString("▌ ROOT CAUSE\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("│ Category: %s\n", r.RootCause.Category))
	sb.WriteString(fmt.Sprintf("│ Type:     %s\n", r.RootCause.Type))
	if r.RootCause.Code != "" {
		sb.WriteString(fmt.Sprintf("│ Code:     %s\n", r.RootCause.Code))
	}
	if r.RootCause.HTTPCode != 0 {
		sb.WriteString(fmt.Sprintf("│ HTTP:     %d\n", r.RootCause.HTTPCode))
	}
	if r.RootCause.Provider != "" {
		sb.WriteString(fmt.Sprintf("│ Provider: %s\n", r.RootCause.Provider))
	}
	sb.WriteString("│\n")
	sb.WriteString(fmt.Sprintf("│ Message:\n│   %s\n", r.RootCause.Message))
	sb.WriteString("│\n")

	// Step Context
	if r.StepContext != nil && r.StepContext.JournalEntrySeq > 0 {
		sb.WriteString("▌ STEP CONTEXT\n")
		sb.WriteString("├─────────────────────────────────────────────────────────────────\n")
		sb.WriteString(fmt.Sprintf("│ Plan:       %s\n", r.StepContext.PlanId))
		sb.WriteString(fmt.Sprintf("│ Entry:      #%d (%s)\n", r.StepContext.JournalEntrySeq, r.StepContext.EntryType))
		sb.WriteString(fmt.Sprintf("│ Phase:      %s\n", r.StepContext.Phase))
		if r.StepContext.Operation != "" {
			sb.WriteString(fmt.Sprintf("│ Operation:  %s\n", r.StepContext.Operation))
		}
		if r.StepContext.FilePath != "" {
			sb.WriteString(fmt.Sprintf("│ File:       %s\n", r.StepContext.FilePath))
		}
		if r.StepContext.TransactionId != "" {
			sb.WriteString(fmt.Sprintf("│ Transaction: %s (state: %s)\n",
				r.StepContext.TransactionId, r.StepContext.TransactionState))
		}
		if r.StepContext.LastCheckpoint != "" {
			sb.WriteString(fmt.Sprintf("│ Last Checkpoint: %s\n", r.StepContext.LastCheckpoint))
		}

		// Model context
		if r.StepContext.ModelContext != nil {
			mc := r.StepContext.ModelContext
			sb.WriteString("│\n│ Model Context:\n")
			sb.WriteString(fmt.Sprintf("│   Model:    %s (%s)\n", mc.Model, mc.Provider))
			if mc.TokensIn > 0 {
				sb.WriteString(fmt.Sprintf("│   Tokens:   %d in", mc.TokensIn))
				if mc.TokensOut > 0 {
					sb.WriteString(fmt.Sprintf(", %d out", mc.TokensOut))
				}
				sb.WriteString("\n")
			}
			if mc.StreamProgress > 0 {
				sb.WriteString(fmt.Sprintf("│   Progress: %d%% of response received\n", mc.StreamProgress))
			}
			if mc.RetryAttempt > 1 {
				sb.WriteString(fmt.Sprintf("│   Attempt:  %d\n", mc.RetryAttempt))
			}
		}
		sb.WriteString("│\n")
	}

	// Recovery Action
	sb.WriteString("▌ RECOVERY\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────\n")
	if r.Recovery.CanAutoRecover {
		sb.WriteString("│ ✓ Automatic recovery available\n")
		sb.WriteString(fmt.Sprintf("│   %s\n", r.Recovery.AutoRecoveryAction))
		if r.Recovery.RollbackRequired {
			sb.WriteString(fmt.Sprintf("│   Rollback to: %s\n", r.Recovery.RollbackTarget))
			sb.WriteString("│   Method: sequential restore → remove → sweep using\n")
			sb.WriteString("│   persisted snapshots captured before any write began.\n")
		}
	} else {
		sb.WriteString("│ ✗ Manual intervention required\n")
	}

	if len(r.Recovery.ManualActions) > 0 {
		sb.WriteString("│\n│ Required Actions:\n")
		for i, action := range r.Recovery.ManualActions {
			sb.WriteString(fmt.Sprintf("│   %d. [%s] %s\n", i+1, action.Priority, action.Description))
			if action.Command != "" {
				sb.WriteString(fmt.Sprintf("│      Command: %s\n", action.Command))
			}
			if action.Link != "" {
				sb.WriteString(fmt.Sprintf("│      Link: %s\n", action.Link))
			}
		}
	}

	if len(r.Recovery.AlternativeApproaches) > 0 {
		sb.WriteString("│\n│ Alternative Approaches:\n")
		for _, alt := range r.Recovery.AlternativeApproaches {
			sb.WriteString(fmt.Sprintf("│   • %s\n", alt))
		}
	}

	sb.WriteString("│\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════\n")

	return sb.String()
}

// FormatCompact returns a single-line summary
func (r *ErrorReport) FormatCompact() string {
	recovery := "manual action required"
	if r.Recovery.CanAutoRecover {
		recovery = "will auto-retry"
	}

	return fmt.Sprintf("[%s] %s: %s (%s)",
		r.RootCause.Category,
		r.RootCause.Type,
		r.RootCause.Message,
		recovery,
	)
}

// =============================================================================
// HELPERS
// =============================================================================

func generateErrorId() string {
	return fmt.Sprintf("err_%d", time.Now().UnixNano())
}

func classifyFileError(err error) string {
	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "permission denied"):
		return "permission_denied"
	case strings.Contains(errStr, "no such file"), strings.Contains(errStr, "does not exist"):
		return "file_not_found"
	case strings.Contains(errStr, "is a directory"):
		return "is_directory"
	case strings.Contains(errStr, "disk full"), strings.Contains(errStr, "no space"):
		return "disk_full"
	case strings.Contains(errStr, "too many open files"):
		return "too_many_files"
	default:
		return "unknown"
	}
}

func formatBackoff(strategy RetryStrategy) string {
	if strategy.BackoffMultiplier > 1 {
		return "exponential"
	}
	return "linear"
}

func getProviderConsoleLink(provider string) string {
	links := map[string]string{
		"openai":    "https://platform.openai.com/account",
		"anthropic": "https://console.anthropic.com/settings",
		"google":    "https://console.cloud.google.com/apis",
		"azure":     "https://portal.azure.com",
	}
	if link, ok := links[strings.ToLower(provider)]; ok {
		return link
	}
	return ""
}

func getProviderBillingLink(provider string) string {
	links := map[string]string{
		"openai":     "https://platform.openai.com/account/billing",
		"anthropic":  "https://console.anthropic.com/settings/billing",
		"google":     "https://console.cloud.google.com/billing",
		"azure":      "https://portal.azure.com/#blade/Microsoft_Azure_Billing",
		"openrouter": "https://openrouter.ai/account",
	}
	if link, ok := links[strings.ToLower(provider)]; ok {
		return link
	}
	return ""
}
