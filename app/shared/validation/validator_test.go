package validation

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestDatabaseValidation_MissingConfig tests validation when no database config is set
func TestDatabaseValidation_MissingConfig(t *testing.T) {
	// Save current environment
	oldVars := map[string]string{
		"DATABASE_URL": os.Getenv("DATABASE_URL"),
		"DB_HOST":      os.Getenv("DB_HOST"),
		"DB_PORT":      os.Getenv("DB_PORT"),
		"DB_USER":      os.Getenv("DB_USER"),
		"DB_PASSWORD":  os.Getenv("DB_PASSWORD"),
		"DB_NAME":      os.Getenv("DB_NAME"),
	}

	// Restore environment after test
	defer func() {
		for k, v := range oldVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Clear all database environment variables
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")

	ctx := context.Background()
	result := ValidateDatabase(ctx)

	if !result.HasErrors() {
		t.Fatal("Expected error for missing database configuration")
	}

	err := result.Errors[0]
	if err.Category != CategoryDatabase {
		t.Errorf("Expected category %s, got %s", CategoryDatabase, err.Category)
	}

	if err.Severity != SeverityCritical {
		t.Errorf("Expected critical severity, got %s", err.Severity)
	}

	if !strings.Contains(err.Summary, "database configuration") {
		t.Errorf("Expected summary to mention database configuration, got: %s", err.Summary)
	}

	if err.Solution == "" {
		t.Error("Error should include a solution")
	}

	if err.Example == "" {
		t.Error("Error should include an example")
	}

	if len(err.RelatedVars) == 0 {
		t.Error("Error should list related variables")
	}
}

// TestDatabaseValidation_IncompleteConfig tests partial DB_* variable configuration
func TestDatabaseValidation_IncompleteConfig(t *testing.T) {
	// Save current environment
	oldVars := map[string]string{
		"DATABASE_URL": os.Getenv("DATABASE_URL"),
		"DB_HOST":      os.Getenv("DB_HOST"),
		"DB_PORT":      os.Getenv("DB_PORT"),
		"DB_USER":      os.Getenv("DB_USER"),
		"DB_PASSWORD":  os.Getenv("DB_PASSWORD"),
		"DB_NAME":      os.Getenv("DB_NAME"),
	}

	// Restore environment after test
	defer func() {
		for k, v := range oldVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Clear all database variables first
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")

	// Set only some variables
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_USER", "test")
	// Missing DB_PASSWORD and DB_NAME

	ctx := context.Background()
	result := ValidateDatabase(ctx)

	if !result.HasErrors() {
		t.Fatal("Expected error for incomplete database configuration")
	}

	err := result.Errors[0]
	if !strings.Contains(err.Summary, "Incomplete") {
		t.Errorf("Expected summary to mention incomplete config, got: %s", err.Summary)
	}

	// Should mention the missing variables
	if !strings.Contains(err.Details, "DB_PASSWORD") || !strings.Contains(err.Details, "DB_NAME") {
		t.Errorf("Expected details to mention missing variables, got: %s", err.Details)
	}
}

// TestProviderValidation_MissingCredentials tests provider validation without credentials
func TestProviderValidation_MissingCredentials(t *testing.T) {
	// Clear OpenAI credentials
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_ORG_ID")

	result := ValidateProviderCredentials("openai", false)

	if !result.HasErrors() {
		t.Fatal("Expected error for missing OpenAI credentials")
	}

	err := result.Errors[0]
	if err.Category != CategoryProvider {
		t.Errorf("Expected provider category, got %s", err.Category)
	}

	if !strings.Contains(err.Summary, "OpenAI") {
		t.Errorf("Expected summary to mention OpenAI, got: %s", err.Summary)
	}

	if !contains(err.RelatedVars, "OPENAI_API_KEY") {
		t.Error("Expected OPENAI_API_KEY in related variables")
	}
}

// TestProviderValidation_ValidCredentials tests provider validation with valid credentials
func TestProviderValidation_ValidCredentials(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key-12345")
	defer os.Unsetenv("OPENAI_API_KEY")

	result := ValidateProviderCredentials("openai", false)

	if result.HasErrors() {
		t.Errorf("Expected no errors with valid credentials, got: %v", result.Errors)
	}
}

// TestValidateAllProviders_NoCredentials tests when no providers are configured
func TestValidateAllProviders_NoCredentials(t *testing.T) {
	// Clear all provider credentials
	providerVars := []string{
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"OPENROUTER_API_KEY",
		"GEMINI_API_KEY",
		"AZURE_OPENAI_API_KEY",
		"DEEPSEEK_API_KEY",
		"PERPLEXITY_API_KEY",
	}

	for _, v := range providerVars {
		os.Unsetenv(v)
	}

	result := ValidateAllProviders(false)

	if !result.HasErrors() {
		t.Fatal("Expected error when no providers are configured")
	}

	// Should have an error about no providers configured
	foundNoProviderError := false
	for _, err := range result.Errors {
		if strings.Contains(err.Summary, "No provider credentials") {
			foundNoProviderError = true
			break
		}
	}

	if !foundNoProviderError {
		t.Error("Expected error about no providers configured")
	}
}

// TestEnvironmentValidation_InvalidPort tests port validation
func TestEnvironmentValidation_InvalidPort(t *testing.T) {
	os.Setenv("PORT", "invalid")
	defer os.Unsetenv("PORT")

	result := ValidateEnvironment()

	if !result.HasErrors() {
		t.Fatal("Expected error for invalid PORT")
	}

	err := result.Errors[0]
	if !strings.Contains(err.Summary, "PORT") {
		t.Errorf("Expected summary to mention PORT, got: %s", err.Summary)
	}
}

// TestEnvironmentValidation_ValidPort tests valid port configuration
func TestEnvironmentValidation_ValidPort(t *testing.T) {
	os.Setenv("PORT", "8099")
	defer os.Unsetenv("PORT")

	result := ValidateEnvironment()

	// Should not have errors for valid port
	hasPortError := false
	for _, err := range result.Errors {
		if strings.Contains(err.Summary, "PORT") {
			hasPortError = true
			break
		}
	}

	if hasPortError {
		t.Error("Should not have error for valid PORT")
	}
}

// TestEnvironmentValidation_ConflictingVars tests conflicting configuration
func TestEnvironmentValidation_ConflictingVars(t *testing.T) {
	// Set both DATABASE_URL and DB_* variables
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("DB_HOST", "localhost")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("DB_HOST")
	}()

	result := ValidateEnvironment()

	// Should have a warning about conflicting vars
	hasConflictWarning := false
	for _, warn := range result.Warnings {
		if strings.Contains(warn.Summary, "DATABASE_URL") && strings.Contains(warn.Summary, "DB_*") {
			hasConflictWarning = true
			break
		}
	}

	if !hasConflictWarning {
		t.Error("Expected warning about conflicting database configuration")
	}
}

// TestValidationResult_Merge tests merging validation results
func TestValidationResult_Merge(t *testing.T) {
	result1 := &ValidationResult{
		Errors: []*ValidationError{
			{Summary: "Error 1"},
		},
		Warnings: []*ValidationError{
			{Summary: "Warning 1"},
		},
	}

	result2 := &ValidationResult{
		Errors: []*ValidationError{
			{Summary: "Error 2"},
		},
		Warnings: []*ValidationError{
			{Summary: "Warning 2"},
		},
	}

	result1.Merge(result2)

	if len(result1.Errors) != 2 {
		t.Errorf("Expected 2 errors after merge, got %d", len(result1.Errors))
	}

	if len(result1.Warnings) != 2 {
		t.Errorf("Expected 2 warnings after merge, got %d", len(result1.Warnings))
	}
}

// TestFormatError tests error formatting
func TestFormatError(t *testing.T) {
	err := &ValidationError{
		Category:    CategoryDatabase,
		Severity:    SeverityError,
		Summary:     "Test error",
		Details:     "This is a test error",
		Impact:      "Testing will fail",
		Solution:    "Fix the test",
		Example:     "export TEST=value",
		RelatedVars: []string{"TEST"},
	}

	formatted := FormatError(err, false)

	// Check that all components are present
	requiredParts := []string{
		"ERROR",
		"Test error",
		"Details:",
		"Impact:",
		"Solution:",
		"Example:",
		"Related variables:",
	}

	for _, part := range requiredParts {
		if !strings.Contains(formatted, part) {
			t.Errorf("Formatted error missing required part: %s", part)
		}
	}
}

// TestFormatResult tests result formatting
func TestFormatResult(t *testing.T) {
	result := &ValidationResult{
		Errors: []*ValidationError{
			{
				Category: CategoryDatabase,
				Severity: SeverityError,
				Summary:  "Error 1",
				Solution: "Fix error 1",
			},
			{
				Category: CategoryProvider,
				Severity: SeverityError,
				Summary:  "Error 2",
				Solution: "Fix error 2",
			},
		},
		Warnings: []*ValidationError{
			{
				Category: CategoryEnvironment,
				Severity: SeverityWarning,
				Summary:  "Warning 1",
				Solution: "Fix warning 1",
			},
		},
	}

	formatted := FormatResult(result, false)

	// Check for error count
	if !strings.Contains(formatted, "2 error(s)") {
		t.Error("Formatted result should show error count")
	}

	// Check for warning count
	if !strings.Contains(formatted, "1 warning(s)") {
		t.Error("Formatted result should show warning count")
	}

	// Check both errors are present
	if !strings.Contains(formatted, "Error 1") || !strings.Contains(formatted, "Error 2") {
		t.Error("Formatted result should include all errors")
	}

	// Check warning is present
	if !strings.Contains(formatted, "Warning 1") {
		t.Error("Formatted result should include warnings")
	}
}

// TestValidatorOptions tests validator with different options
func TestValidatorOptions(t *testing.T) {
	tests := []struct {
		name string
		opts ValidationOptions
	}{
		{
			name: "Startup phase",
			opts: DefaultStartupOptions(),
		},
		{
			name: "Execution phase",
			opts: DefaultExecutionOptions(),
		},
		{
			name: "Custom options",
			opts: ValidationOptions{
				Phase:           PhaseRuntime,
				CheckFileAccess: true,
				Verbose:         false,
				Timeout:         5 * time.Second,
				SkipDatabase:    true,
				SkipProvider:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.opts)
			if validator == nil {
				t.Fatal("NewValidator returned nil")
			}

			// Should be able to validate without panicking
			ctx := context.Background()
			result := validator.ValidateAll(ctx)
			if result == nil {
				t.Fatal("ValidateAll returned nil result")
			}
		})
	}
}

// TestDatabaseConfig_GetConnectionString tests connection string generation
func TestDatabaseConfig_GetConnectionString(t *testing.T) {
	tests := []struct {
		name    string
		config  DatabaseConfig
		wantErr bool
		want    string
	}{
		{
			name: "DATABASE_URL set",
			config: DatabaseConfig{
				DatabaseURL: "postgres://user:pass@localhost:5432/db",
			},
			wantErr: false,
			want:    "postgres://user:pass@localhost:5432/db",
		},
		{
			name: "Individual vars set",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "pass",
				Name:     "db",
			},
			wantErr: false,
			want:    "postgres://user:pass@localhost:5432/db",
		},
		{
			name: "Password with special chars",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "p@ss:word#123",
				Name:     "db",
			},
			wantErr: false,
		},
		{
			name:    "No config",
			config:  DatabaseConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.config.GetConnectionString()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetConnectionString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.want != "" && got != tt.want {
				// For special chars test, just check it's not empty
				if tt.config.Password == "p@ss:word#123" {
					if got == "" {
						t.Error("GetConnectionString() returned empty string")
					}
					if !strings.Contains(got, "postgres://") {
						t.Error("GetConnectionString() should start with postgres://")
					}
				} else if got != tt.want {
					t.Errorf("GetConnectionString() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestValidateFileOrJSONVar tests file and JSON validation
func TestValidateFileOrJSONVar(t *testing.T) {
	tests := []struct {
		name    string
		varName string
		value   string
		wantErr bool
	}{
		{
			name:    "Valid JSON",
			varName: "TEST_VAR",
			value:   `{"key": "value"}`,
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			varName: "TEST_VAR",
			value:   `{"key": invalid}`,
			wantErr: true,
		},
		{
			name:    "Non-existent file",
			varName: "TEST_VAR",
			value:   "/nonexistent/file.json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileOrJSONVar(tt.varName, tt.value, true)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFileOrJSONVar() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// BenchmarkValidateAll benchmarks full validation
func BenchmarkValidateAll(b *testing.B) {
	// Set up some valid config
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	defer os.Unsetenv("OPENAI_API_KEY")

	opts := ValidationOptions{
		Phase:           PhaseStartup,
		CheckFileAccess: false,
		Timeout:         5 * time.Second,
		SkipDatabase:    true,
		SkipLiteLLM:     true,
	}

	validator := NewValidator(opts)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateAll(ctx)
	}
}

// BenchmarkFormatError benchmarks error formatting
func BenchmarkFormatError(b *testing.B) {
	err := &ValidationError{
		Category:    CategoryDatabase,
		Severity:    SeverityError,
		Summary:     "Test error",
		Details:     "This is a test error with details",
		Impact:      "Testing will fail without proper configuration",
		Solution:    "Fix the configuration by setting the required variables",
		Example:     "export TEST=value\nexport OTHER=value2",
		RelatedVars: []string{"TEST", "OTHER", "ANOTHER"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatError(err, false)
	}
}
