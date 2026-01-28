package shared

import (
	"strings"
	"testing"
	"time"
)

// =============================================================================
// ValidationResult basic operations
// =============================================================================

func TestNewValidationResult(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	if r.Phase != PhaseSynchronous {
		t.Errorf("Phase = %s, want %s", r.Phase, PhaseSynchronous)
	}
	if !r.Passed {
		t.Error("new result should start as Passed")
	}
	if r.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
	if len(r.Errors) != 0 {
		t.Error("Errors should be empty")
	}
}

func TestValidationResult_AddFatal(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	r.Add(&ValidationError{
		Category: CategoryProvider,
		Severity: ValidationSeverityFatal,
		Phase:    PhaseSynchronous,
		Code:     "MISSING_KEY",
		Message:  "Missing OPENAI_API_KEY",
		Fix:      "export OPENAI_API_KEY=...",
		EnvVar:   "OPENAI_API_KEY",
	})

	if r.Passed {
		t.Error("Passed should be false after adding a fatal error")
	}
	if len(r.FatalErrors) != 1 {
		t.Errorf("FatalErrors = %d, want 1", len(r.FatalErrors))
	}
	if len(r.Warnings) != 0 {
		t.Errorf("Warnings = %d, want 0", len(r.Warnings))
	}
	if len(r.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(r.Errors))
	}
}

func TestValidationResult_AddWarning(t *testing.T) {
	r := NewValidationResult(PhaseDeferred)
	r.Add(&ValidationError{
		Category: CategoryProvider,
		Severity: ValidationSeverityWarn,
		Phase:    PhaseDeferred,
		Code:     "DUAL_PROVIDERS",
		Message:  "Both providers configured",
		Fix:      "Pick one",
	})

	if !r.Passed {
		t.Error("Passed should remain true when only warnings exist")
	}
	if len(r.Warnings) != 1 {
		t.Errorf("Warnings = %d, want 1", len(r.Warnings))
	}
	if len(r.FatalErrors) != 0 {
		t.Errorf("FatalErrors = %d, want 0", len(r.FatalErrors))
	}
}

func TestValidationResult_AddNil(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	r.Add(nil)
	if len(r.Errors) != 0 {
		t.Error("nil error should not be added")
	}
}

func TestValidationResult_Merge(t *testing.T) {
	a := NewValidationResult(PhaseSynchronous)
	b := NewValidationResult(PhaseSynchronous)

	a.Add(&ValidationError{Severity: ValidationSeverityFatal, Code: "A1", Message: "a"})
	b.Add(&ValidationError{Severity: ValidationSeverityWarn, Code: "B1", Message: "b"})
	b.Add(&ValidationError{Severity: ValidationSeverityFatal, Code: "B2", Message: "c"})

	a.Merge(b)

	if len(a.Errors) != 3 {
		t.Errorf("Errors after merge = %d, want 3", len(a.Errors))
	}
	if len(a.FatalErrors) != 2 {
		t.Errorf("FatalErrors after merge = %d, want 2", len(a.FatalErrors))
	}
	if len(a.Warnings) != 1 {
		t.Errorf("Warnings after merge = %d, want 1", len(a.Warnings))
	}
}

func TestValidationResult_MergeNil(t *testing.T) {
	a := NewValidationResult(PhaseSynchronous)
	a.Merge(nil) // should not panic
	if len(a.Errors) != 0 {
		t.Error("merging nil should be a no-op")
	}
}

// =============================================================================
// ValidationError implements error interface
// =============================================================================

func TestValidationError_Error(t *testing.T) {
	ve := &ValidationError{
		Code:     "MISSING_API_KEY",
		Severity: ValidationSeverityFatal,
		Message:  "OPENAI_API_KEY not set",
	}
	got := ve.Error()
	if !strings.Contains(got, "MISSING_API_KEY") {
		t.Errorf("Error() = %q, should contain code", got)
	}
	if !strings.Contains(got, "fatal") {
		t.Errorf("Error() = %q, should contain severity", got)
	}
}

// =============================================================================
// FormatCLI output
// =============================================================================

func TestValidationResult_FormatCLI_Empty(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	if r.FormatCLI() != "" {
		t.Error("FormatCLI on empty result should return empty string")
	}
}

func TestValidationResult_FormatCLI_FatalHeader(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	r.Add(&ValidationError{
		Category: CategoryProvider,
		Severity: ValidationSeverityFatal,
		Code:     "MISSING_KEY",
		Message:  "Missing API key for openai",
		Fix:      "export OPENAI_API_KEY=sk-...",
	})

	out := r.FormatCLI()
	if !strings.Contains(out, "must be fixed") {
		t.Errorf("FormatCLI should contain 'must be fixed' header for fatal errors, got:\n%s", out)
	}
	if !strings.Contains(out, "Missing API key for openai") {
		t.Errorf("FormatCLI should contain error message, got:\n%s", out)
	}
	if !strings.Contains(out, "export OPENAI_API_KEY") {
		t.Errorf("FormatCLI should contain fix instruction, got:\n%s", out)
	}
}

func TestValidationResult_FormatCLI_WarningHeader(t *testing.T) {
	r := NewValidationResult(PhaseDeferred)
	r.Add(&ValidationError{
		Category: CategoryProvider,
		Severity: ValidationSeverityWarn,
		Code:     "DUAL_ANTHROPIC",
		Message:  "Both Claude Max and ANTHROPIC_API_KEY are set",
		Fix:      "Pick one provider",
	})

	out := r.FormatCLI()
	if !strings.Contains(out, "warnings") {
		t.Errorf("FormatCLI should show 'warnings' header when only warnings present, got:\n%s", out)
	}
}

func TestValidationResult_FormatCLI_GroupsByCategory(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	r.Add(&ValidationError{
		Category: CategoryProvider,
		Severity: ValidationSeverityFatal,
		Code:     "P1",
		Message:  "Provider problem",
	})
	r.Add(&ValidationError{
		Category: CategoryFilesystem,
		Severity: ValidationSeverityFatal,
		Code:     "F1",
		Message:  "File problem",
	})
	r.Add(&ValidationError{
		Category: CategoryEnvironment,
		Severity: ValidationSeverityWarn,
		Code:     "E1",
		Message:  "Env problem",
	})

	out := r.FormatCLI()
	// Filesystem should appear before Provider in output (defined order).
	fsIdx := strings.Index(out, "FILESYSTEM")
	provIdx := strings.Index(out, "PROVIDER")
	if fsIdx == -1 || provIdx == -1 {
		t.Fatalf("FormatCLI should contain category headers, got:\n%s", out)
	}
	if fsIdx > provIdx {
		t.Errorf("FILESYSTEM should appear before PROVIDER in output")
	}
}

// =============================================================================
// ToErrorReport conversion
// =============================================================================

func TestValidationResult_ToErrorReport_Passed(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	if r.ToErrorReport() != nil {
		t.Error("ToErrorReport on passed result should return nil")
	}
}

func TestValidationResult_ToErrorReport_WithFatals(t *testing.T) {
	r := NewValidationResult(PhaseSynchronous)
	r.Add(&ValidationError{
		Category: CategoryProvider,
		Severity: ValidationSeverityFatal,
		Code:     "MISSING_KEY",
		Message:  "Missing OPENAI_API_KEY",
		Fix:      "export OPENAI_API_KEY=...",
	})
	r.Add(&ValidationError{
		Category: CategoryFilesystem,
		Severity: ValidationSeverityFatal,
		Code:     "BAD_PATH",
		Message:  "Credentials file not found",
		Fix:      "Create the credentials file",
	})

	report := r.ToErrorReport()
	if report == nil {
		t.Fatal("ToErrorReport should return non-nil for failed result")
	}
	if report.RootCause.Category != ErrorCategoryValidation {
		t.Errorf("Category = %s, want %s", report.RootCause.Category, ErrorCategoryValidation)
	}
	if report.RootCause.Code != "STARTUP_VALIDATION" {
		t.Errorf("Code = %s, want STARTUP_VALIDATION", report.RootCause.Code)
	}
	if !strings.Contains(report.RootCause.Message, "MISSING_KEY") {
		t.Errorf("Message should reference error codes, got: %s", report.RootCause.Message)
	}
	if len(report.Recovery.ManualActions) != 2 {
		t.Errorf("ManualActions = %d, want 2", len(report.Recovery.ManualActions))
	}
	if report.Recovery.CanAutoRecover {
		t.Error("CanAutoRecover should be false for validation errors")
	}
}

// =============================================================================
// ValidateEnvVarSet helper
// =============================================================================

func TestValidateEnvVarSet_Present(t *testing.T) {
	if ve := ValidateEnvVarSet("MY_KEY", "sk-abc123", "openai"); ve != nil {
		t.Errorf("should return nil for non-empty value, got: %v", ve)
	}
}

func TestValidateEnvVarSet_WhitespaceOnly(t *testing.T) {
	ve := ValidateEnvVarSet("MY_KEY", "   ", "openai")
	if ve == nil {
		t.Error("should return error for whitespace-only value")
	}
	if ve.Code != "MISSING_ENV_VAR" {
		t.Errorf("Code = %s, want MISSING_ENV_VAR", ve.Code)
	}
	if ve.EnvVar != "MY_KEY" {
		t.Errorf("EnvVar = %s, want MY_KEY", ve.EnvVar)
	}
	if ve.Provider != "openai" {
		t.Errorf("Provider = %s, want openai", ve.Provider)
	}
}

func TestValidateEnvVarSet_Empty(t *testing.T) {
	ve := ValidateEnvVarSet("OPENAI_API_KEY", "", "openai")
	if ve == nil {
		t.Error("should return error for empty value")
	}
	if ve.Severity != ValidationSeverityFatal {
		t.Errorf("Severity = %s, want %s", ve.Severity, ValidationSeverityFatal)
	}
	if !strings.Contains(ve.Fix, "export OPENAI_API_KEY") {
		t.Errorf("Fix should contain export command, got: %s", ve.Fix)
	}
}

// =============================================================================
// ValidateProviderCompatibility
// =============================================================================

func TestValidateProviderCompatibility_NoConflict(t *testing.T) {
	errs := ValidateProviderCompatibility([]ModelProvider{
		ModelProviderOpenAI,
		ModelProviderOpenRouter,
	})
	if len(errs) != 0 {
		t.Errorf("should have no warnings for compatible providers, got %d", len(errs))
	}
}

func TestValidateProviderCompatibility_DualAnthropic(t *testing.T) {
	errs := ValidateProviderCompatibility([]ModelProvider{
		ModelProviderAnthropic,
		ModelProviderAnthropicClaudeMax,
		ModelProviderOpenAI,
	})
	if len(errs) != 1 {
		t.Fatalf("should warn about dual Anthropic providers, got %d errors", len(errs))
	}
	if errs[0].Code != "DUAL_ANTHROPIC_PROVIDERS" {
		t.Errorf("Code = %s, want DUAL_ANTHROPIC_PROVIDERS", errs[0].Code)
	}
	if errs[0].Severity != ValidationSeverityWarn {
		t.Errorf("Severity = %s, want warning (dual anthropic is not fatal)", errs[0].Severity)
	}
}

func TestValidateProviderCompatibility_OnlyClaudeMax(t *testing.T) {
	errs := ValidateProviderCompatibility([]ModelProvider{
		ModelProviderAnthropicClaudeMax,
	})
	if len(errs) != 0 {
		t.Errorf("single Claude Max provider should not warn, got %d", len(errs))
	}
}

func TestValidateProviderCompatibility_Empty(t *testing.T) {
	errs := ValidateProviderCompatibility(nil)
	if len(errs) != 0 {
		t.Errorf("nil providers should not error, got %d", len(errs))
	}
}

// =============================================================================
// ValidateFilePath helpers
// =============================================================================

func TestValidateFilePath_Empty(t *testing.T) {
	ve := ValidateFilePath("", "trace file")
	if ve == nil {
		t.Error("should return error for empty path")
	}
	if ve.Code != "EMPTY_PATH" {
		t.Errorf("Code = %s, want EMPTY_PATH", ve.Code)
	}
}

func TestValidateFilePath_NonEmpty(t *testing.T) {
	if ve := ValidateFilePath("/some/path", "trace file"); ve != nil {
		t.Errorf("should return nil for non-empty path, got: %v", ve)
	}
}

// =============================================================================
// Timestamp freshness
// =============================================================================

func TestValidationResult_TimestampIsRecent(t *testing.T) {
	before := time.Now()
	r := NewValidationResult(PhaseSynchronous)
	after := time.Now()

	if r.Timestamp.Before(before) || r.Timestamp.After(after) {
		t.Error("Timestamp should be between before and after creation")
	}
}
