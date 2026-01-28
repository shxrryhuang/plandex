package validation

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ValidationPhase represents when validation should occur
type ValidationPhase string

const (
	PhaseStartup   ValidationPhase = "startup"   // Synchronous checks at server startup
	PhaseExecution ValidationPhase = "execution" // Checks before plan execution
	PhaseRuntime   ValidationPhase = "runtime"   // Deferred checks during execution
)

// ValidationOptions configures validation behavior
type ValidationOptions struct {
	// Phase determines what gets validated
	Phase ValidationPhase

	// CheckFileAccess enables file access validation (may be slow)
	CheckFileAccess bool

	// Verbose enables detailed error messages
	Verbose bool

	// Timeout for validation operations
	Timeout time.Duration

	// ProviderNames lists specific providers to validate (empty = all)
	ProviderNames []string

	// SkipDatabase skips database validation
	SkipDatabase bool

	// SkipProvider skips provider validation
	SkipProvider bool

	// SkipEnvironment skips environment validation
	SkipEnvironment bool

	// SkipLiteLLM skips LiteLLM proxy validation
	SkipLiteLLM bool
}

// DefaultStartupOptions returns default options for startup validation
func DefaultStartupOptions() ValidationOptions {
	return ValidationOptions{
		Phase:           PhaseStartup,
		CheckFileAccess: false, // Fast startup check
		Verbose:         true,
		Timeout:         30 * time.Second,
		SkipProvider:    true, // Defer provider checks to execution time
		SkipLiteLLM:     false,
	}
}

// DefaultExecutionOptions returns default options for execution validation
func DefaultExecutionOptions() ValidationOptions {
	return ValidationOptions{
		Phase:           PhaseExecution,
		CheckFileAccess: true, // Thorough check before execution
		Verbose:         true,
		Timeout:         10 * time.Second,
		SkipDatabase:    true, // Already validated at startup
		SkipLiteLLM:     true, // Already validated at startup
	}
}

// Validator orchestrates all validation checks
type Validator struct {
	options ValidationOptions
}

// NewValidator creates a new validator with the given options
func NewValidator(options ValidationOptions) *Validator {
	return &Validator{
		options: options,
	}
}

// ValidateAll runs all configured validation checks
func (v *Validator) ValidateAll(ctx context.Context) *ValidationResult {
	result := &ValidationResult{}

	// Apply timeout
	if v.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.options.Timeout)
		defer cancel()
	}

	// Database validation
	if !v.options.SkipDatabase {
		if dbResult := ValidateDatabase(ctx); !dbResult.IsValid() {
			result.Merge(dbResult)
			// Database is critical - don't continue if it fails
			if !dbResult.IsValid() {
				return result
			}
		}
	}

	// Environment validation
	if !v.options.SkipEnvironment {
		if envResult := ValidateEnvironment(); !envResult.IsValid() || envResult.HasWarnings() {
			result.Merge(envResult)
		}
	}

	// LiteLLM proxy validation
	if !v.options.SkipLiteLLM {
		// First check if port is available (before starting)
		if v.options.Phase == PhaseStartup {
			if litellmResult := ValidateLiteLLMProxy(ctx); !litellmResult.IsValid() || litellmResult.HasWarnings() {
				result.Merge(litellmResult)
			}
		} else {
			// Check health if already running
			if litellmResult := ValidateLiteLLMProxyHealth(ctx); !litellmResult.IsValid() {
				result.Merge(litellmResult)
			}
		}
	}

	// Provider validation
	if !v.options.SkipProvider {
		if len(v.options.ProviderNames) > 0 {
			// Validate specific providers
			for _, provider := range v.options.ProviderNames {
				if providerResult := ValidateProviderCredentials(provider, v.options.CheckFileAccess); !providerResult.IsValid() {
					result.Merge(providerResult)
				}
			}
		} else {
			// Validate all providers
			if providerResult := ValidateAllProviders(v.options.CheckFileAccess); !providerResult.IsValid() {
				result.Merge(providerResult)
			}
		}
	}

	return result
}

// ValidateAndLog runs validation and logs results
func (v *Validator) ValidateAndLog(ctx context.Context) error {
	log.Printf("Running %s validation checks...\n", v.options.Phase)

	result := v.ValidateAll(ctx)

	if result.HasWarnings() {
		log.Println("\n" + FormatResult(result, v.options.Verbose))
	}

	if result.HasErrors() {
		log.Println("\n" + FormatResult(result, v.options.Verbose))
		return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
	}

	log.Println("✅ Validation checks passed")
	return nil
}

// ValidateAndExit runs validation and exits with error code if validation fails
func (v *Validator) ValidateAndExit(ctx context.Context, exitFunc func(int)) {
	if err := v.ValidateAndLog(ctx); err != nil {
		exitFunc(1)
	}
}

// Quick validation functions for common scenarios

// ValidateStartup performs startup validation (database, environment, LiteLLM port)
func ValidateStartup(ctx context.Context) error {
	v := NewValidator(DefaultStartupOptions())
	return v.ValidateAndLog(ctx)
}

// ValidateExecution performs pre-execution validation (providers, environment, LiteLLM health)
func ValidateExecution(ctx context.Context, providers []string) error {
	opts := DefaultExecutionOptions()
	opts.ProviderNames = providers
	v := NewValidator(opts)
	return v.ValidateAndLog(ctx)
}

// ValidateProvider validates a specific provider's credentials
func ValidateProvider(ctx context.Context, providerName string, checkFiles bool) error {
	result := ValidateProviderCredentials(providerName, checkFiles)
	if result.HasErrors() {
		log.Println("\n" + FormatResult(result, true))
		return fmt.Errorf("provider validation failed")
	}
	return nil
}

// QuickDatabaseCheck performs a fast database connectivity check
func QuickDatabaseCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result := ValidateDatabase(ctx)
	if result.HasErrors() {
		log.Println("\n" + FormatResult(result, false))
		return fmt.Errorf("database validation failed")
	}
	return nil
}
