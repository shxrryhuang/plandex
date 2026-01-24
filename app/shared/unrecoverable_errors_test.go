package shared

import (
	"strings"
	"testing"
)

func TestGetUnrecoverableEdgeCases(t *testing.T) {
	cases := GetUnrecoverableEdgeCases()

	if len(cases) == 0 {
		t.Fatal("Should have unrecoverable edge cases defined")
	}

	// Verify each case has required fields
	for _, ec := range cases {
		if ec.Reason == "" {
			t.Error("Edge case missing Reason")
		}
		if ec.Category == "" {
			t.Error("Edge case missing Category")
		}
		if ec.Title == "" {
			t.Errorf("Edge case %s missing Title", ec.Reason)
		}
		if ec.Description == "" {
			t.Errorf("Edge case %s missing Description", ec.Reason)
		}
		if ec.WhyUnrecoverable == "" {
			t.Errorf("Edge case %s missing WhyUnrecoverable explanation", ec.Reason)
		}
		if len(ec.UserActions) == 0 {
			t.Errorf("Edge case %s missing UserActions", ec.Reason)
		}
	}
}

func TestEdgeCaseCoverage(t *testing.T) {
	cases := GetUnrecoverableEdgeCases()

	// Check that we cover all major categories
	categories := make(map[UnrecoverableCategory]bool)
	for _, ec := range cases {
		categories[ec.Category] = true
	}

	expectedCategories := []UnrecoverableCategory{
		CategoryProviderLimit,
		CategoryAuthentication,
		CategoryDataLoss,
		CategoryExternalState,
		CategorySystemResource,
	}

	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("Missing edge cases for category: %s", cat)
		}
	}
}

func TestDetectUnrecoverableCondition_QuotaExhausted(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeQuotaExhausted,
		Category: FailureCategoryNonRetryable,
		HTTPCode: 429,
		Message:  "You exceeded your current quota",
		Provider: "openai",
	}

	report := FromProviderFailure(failure, nil)
	unrecoverable := DetectUnrecoverableCondition(report)

	if unrecoverable == nil {
		t.Fatal("Should detect quota exhausted as unrecoverable")
	}

	if unrecoverable.Reason != UnrecoverableQuotaExhausted {
		t.Errorf("Reason = %s, want quota_exhausted", unrecoverable.Reason)
	}

	if unrecoverable.Category != CategoryProviderLimit {
		t.Errorf("Category = %s, want provider_limit", unrecoverable.Category)
	}

	if len(unrecoverable.UserActions) == 0 {
		t.Error("Should have user actions")
	}
}

func TestDetectUnrecoverableCondition_AuthInvalid(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeAuthInvalid,
		Category: FailureCategoryNonRetryable,
		HTTPCode: 401,
		Message:  "Invalid API key",
		Provider: "anthropic",
	}

	report := FromProviderFailure(failure, nil)
	unrecoverable := DetectUnrecoverableCondition(report)

	if unrecoverable == nil {
		t.Fatal("Should detect auth invalid as unrecoverable")
	}

	if unrecoverable.Reason != UnrecoverableAuthInvalid {
		t.Errorf("Reason = %s, want auth_invalid", unrecoverable.Reason)
	}

	if unrecoverable.Category != CategoryAuthentication {
		t.Errorf("Category = %s, want authentication", unrecoverable.Category)
	}
}

func TestDetectUnrecoverableCondition_ContextTooLong(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeContextTooLong,
		Category: FailureCategoryNonRetryable,
		HTTPCode: 400,
		Message:  "Context length exceeded",
		Provider: "openai",
	}

	report := FromProviderFailure(failure, nil)
	unrecoverable := DetectUnrecoverableCondition(report)

	if unrecoverable == nil {
		t.Fatal("Should detect context too long as unrecoverable")
	}

	if unrecoverable.Reason != UnrecoverableContextTooLong {
		t.Errorf("Reason = %s, want context_too_long", unrecoverable.Reason)
	}

	// Should have partial recovery
	if !unrecoverable.PartialRecoveryPossible {
		t.Error("Context too long should have partial recovery option")
	}
}

func TestDetectUnrecoverableCondition_ContentPolicy(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeContentPolicy,
		Category: FailureCategoryNonRetryable,
		HTTPCode: 400,
		Message:  "Content policy violation",
		Provider: "openai",
	}

	report := FromProviderFailure(failure, nil)
	unrecoverable := DetectUnrecoverableCondition(report)

	if unrecoverable == nil {
		t.Fatal("Should detect content policy as unrecoverable")
	}

	if unrecoverable.Reason != UnrecoverableContentPolicy {
		t.Errorf("Reason = %s, want content_policy", unrecoverable.Reason)
	}
}

func TestDetectUnrecoverableCondition_RetryableIsRecoverable(t *testing.T) {
	failure := &ProviderFailure{
		Type:     FailureTypeRateLimit,
		Category: FailureCategoryRetryable,
		HTTPCode: 429,
		Message:  "Rate limit exceeded",
		Provider: "openai",
	}

	report := FromProviderFailure(failure, nil)
	unrecoverable := DetectUnrecoverableCondition(report)

	if unrecoverable != nil {
		t.Error("Rate limit (retryable) should NOT be detected as unrecoverable")
	}
}

func TestDetectUnrecoverableCondition_DiskFull(t *testing.T) {
	report := FromFileError(
		&mockError{msg: "no space left on device"},
		"write",
		"/tmp/file.txt",
		&StepContext{FilePath: "/tmp/file.txt"},
	)

	unrecoverable := DetectUnrecoverableCondition(report)

	if unrecoverable == nil {
		t.Fatal("Should detect disk full as unrecoverable")
	}

	if unrecoverable.Reason != UnrecoverableDiskFull {
		t.Errorf("Reason = %s, want disk_full", unrecoverable.Reason)
	}

	if unrecoverable.Category != CategorySystemResource {
		t.Errorf("Category = %s, want system_resource", unrecoverable.Category)
	}
}

func TestDetectUnrecoverableCondition_PermissionError(t *testing.T) {
	report := FromFileError(
		&mockError{msg: "permission denied"},
		"write",
		"/etc/passwd",
		&StepContext{FilePath: "/etc/passwd"},
	)

	unrecoverable := DetectUnrecoverableCondition(report)

	if unrecoverable == nil {
		t.Fatal("Should detect permission error as unrecoverable")
	}

	if unrecoverable.Reason != UnrecoverablePermissionError {
		t.Errorf("Reason = %s, want permission_error", unrecoverable.Reason)
	}
}

func TestUnrecoverableErrorFormat(t *testing.T) {
	err := &UnrecoverableError{
		Reason:                  UnrecoverableQuotaExhausted,
		Category:                CategoryProviderLimit,
		Message:                 "You exceeded your current quota",
		TechnicalDetails:        "HTTP 429 from openai",
		PartialRecoveryPossible: true,
		PartialRecoverySteps: []string{
			"Rollback transaction",
			"Save journal for later",
		},
		UserActions: []UserAction{
			{Description: "Add credits", Priority: "critical"},
			{Description: "Switch provider", Priority: "high", Command: "plandex providers switch anthropic"},
		},
		PreventionAdvice: []string{
			"Monitor usage",
			"Set billing alerts",
		},
	}

	formatted := err.Format()

	// Check sections present
	if !strings.Contains(formatted, "UNRECOVERABLE ERROR") {
		t.Error("Should have header")
	}
	if !strings.Contains(formatted, "WHAT HAPPENED") {
		t.Error("Should have WHAT HAPPENED section")
	}
	if !strings.Contains(formatted, "WHY AUTOMATIC RECOVERY IS NOT POSSIBLE") {
		t.Error("Should have explanation section")
	}
	if !strings.Contains(formatted, "PARTIAL RECOVERY AVAILABLE") {
		t.Error("Should have partial recovery section")
	}
	if !strings.Contains(formatted, "REQUIRED ACTIONS") {
		t.Error("Should have required actions section")
	}
	if !strings.Contains(formatted, "PREVENTION") {
		t.Error("Should have prevention section")
	}

	// Check content
	if !strings.Contains(formatted, "quota") {
		t.Error("Should mention quota")
	}
	if !strings.Contains(formatted, "Add credits") {
		t.Error("Should include user action")
	}
	if !strings.Contains(formatted, "plandex providers switch") {
		t.Error("Should include command")
	}
}

func TestUnrecoverableErrorFormatCompact(t *testing.T) {
	err := &UnrecoverableError{
		Reason:                  UnrecoverableAuthInvalid,
		Message:                 "Invalid API key",
		PartialRecoveryPossible: true,
		UserActions: []UserAction{
			{Description: "Update API key", Priority: "critical"},
		},
	}

	compact := err.FormatCompact()

	if !strings.Contains(compact, "UNRECOVERABLE") {
		t.Error("Should indicate unrecoverable")
	}
	if !strings.Contains(compact, "auth_invalid") {
		t.Error("Should include reason")
	}
	if !strings.Contains(compact, "1 action(s) required") {
		t.Error("Should include action count")
	}
	if !strings.Contains(compact, "partial recovery possible") {
		t.Error("Should indicate partial recovery")
	}
}

func TestUnrecoverableReasonConstants(t *testing.T) {
	reasons := []struct {
		reason   UnrecoverableReason
		expected string
	}{
		{UnrecoverableQuotaExhausted, "quota_exhausted"},
		{UnrecoverableAuthInvalid, "auth_invalid"},
		{UnrecoverablePermissionDenied, "permission_denied"},
		{UnrecoverableContentPolicy, "content_policy"},
		{UnrecoverableContextTooLong, "context_too_long"},
		{UnrecoverableCheckpointLost, "checkpoint_lost"},
		{UnrecoverableJournalCorrupted, "journal_corrupted"},
		{UnrecoverableSnapshotMissing, "snapshot_missing"},
		{UnrecoverableExternalModification, "external_modification"},
		{UnrecoverableDiskFull, "disk_full"},
	}

	for _, tc := range reasons {
		if string(tc.reason) != tc.expected {
			t.Errorf("UnrecoverableReason %v = %s, want %s", tc.reason, string(tc.reason), tc.expected)
		}
	}
}

func TestUnrecoverableCategoryConstants(t *testing.T) {
	categories := []struct {
		category UnrecoverableCategory
		expected string
	}{
		{CategoryProviderLimit, "provider_limit"},
		{CategoryAuthentication, "authentication"},
		{CategoryDataLoss, "data_loss"},
		{CategoryExternalState, "external_state"},
		{CategorySystemResource, "system_resource"},
	}

	for _, tc := range categories {
		if string(tc.category) != tc.expected {
			t.Errorf("UnrecoverableCategory %v = %s, want %s", tc.category, string(tc.category), tc.expected)
		}
	}
}

func TestEdgeCaseHasPreventionAdvice(t *testing.T) {
	cases := GetUnrecoverableEdgeCases()

	for _, ec := range cases {
		if len(ec.Prevention) == 0 {
			t.Errorf("Edge case %s should have prevention advice", ec.Reason)
		}
	}
}

func TestEdgeCaseUserActionsHavePriority(t *testing.T) {
	cases := GetUnrecoverableEdgeCases()

	validPriorities := map[string]bool{
		"critical": true,
		"high":     true,
		"medium":   true,
		"low":      true,
	}

	for _, ec := range cases {
		for _, action := range ec.UserActions {
			if !validPriorities[action.Priority] {
				t.Errorf("Edge case %s has invalid priority '%s' for action '%s'",
					ec.Reason, action.Priority, action.Description)
			}
		}
	}
}

func TestFindEdgeCase(t *testing.T) {
	// Test finding existing edge case
	ec := findEdgeCase(UnrecoverableQuotaExhausted)
	if ec == nil {
		t.Fatal("Should find quota_exhausted edge case")
	}
	if ec.Reason != UnrecoverableQuotaExhausted {
		t.Errorf("Found wrong edge case: %s", ec.Reason)
	}

	// Test finding non-existent edge case
	ec2 := findEdgeCase(UnrecoverableReason("nonexistent"))
	if ec2 != nil {
		t.Error("Should not find nonexistent edge case")
	}
}

// mockError implements error interface for testing
type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}
