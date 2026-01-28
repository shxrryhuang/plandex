package validation

import (
	"context"
	"testing"
	"time"
)

// TestPreflightValidator tests the preflight validator
func TestPreflightValidator(t *testing.T) {
	t.Run("Create preflight validator", func(t *testing.T) {
		opts := DefaultPreflightOptions()
		pv := NewPreflightValidator(opts)

		if pv == nil {
			t.Error("Should create validator")
		}

		if len(pv.checks) == 0 {
			t.Error("Should have preflight checks")
		}
	})

	t.Run("Preflight checks have required fields", func(t *testing.T) {
		opts := DefaultPreflightOptions()
		pv := NewPreflightValidator(opts)

		for _, check := range pv.checks {
			if check.Name == "" {
				t.Error("Check should have name")
			}
			if check.Description == "" {
				t.Error("Check should have description")
			}
			if check.Execute == nil {
				t.Error("Check should have execute function")
			}
		}
	})

	t.Run("Checks are ordered by priority", func(t *testing.T) {
		opts := DefaultPreflightOptions()
		pv := NewPreflightValidator(opts)

		// Verify highest priority check comes first
		if len(pv.checks) > 0 {
			firstPriority := pv.checks[0].Priority
			for i := 1; i < len(pv.checks); i++ {
				if pv.checks[i].Priority > firstPriority {
					t.Errorf("Checks not in priority order: %d > %d", pv.checks[i].Priority, firstPriority)
				}
				firstPriority = pv.checks[i].Priority
			}
		}
	})
}

// TestRunPreflight tests running preflight validation
func TestRunPreflight(t *testing.T) {
	t.Run("Run preflight with skipped checks", func(t *testing.T) {
		opts := DefaultPreflightOptions()
		opts.SkipDatabase = true
		opts.SkipProvider = true
		opts.SkipLiteLLM = true
		opts.Timeout = 5 * time.Second

		pv := NewPreflightValidator(opts)
		ctx := context.Background()

		result := pv.RunPreflight(ctx)

		if result == nil {
			t.Error("Should return result")
		}

		if result.Duration == 0 {
			t.Error("Should have duration")
		}

		if len(result.Checks) == 0 {
			t.Error("Should have check results")
		}
	})

	t.Run("Preflight result has summary", func(t *testing.T) {
		result := &PreflightResult{
			StartTime:           time.Now(),
			Duration:            100 * time.Millisecond,
			Passed:              5,
			Warnings:            1,
			CriticalFailures:    0,
			NonCriticalFailures: 0,
			Blocked:             false,
		}

		summary := result.Summary()
		if summary == "" {
			t.Error("Should have summary")
		}

		if !containsSubstring(summary, "READY") {
			t.Error("Summary should indicate ready status")
		}
	})

	t.Run("Blocked result shows correct status", func(t *testing.T) {
		result := &PreflightResult{
			StartTime:        time.Now(),
			Duration:         100 * time.Millisecond,
			Passed:           3,
			CriticalFailures: 1,
			Blocked:          true,
		}

		if result.IsReadyToExecute() {
			t.Error("Blocked result should not be ready to execute")
		}

		summary := result.Summary()
		if !containsSubstring(summary, "BLOCKED") {
			t.Error("Summary should indicate blocked status")
		}
	})

	t.Run("Warning result is ready to execute", func(t *testing.T) {
		result := &PreflightResult{
			StartTime:           time.Now(),
			Duration:            100 * time.Millisecond,
			Passed:              5,
			Warnings:            2,
			CriticalFailures:    0,
			NonCriticalFailures: 1,
			Blocked:             false,
		}

		if !result.IsReadyToExecute() {
			t.Error("Result with warnings should be ready to execute")
		}
	})
}

// TestPreflightResult tests preflight result methods
func TestPreflightResult(t *testing.T) {
	t.Run("Get failed checks", func(t *testing.T) {
		result := &PreflightResult{
			Checks: map[string]*CheckResult{
				"check1": {
					Name: "check1",
					Result: &ValidationResult{
						Errors: []*ValidationError{{Summary: "error"}},
					},
				},
				"check2": {
					Name: "check2",
					Result: &ValidationResult{},
				},
			},
		}

		failed := result.GetFailedChecks()
		if len(failed) != 1 {
			t.Errorf("Should have 1 failed check, got %d", len(failed))
		}
	})

	t.Run("Get warning checks", func(t *testing.T) {
		result := &PreflightResult{
			Checks: map[string]*CheckResult{
				"check1": {
					Name: "check1",
					Result: &ValidationResult{
						Warnings: []*ValidationError{{Summary: "warning"}},
					},
				},
				"check2": {
					Name: "check2",
					Result: &ValidationResult{},
				},
			},
		}

		warnings := result.GetWarningChecks()
		if len(warnings) != 1 {
			t.Errorf("Should have 1 warning check, got %d", len(warnings))
		}
	})

	t.Run("IsReadyToExecute with critical failure", func(t *testing.T) {
		result := &PreflightResult{
			CriticalFailures: 1,
		}

		if result.IsReadyToExecute() {
			t.Error("Should not be ready with critical failures")
		}
	})

	t.Run("IsReadyToExecute when blocked", func(t *testing.T) {
		result := &PreflightResult{
			Blocked: true,
		}

		if result.IsReadyToExecute() {
			t.Error("Should not be ready when blocked")
		}
	})

	t.Run("IsReadyToExecute with non-critical failures", func(t *testing.T) {
		result := &PreflightResult{
			NonCriticalFailures: 2,
		}

		if !result.IsReadyToExecute() {
			t.Error("Should be ready with only non-critical failures")
		}
	})
}

// TestRunPreflightValidation tests the convenience function
func TestRunPreflightValidation(t *testing.T) {
	t.Run("Run with skipped checks", func(t *testing.T) {
		ctx := context.Background()

		// This will likely fail without proper setup, but we're testing the function exists
		// and returns appropriate types
		result, err := RunPreflightValidation(ctx)

		// Result should be non-nil even if validation fails
		if result == nil {
			t.Error("Should return result even on failure")
		}

		// Error may or may not be nil depending on system state
		// We're just testing the function can be called
		_ = err
	})
}

// TestQuickPreflightCheck tests the quick preflight check
func TestQuickPreflightCheck(t *testing.T) {
	t.Run("Quick check has reduced timeout", func(t *testing.T) {
		ctx := context.Background()

		start := time.Now()
		result, _ := QuickPreflightCheck(ctx)
		duration := time.Since(start)

		// Quick check should complete within 15 seconds (10s timeout + overhead)
		if duration > 15*time.Second {
			t.Errorf("Quick preflight took too long: %v", duration)
		}

		if result == nil {
			t.Error("Should return result")
		}
	})
}

// TestDefaultPreflightOptions tests default options
func TestDefaultPreflightOptions(t *testing.T) {
	t.Run("Default options are comprehensive", func(t *testing.T) {
		opts := DefaultPreflightOptions()

		if opts.Phase != PhasePreflight {
			t.Error("Phase should be preflight")
		}

		if !opts.CheckFileAccess {
			t.Error("Should check file access for thorough validation")
		}

		if opts.SkipDatabase {
			t.Error("Should not skip database in preflight")
		}

		if opts.SkipProvider {
			t.Error("Should not skip providers in preflight")
		}

		if opts.SkipEnvironment {
			t.Error("Should not skip environment in preflight")
		}

		if opts.SkipLiteLLM {
			t.Error("Should not skip LiteLLM in preflight")
		}
	})
}

// TestValidateSystemResources tests system resource validation
func TestValidateSystemResources(t *testing.T) {
	t.Run("Validate system resources", func(t *testing.T) {
		result := ValidateSystemResources()

		if result == nil {
			t.Error("Should return result")
		}

		// Currently returns success (placeholder implementation)
		if result.HasErrors() {
			t.Error("Placeholder implementation should not have errors")
		}
	})
}

// Helper function for string containment check
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
