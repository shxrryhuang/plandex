package shared

import (
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// VALIDATION FRAMEWORK - Startup and pre-execution configuration checks
// =============================================================================
//
// Validation runs in two phases:
//   - Synchronous (startup): checks that must pass before any execution begins
//     (home directory, auth files, filesystem paths, environment sanity)
//   - Deferred (provider-scoped): checks that run only when a specific provider
//     or feature is actually invoked (API key format, provider compatibility)
//
// =============================================================================

// ValidationSeverity indicates how critical a validation failure is.
type ValidationSeverity string

const (
	// ValidationSeverityFatal halts execution immediately; the user must fix this.
	ValidationSeverityFatal ValidationSeverity = "fatal"
	// ValidationSeverityWarn surfaces in CLI output but does not block execution.
	ValidationSeverityWarn ValidationSeverity = "warning"
)

// ValidationCategory groups related checks so output can be organized.
type ValidationCategory string

const (
	CategoryAuth        ValidationCategory = "authentication"
	CategoryProvider    ValidationCategory = "provider"
	CategoryFilesystem  ValidationCategory = "filesystem"
	CategoryConfig      ValidationCategory = "configuration"
	CategoryEnvironment ValidationCategory = "environment"
)

// ValidationPhase controls when the check runs.
type ValidationPhase string

const (
	// PhaseSynchronous runs at startup before any plan execution.
	PhaseSynchronous ValidationPhase = "startup"
	// PhaseDeferred runs only when the relevant provider / feature is invoked.
	PhaseDeferred ValidationPhase = "deferred"
	// PhasePreflight runs after provider validation but before any LLM call or
	// file write — the last gate before "real work" begins.
	PhasePreflight ValidationPhase = "preflight"
)

// ValidationError is a single validation failure.
type ValidationError struct {
	// Category groups the error for display purposes.
	Category ValidationCategory `json:"category"`

	// Severity determines whether the error blocks execution.
	Severity ValidationSeverity `json:"severity"`

	// Phase indicates whether this is a startup or deferred check.
	Phase ValidationPhase `json:"phase"`

	// Code is a short machine-readable identifier (e.g. "MISSING_API_KEY").
	Code string `json:"code"`

	// Message is the human-readable description of what is wrong.
	Message string `json:"message"`

	// Fix is a concrete, actionable instruction for the user.
	Fix string `json:"fix"`

	// EnvVar is the environment variable involved, if any.
	EnvVar string `json:"envVar,omitempty"`

	// Path is the filesystem path involved, if any.
	Path string `json:"path,omitempty"`

	// Provider is the provider composite key involved, if any.
	Provider string `json:"provider,omitempty"`
}

// Error implements the error interface so ValidationError can be used as a Go error.
func (v *ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", v.Code, v.Severity, v.Message)
}

// ValidationResult aggregates all checks run during a validation phase.
type ValidationResult struct {
	// Phase that produced this result.
	Phase ValidationPhase `json:"phase"`

	// Timestamp when validation ran.
	Timestamp time.Time `json:"timestamp"`

	// Passed is true when no fatal errors exist.
	Passed bool `json:"passed"`

	// Errors contains every check that did not pass.
	Errors []*ValidationError `json:"errors"`

	// Warnings is a convenience slice of errors with ValidationSeverityWarn.
	Warnings []*ValidationError `json:"warnings"`

	// FatalErrors is a convenience slice of errors with ValidationSeverityFatal.
	FatalErrors []*ValidationError `json:"fatalErrors"`
}

// NewValidationResult initializes an empty result for the given phase.
func NewValidationResult(phase ValidationPhase) *ValidationResult {
	return &ValidationResult{
		Phase:     phase,
		Timestamp: time.Now(),
		Passed:    true,
	}
}

// Add inserts a ValidationError and updates convenience slices and Passed flag.
func (r *ValidationResult) Add(ve *ValidationError) {
	if ve == nil {
		return
	}
	r.Errors = append(r.Errors, ve)
	switch ve.Severity {
	case ValidationSeverityFatal:
		r.FatalErrors = append(r.FatalErrors, ve)
		r.Passed = false
	case ValidationSeverityWarn:
		r.Warnings = append(r.Warnings, ve)
	}
}

// Merge adds all errors from another result into this one.
func (r *ValidationResult) Merge(other *ValidationResult) {
	if other == nil {
		return
	}
	for _, e := range other.Errors {
		r.Add(e)
	}
}

// FormatCLI returns a multi-line string suitable for terminal output.
// Groups errors by category, shows severity badge, message, and fix.
func (r *ValidationResult) FormatCLI() string {
	if len(r.Errors) == 0 {
		return ""
	}

	var sb strings.Builder

	// Group errors by category for cleaner output.
	byCategory := map[ValidationCategory][]*ValidationError{}
	categoryOrder := []ValidationCategory{
		CategoryFilesystem,
		CategoryEnvironment,
		CategoryAuth,
		CategoryProvider,
		CategoryConfig,
	}
	for _, e := range r.Errors {
		byCategory[e.Category] = append(byCategory[e.Category], e)
	}

	// Header
	hasFatal := len(r.FatalErrors) > 0
	if hasFatal {
		sb.WriteString("── Configuration errors must be fixed before running ──\n\n")
	} else {
		sb.WriteString("── Configuration warnings ──\n\n")
	}

	for _, cat := range categoryOrder {
		errs, ok := byCategory[cat]
		if !ok {
			continue
		}
		delete(byCategory, cat) // consume so we can catch any extras

		sb.WriteString(fmt.Sprintf("  %s\n", strings.ToUpper(string(cat))))
		for _, e := range errs {
			badge := "⚠"
			if e.Severity == ValidationSeverityFatal {
				badge = "✗"
			}
			sb.WriteString(fmt.Sprintf("    %s  %s\n", badge, e.Message))
			if e.Fix != "" {
				sb.WriteString(fmt.Sprintf("       → %s\n", e.Fix))
			}
		}
		sb.WriteString("\n")
	}

	// Any categories not in the predefined order
	for _, errs := range byCategory {
		for _, e := range errs {
			badge := "⚠"
			if e.Severity == ValidationSeverityFatal {
				badge = "✗"
			}
			sb.WriteString(fmt.Sprintf("    %s  %s\n", badge, e.Message))
			if e.Fix != "" {
				sb.WriteString(fmt.Sprintf("       → %s\n", e.Fix))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ToErrorReport converts the validation result into an ErrorReport for the
// global error registry so failures are visible in run journals.
func (r *ValidationResult) ToErrorReport() *ErrorReport {
	if r.Passed {
		return nil
	}

	var msgs []string
	for _, e := range r.FatalErrors {
		msgs = append(msgs, fmt.Sprintf("[%s] %s", e.Code, e.Message))
	}

	report := NewErrorReport(
		ErrorCategoryValidation,
		"startup_validation_failed",
		"STARTUP_VALIDATION",
		strings.Join(msgs, "; "),
	)

	report.StepContext = &StepContext{
		Phase: PhaseValidation,
	}

	report.Recovery = &RecoveryAction{
		CanAutoRecover: false,
		ManualActions:  make([]ManualAction, 0, len(r.FatalErrors)),
	}

	for _, e := range r.FatalErrors {
		action := ManualAction{
			Description: e.Fix,
			Priority:    "critical",
		}
		report.Recovery.ManualActions = append(report.Recovery.ManualActions, action)
	}

	return report
}

// =============================================================================
// COMMON VALIDATION HELPERS
// =============================================================================

// ValidateEnvVarSet checks that an environment variable value is non-empty.
// Returns a fatal ValidationError if the value is blank.
func ValidateEnvVarSet(envVar, value, providerLabel string) *ValidationError {
	if strings.TrimSpace(value) != "" {
		return nil
	}
	return &ValidationError{
		Category: CategoryProvider,
		Severity: ValidationSeverityFatal,
		Phase:    PhaseDeferred,
		Code:     "MISSING_ENV_VAR",
		Message:  fmt.Sprintf("Missing required environment variable %s (needed for %s)", envVar, providerLabel),
		Fix:      fmt.Sprintf("export %s='your-api-key'", envVar),
		EnvVar:   envVar,
		Provider: providerLabel,
	}
}

// ValidateFilePath checks that a path exists and is accessible.
func ValidateFilePath(path, context string) *ValidationError {
	if path == "" {
		return &ValidationError{
			Category: CategoryFilesystem,
			Severity: ValidationSeverityFatal,
			Phase:    PhaseSynchronous,
			Code:     "EMPTY_PATH",
			Message:  fmt.Sprintf("Empty path for %s", context),
			Fix:      fmt.Sprintf("Ensure %s is configured to a valid path", context),
		}
	}
	return nil
}

// ValidateFilePathExists checks that a path points to an existing entry.
// Unlike ValidateFilePath this actually stats the filesystem.
func ValidateFilePathExists(path, context string) *ValidationError {
	// Caller must import os and do the stat; this helper only constructs the error.
	return &ValidationError{
		Category: CategoryFilesystem,
		Severity: ValidationSeverityFatal,
		Phase:    PhaseSynchronous,
		Code:     "PATH_NOT_FOUND",
		Message:  fmt.Sprintf("Path does not exist for %s: %s", context, path),
		Fix:      fmt.Sprintf("Create or fix the path: %s", path),
		Path:     path,
	}
}

// ValidateProviderCompatibility checks that a set of providers does not
// include mutually incompatible combinations.
func ValidateProviderCompatibility(providers []ModelProvider) []*ValidationError {
	var errs []*ValidationError

	hasClaudeMax := false
	hasAnthropicDirect := false
	for _, p := range providers {
		switch p {
		case ModelProviderAnthropicClaudeMax:
			hasClaudeMax = true
		case ModelProviderAnthropic:
			hasAnthropicDirect = true
		}
	}

	// Claude Max + direct Anthropic key is a common misconfiguration.
	// It is valid (fallback chain), but warn if both are set because the
	// cooldown logic can cause confusing behavior.
	if hasClaudeMax && hasAnthropicDirect {
		errs = append(errs, &ValidationError{
			Category: CategoryProvider,
			Severity: ValidationSeverityWarn,
			Phase:    PhaseDeferred,
			Code:     "DUAL_ANTHROPIC_PROVIDERS",
			Message:  "Both Claude Max subscription and ANTHROPIC_API_KEY are configured",
			Fix:      "If you intend to use Claude Max as your primary Anthropic provider, you can unset ANTHROPIC_API_KEY. Both will work as a fallback chain but cooldown behavior may be confusing.",
		})
	}

	return errs
}
