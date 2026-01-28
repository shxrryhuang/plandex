package shared

import (
	"errors"
	"strings"
	"testing"
)

func TestNewErrorReport(t *testing.T) {
	report := NewErrorReport(
		ErrorCategoryProvider,
		"rate_limit",
		"RATE_LIMIT_EXCEEDED",
		"Too many requests",
	)

	if report.Id == "" {
		t.Error("Error ID should not be empty")
	}
	if report.RootCause.Category != ErrorCategoryProvider {
		t.Errorf("Category = %s, want provider", report.RootCause.Category)
	}
	if report.RootCause.Type != "rate_limit" {
		t.Errorf("Type = %s, want rate_limit", report.RootCause.Type)
	}
	if report.RootCause.Code != "RATE_LIMIT_EXCEEDED" {
		t.Errorf("Code = %s, want RATE_LIMIT_EXCEEDED", report.RootCause.Code)
	}
	if report.RootCause.Message != "Too many requests" {
		t.Errorf("Message = %s, want 'Too many requests'", report.RootCause.Message)
	}
}

func TestFromProviderFailure_RateLimit(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeRateLimit,
		Category: FailureCategoryRetryable,
		HTTPCode: 429,
		Message:  "Rate limit exceeded for gpt-4",
		Provider: "openai",
	}

	stepCtx := &StepContext{
		PlanId:          "plan-123",
		JournalEntrySeq: 5,
		EntryType:       EntryTypeModelRequest,
		Phase:           PhaseStreaming,
	}

	report := ErrorReportFromProviderFailure(failure, stepCtx)

	// Check root cause
	if report.RootCause.Category != ErrorCategoryProvider {
		t.Errorf("Category = %s, want provider", report.RootCause.Category)
	}
	if report.RootCause.HTTPCode != 429 {
		t.Errorf("HTTPCode = %d, want 429", report.RootCause.HTTPCode)
	}
	if report.RootCause.Provider != "openai" {
		t.Errorf("Provider = %s, want openai", report.RootCause.Provider)
	}

	// Check step context
	if report.StepContext.PlanId != "plan-123" {
		t.Errorf("PlanId = %s, want plan-123", report.StepContext.PlanId)
	}
	if report.StepContext.Phase != PhaseStreaming {
		t.Errorf("Phase = %s, want streaming", report.StepContext.Phase)
	}

	// Check recovery
	if !report.Recovery.CanAutoRecover {
		t.Error("Rate limit should be auto-recoverable")
	}
	if report.Recovery.RetryStrategy == nil {
		t.Error("RetryStrategy should be set")
	}
	if !report.Recovery.RollbackRequired {
		t.Error("RollbackRequired should be true")
	}
}

func TestFromProviderFailure_QuotaExhausted(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeQuotaExhausted,
		Category: FailureCategoryNonRetryable,
		HTTPCode: 429,
		Message:  "You exceeded your current quota",
		Provider: "openai",
	}

	report := ErrorReportFromProviderFailure(failure, nil)

	// Check recovery - should NOT auto-recover
	if report.Recovery.CanAutoRecover {
		t.Error("Quota exhausted should NOT be auto-recoverable")
	}
	if len(report.Recovery.ManualActions) == 0 {
		t.Error("Should have manual actions")
	}

	// Should suggest adding credits
	found := false
	for _, action := range report.Recovery.ManualActions {
		if strings.Contains(strings.ToLower(action.Description), "credit") ||
			strings.Contains(strings.ToLower(action.Description), "upgrade") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should suggest adding credits or upgrading")
	}
}

func TestFromProviderFailure_ContextTooLong(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeContextTooLong,
		Category: FailureCategoryNonRetryable,
		HTTPCode: 400,
		Message:  "Context length exceeded: 150000 > 128000",
		Provider: "anthropic",
	}

	report := ErrorReportFromProviderFailure(failure, nil)

	// Should not auto-recover
	if report.Recovery.CanAutoRecover {
		t.Error("Context too long should NOT be auto-recoverable")
	}

	// Should have alternatives
	if len(report.Recovery.AlternativeApproaches) == 0 {
		t.Error("Should suggest alternative approaches")
	}

	// Should suggest larger context model
	foundLargerModel := false
	for _, alt := range report.Recovery.AlternativeApproaches {
		if strings.Contains(strings.ToLower(alt), "larger context") {
			foundLargerModel = true
			break
		}
	}
	if !foundLargerModel {
		t.Error("Should suggest using larger context model")
	}
}

func TestFromFileError_PermissionDenied(t *testing.T) {
	err := errors.New("open /etc/passwd: permission denied")

	stepCtx := &StepContext{
		PlanId:          "plan-123",
		JournalEntrySeq: 3,
		Phase:           PhaseExecution,
	}

	report := FromFileError(err, "read", "/etc/passwd", stepCtx)

	// Check root cause
	if report.RootCause.Category != ErrorCategoryFileSystem {
		t.Errorf("Category = %s, want file_system", report.RootCause.Category)
	}
	if report.RootCause.Type != "permission_denied" {
		t.Errorf("Type = %s, want permission_denied", report.RootCause.Type)
	}

	// Check context
	if report.StepContext.FilePath != "/etc/passwd" {
		t.Errorf("FilePath = %s, want /etc/passwd", report.StepContext.FilePath)
	}
	if report.StepContext.Operation != "read" {
		t.Errorf("Operation = %s, want read", report.StepContext.Operation)
	}

	// Check recovery
	if report.Recovery.CanAutoRecover {
		t.Error("Permission denied should NOT be auto-recoverable")
	}
	if len(report.Recovery.ManualActions) == 0 {
		t.Error("Should have manual actions")
	}

	// Should suggest chmod
	foundChmod := false
	for _, action := range report.Recovery.ManualActions {
		if strings.Contains(action.Command, "chmod") {
			foundChmod = true
			break
		}
	}
	if !foundChmod {
		t.Error("Should suggest chmod command")
	}
}

func TestFromFileError_FileNotFound(t *testing.T) {
	err := errors.New("open /tmp/missing.txt: no such file or directory")

	report := FromFileError(err, "read", "/tmp/missing.txt", nil)

	if report.RootCause.Type != "file_not_found" {
		t.Errorf("Type = %s, want file_not_found", report.RootCause.Type)
	}

	// File not found can be auto-recovered for create operations
	if !report.Recovery.CanAutoRecover {
		t.Error("File not found should be auto-recoverable (for create)")
	}
}

func TestFromFileError_DiskFull(t *testing.T) {
	err := errors.New("write /tmp/file.txt: no space left on device")

	report := FromFileError(err, "write", "/tmp/file.txt", nil)

	if report.RootCause.Type != "disk_full" {
		t.Errorf("Type = %s, want disk_full", report.RootCause.Type)
	}

	if report.Recovery.CanAutoRecover {
		t.Error("Disk full should NOT be auto-recoverable")
	}
}

func TestFromValidationError(t *testing.T) {
	stepCtx := &StepContext{
		PlanId:          "plan-123",
		JournalEntrySeq: 2,
		Phase:           PhaseValidation,
	}

	report := FromValidationError("Invalid checkpoint name", stepCtx)

	if report.RootCause.Category != ErrorCategoryValidation {
		t.Errorf("Category = %s, want validation", report.RootCause.Category)
	}
	if report.RootCause.Message != "Invalid checkpoint name" {
		t.Errorf("Message = %s, want 'Invalid checkpoint name'", report.RootCause.Message)
	}
	if report.Recovery.CanAutoRecover {
		t.Error("Validation error should NOT be auto-recoverable")
	}
}

func TestErrorReportFormat(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeRateLimit,
		Category: FailureCategoryRetryable,
		HTTPCode: 429,
		Message:  "Rate limit exceeded",
		Provider: "openai",
	}

	stepCtx := &StepContext{
		PlanId:          "plan-123",
		JournalEntrySeq: 5,
		EntryType:       EntryTypeModelRequest,
		Phase:           PhaseStreaming,
		LastCheckpoint:  "checkpoint_1",
		ModelContext: &ModelContext{
			Model:          "gpt-4",
			Provider:       "openai",
			TokensIn:       5000,
			TokensOut:      1500,
			StreamProgress: 45,
			RetryAttempt:   2,
		},
	}

	report := ErrorReportFromProviderFailure(failure, stepCtx)
	formatted := report.Format()

	// Check that key sections are present
	if !strings.Contains(formatted, "ROOT CAUSE") {
		t.Error("Should contain ROOT CAUSE section")
	}
	if !strings.Contains(formatted, "STEP CONTEXT") {
		t.Error("Should contain STEP CONTEXT section")
	}
	if !strings.Contains(formatted, "RECOVERY") {
		t.Error("Should contain RECOVERY section")
	}

	// Check that key information is present
	if !strings.Contains(formatted, "provider") {
		t.Error("Should contain category")
	}
	if !strings.Contains(formatted, "rate_limit") {
		t.Error("Should contain error type")
	}
	if !strings.Contains(formatted, "429") {
		t.Error("Should contain HTTP code")
	}
	if !strings.Contains(formatted, "plan-123") {
		t.Error("Should contain plan ID")
	}
	if !strings.Contains(formatted, "gpt-4") {
		t.Error("Should contain model name")
	}
	if !strings.Contains(formatted, "45%") {
		t.Error("Should contain stream progress")
	}
	if !strings.Contains(formatted, "checkpoint_1") {
		t.Error("Should contain last checkpoint")
	}
	if !strings.Contains(formatted, "Automatic recovery") {
		t.Error("Should indicate automatic recovery")
	}
}

func TestErrorReportFormatCompact(t *testing.T) {
	report := NewErrorReport(
		ErrorCategoryProvider,
		"rate_limit",
		"RATE_LIMIT",
		"Too many requests",
	)
	report.Recovery.CanAutoRecover = true

	compact := report.FormatCompact()

	if !strings.Contains(compact, "provider") {
		t.Error("Compact format should contain category")
	}
	if !strings.Contains(compact, "rate_limit") {
		t.Error("Compact format should contain type")
	}
	if !strings.Contains(compact, "Too many requests") {
		t.Error("Compact format should contain message")
	}
	if !strings.Contains(compact, "auto-retry") {
		t.Error("Compact format should indicate auto-retry")
	}
}

func TestErrorCategoryConstants(t *testing.T) {
	categories := []struct {
		cat      ErrorCategory
		expected string
	}{
		{ErrorCategoryProvider, "provider"},
		{ErrorCategoryFileSystem, "file_system"},
		{ErrorCategoryValidation, "validation"},
		{ErrorCategoryNetwork, "network"},
		{ErrorCategoryInternal, "internal"},
		{ErrorCategoryUser, "user"},
		{ErrorCategoryResource, "resource"},
	}

	for _, tc := range categories {
		if string(tc.cat) != tc.expected {
			t.Errorf("ErrorCategory %v = %s, want %s", tc.cat, string(tc.cat), tc.expected)
		}
	}
}

func TestExecutionPhaseConstants(t *testing.T) {
	phases := []struct {
		phase    ExecutionPhase
		expected string
	}{
		{PhaseInitialization, "initialization"},
		{PhaseValidation, "validation"},
		{PhaseExecution, "execution"},
		{PhaseStreaming, "streaming"},
		{PhaseProcessing, "processing"},
		{PhaseCommit, "commit"},
		{PhaseCleanup, "cleanup"},
	}

	for _, tc := range phases {
		if string(tc.phase) != tc.expected {
			t.Errorf("ExecutionPhase %v = %s, want %s", tc.phase, string(tc.phase), tc.expected)
		}
	}
}

func TestGetProviderLinks(t *testing.T) {
	// Test console links
	consoleLink := getProviderConsoleLink("openai")
	if !strings.Contains(consoleLink, "openai") {
		t.Error("OpenAI console link should contain 'openai'")
	}

	// Test billing links
	billingLink := getProviderBillingLink("anthropic")
	if !strings.Contains(billingLink, "anthropic") {
		t.Error("Anthropic billing link should contain 'anthropic'")
	}

	// Test unknown provider returns empty
	unknownLink := getProviderConsoleLink("unknown_provider")
	if unknownLink != "" {
		t.Error("Unknown provider should return empty link")
	}
}

func TestClassifyFileError(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{errors.New("permission denied"), "permission_denied"},
		{errors.New("no such file or directory"), "file_not_found"},
		{errors.New("file does not exist"), "file_not_found"},
		{errors.New("is a directory"), "is_directory"},
		{errors.New("no space left on device"), "disk_full"},
		{errors.New("disk full"), "disk_full"},
		{errors.New("too many open files"), "too_many_files"},
		{errors.New("some random error"), "unknown"},
	}

	for _, tc := range tests {
		result := classifyFileError(tc.err)
		if result != tc.expected {
			t.Errorf("classifyFileError(%q) = %s, want %s", tc.err, result, tc.expected)
		}
	}
}

func TestManualActionsForAuthError(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeAuthInvalid,
		Provider: "openai",
	}

	actions := getManualActionsForFailure(failure)

	if len(actions) == 0 {
		t.Fatal("Should have manual actions for auth error")
	}

	// Should mention API key
	foundApiKey := false
	for _, action := range actions {
		if strings.Contains(strings.ToLower(action.Description), "api key") {
			foundApiKey = true
			break
		}
	}
	if !foundApiKey {
		t.Error("Should mention API key in manual actions")
	}
}

func TestAlternativesForContextTooLong(t *testing.T) {
	failure := &ProviderFailure{
		Type: FailureTypeContextTooLong,
	}

	alternatives := getAlternativesForFailure(failure)

	if len(alternatives) == 0 {
		t.Fatal("Should have alternatives for context too long")
	}

	// Should suggest larger context model
	foundLargerContext := false
	foundSummarization := false
	for _, alt := range alternatives {
		lower := strings.ToLower(alt)
		if strings.Contains(lower, "larger context") {
			foundLargerContext = true
		}
		if strings.Contains(lower, "summariz") {
			foundSummarization = true
		}
	}

	if !foundLargerContext {
		t.Error("Should suggest larger context model")
	}
	if !foundSummarization {
		t.Error("Should suggest summarization")
	}
}
