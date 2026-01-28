package validation

import (
	"testing"
)

// TestNilSafety tests that all functions handle nil inputs gracefully
func TestNilSafety(t *testing.T) {
	t.Run("ValidationResult nil receiver", func(t *testing.T) {
		var result *ValidationResult

		// These should not panic
		if result.HasErrors() {
			t.Error("nil ValidationResult should not have errors")
		}
		if result.HasWarnings() {
			t.Error("nil ValidationResult should not have warnings")
		}
		if !result.IsValid() {
			t.Error("nil ValidationResult should be valid")
		}
	})

	t.Run("AddError with nil result", func(t *testing.T) {
		var result *ValidationResult
		err := &ValidationError{
			Category: CategoryDatabase,
			Severity: SeverityError,
			Summary:  "Test error",
		}

		// Should not panic
		result.AddError(err)
	})

	t.Run("AddError with nil error", func(t *testing.T) {
		result := &ValidationResult{}

		// Should not panic
		result.AddError(nil)

		if result.HasErrors() {
			t.Error("Should not have added nil error")
		}
	})

	t.Run("Merge with nil result", func(t *testing.T) {
		var result *ValidationResult
		other := &ValidationResult{
			Errors: []*ValidationError{
				{
					Category: CategoryDatabase,
					Severity: SeverityError,
					Summary:  "Test",
				},
			},
		}

		// Should not panic
		result.Merge(other)
	})

	t.Run("Merge with nil other", func(t *testing.T) {
		result := &ValidationResult{}

		// Should not panic
		result.Merge(nil)

		if result.HasErrors() {
			t.Error("Should not have errors after merging nil")
		}
	})

	t.Run("FormatError with nil", func(t *testing.T) {
		// Should not panic and return empty string
		formatted := FormatError(nil, false)
		if formatted != "" {
			t.Error("FormatError(nil) should return empty string")
		}
	})

	t.Run("FormatResult with nil", func(t *testing.T) {
		// Should not panic
		formatted := FormatResult(nil, false)
		if formatted == "" {
			t.Error("FormatResult(nil) should return success message")
		}
	})

	t.Run("AddError with both nil", func(t *testing.T) {
		var result *ValidationResult

		// Should not panic
		result.AddError(nil)
	})

	t.Run("Merge with both nil", func(t *testing.T) {
		var result *ValidationResult

		// Should not panic
		result.Merge(nil)
	})
}

// TestNilReceiverBehavior verifies the expected behavior of nil receivers
func TestNilReceiverBehavior(t *testing.T) {
	var result *ValidationResult

	// Nil result should be considered valid (no errors)
	if !result.IsValid() {
		t.Error("nil result should be valid")
	}

	// Nil result should have no errors
	if result.HasErrors() {
		t.Error("nil result should have no errors")
	}

	// Nil result should have no warnings
	if result.HasWarnings() {
		t.Error("nil result should have no warnings")
	}
}

// TestSafeOperations tests that operations on valid results still work
func TestSafeOperations(t *testing.T) {
	result := &ValidationResult{}

	// Add an error
	result.AddError(&ValidationError{
		Category: CategoryDatabase,
		Severity: SeverityError,
		Summary:  "Test error",
	})

	if !result.HasErrors() {
		t.Error("Should have errors after adding one")
	}

	if result.IsValid() {
		t.Error("Should not be valid with errors")
	}

	// Add a warning
	result.AddError(&ValidationError{
		Category: CategoryEnvironment,
		Severity: SeverityWarning,
		Summary:  "Test warning",
	})

	if !result.HasWarnings() {
		t.Error("Should have warnings after adding one")
	}

	// Merge with another result
	other := &ValidationResult{
		Errors: []*ValidationError{
			{
				Category: CategoryProvider,
				Severity: SeverityError,
				Summary:  "Another error",
			},
		},
	}

	result.Merge(other)

	if len(result.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(result.Errors))
	}

	// Format should work
	formatted := FormatError(result.Errors[0], false)
	if formatted == "" {
		t.Error("FormatError should return non-empty string for valid error")
	}

	resultFormatted := FormatResult(result, false)
	if resultFormatted == "" {
		t.Error("FormatResult should return non-empty string for result with errors")
	}
}
