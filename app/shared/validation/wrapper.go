package validation

import (
	"context"
	"log"
	"plandex-shared/features"
)

// SafeValidator wraps the validation system with feature flag checks
// This ensures validation only runs when explicitly enabled
type SafeValidator struct {
	validator *Validator
	enabled   bool
}

// NewSafeValidator creates a validator that respects feature flags
func NewSafeValidator(opts ValidationOptions) *SafeValidator {
	return &SafeValidator{
		validator: NewValidator(opts),
		enabled:   features.IsValidationEnabled(),
	}
}

// ValidateAll runs validation if enabled, otherwise returns empty result
func (sv *SafeValidator) ValidateAll(ctx context.Context) *ValidationResult {
	if !sv.enabled {
		// Validation disabled - return empty successful result
		return &ValidationResult{}
	}

	// Run validation
	return sv.validator.ValidateAll(ctx)
}

// ValidateAndLog runs validation and logs results if enabled
func (sv *SafeValidator) ValidateAndLog(ctx context.Context) error {
	if !sv.enabled {
		// Validation disabled - silently succeed
		return nil
	}

	return sv.validator.ValidateAndLog(ctx)
}

// IsEnabled returns whether validation is currently enabled
func (sv *SafeValidator) IsEnabled() bool {
	return sv.enabled
}

// SafeValidateStartup validates startup configuration if enabled
// Returns nil if validation is disabled or succeeds
func SafeValidateStartup(ctx context.Context) error {
	if !features.IsStartupValidationEnabled() {
		// Validation disabled
		log.Println("Validation system disabled - skipping startup validation")
		return nil
	}

	log.Println("Running startup validation (validation system enabled)...")
	return ValidateStartup(ctx)
}

// SafeValidateExecution validates execution configuration if enabled
// Returns nil if validation is disabled or succeeds
func SafeValidateExecution(ctx context.Context, providers []string) error {
	if !features.IsExecutionValidationEnabled() {
		// Validation disabled - silently succeed
		return nil
	}

	log.Println("Running execution validation (validation system enabled)...")
	return ValidateExecution(ctx, providers)
}

// SafeValidateProvider validates a provider if validation is enabled
// Returns nil if validation is disabled or succeeds
func SafeValidateProvider(ctx context.Context, providerName string, checkFiles bool) error {
	if !features.IsValidationEnabled() {
		// Validation disabled
		return nil
	}

	return ValidateProvider(ctx, providerName, checkFiles)
}

// SafeQuickDatabaseCheck performs a quick database check if enabled
// Returns nil if validation is disabled or succeeds
func SafeQuickDatabaseCheck(ctx context.Context) error {
	if !features.IsValidationEnabled() {
		// Validation disabled
		return nil
	}

	return QuickDatabaseCheck(ctx)
}

// GetSafeValidationOptions returns validation options based on feature flags
func GetSafeValidationOptions(phase ValidationPhase) ValidationOptions {
	opts := ValidationOptions{
		Phase:   phase,
		Verbose: features.IsVerboseValidationEnabled(),
		Timeout: 30,
	}

	// Configure based on phase
	switch phase {
	case PhaseStartup:
		opts = DefaultStartupOptions()
	case PhaseExecution:
		opts = DefaultExecutionOptions()
	}

	// Override with feature flags
	opts.Verbose = features.IsVerboseValidationEnabled()
	opts.CheckFileAccess = features.IsFileChecksEnabled()

	return opts
}

// ConditionalValidationResult wraps a validation result with feature flag awareness
type ConditionalValidationResult struct {
	Result  *ValidationResult
	Enabled bool
	Strict  bool
}

// ShouldBlock returns true if validation errors should block execution
func (cvr *ConditionalValidationResult) ShouldBlock() bool {
	if !cvr.Enabled {
		return false
	}

	// In strict mode, warnings also block
	if cvr.Strict {
		return cvr.Result.HasErrors() || cvr.Result.HasWarnings()
	}

	// Normal mode - only errors block
	return cvr.Result.HasErrors()
}

// GetFormattedErrors returns formatted errors if validation is enabled
func (cvr *ConditionalValidationResult) GetFormattedErrors() string {
	if !cvr.Enabled {
		return ""
	}

	verbose := features.IsVerboseValidationEnabled()
	return FormatResult(cvr.Result, verbose)
}

// NewConditionalResult creates a conditional validation result
func NewConditionalResult(result *ValidationResult) *ConditionalValidationResult {
	return &ConditionalValidationResult{
		Result:  result,
		Enabled: features.IsValidationEnabled(),
		Strict:  features.IsStrictValidationEnabled(),
	}
}

// SafeValidateWithFallback validates with fallback to original behavior
// If validation is disabled or fails with warnings, it continues execution
// Only hard errors block execution
func SafeValidateWithFallback(ctx context.Context, validateFunc func(context.Context) error, fallbackMsg string) error {
	if !features.IsValidationEnabled() {
		// Validation disabled - use fallback message
		if fallbackMsg != "" {
			log.Println(fallbackMsg)
		}
		return nil
	}

	// Run validation
	err := validateFunc(ctx)
	if err != nil {
		// In non-strict mode, only critical errors block
		if !features.IsStrictValidationEnabled() {
			log.Printf("Validation completed with warnings: %v\n", err)
			log.Println("Continuing execution (non-strict mode)")
			return nil
		}
		return err
	}

	return nil
}
