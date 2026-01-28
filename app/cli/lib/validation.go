package lib

import (
	"context"
	"fmt"
	"plandex-cli/term"
	shared "plandex-shared"
	"plandex-shared/validation"
	"time"
)

// ValidateExecutionEnvironment performs pre-execution validation checks
// This should be called before starting any plan execution to catch configuration issues early
func ValidateExecutionEnvironment(providerNames []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create validation options for execution phase
	opts := validation.ValidationOptions{
		Phase:           validation.PhaseExecution,
		CheckFileAccess: true,
		Verbose:         true,
		Timeout:         10 * time.Second,
		ProviderNames:   providerNames,
		SkipDatabase:    true,  // CLI doesn't need to check database
		SkipLiteLLM:     false, // Check LiteLLM health
		SkipEnvironment: false, // Check environment config
		SkipProvider:    false, // Check provider credentials
	}

	validator := validation.NewValidator(opts)
	result := validator.ValidateAll(ctx)

	// Show warnings but don't block on them
	if result.HasWarnings() {
		term.StopSpinner()
		fmt.Println()
		fmt.Println(validation.FormatResult(&validation.ValidationResult{Warnings: result.Warnings}, false))
		fmt.Println()
	}

	// Exit if there are errors
	if result.HasErrors() {
		term.StopSpinner()
		fmt.Println()
		fmt.Println(validation.FormatResult(result, true))
		fmt.Println()
		term.OutputErrorAndExit("Configuration validation failed. Please fix the errors above before continuing.")
	}
}

// ValidateProviderQuick performs a quick provider validation for a specific provider
// This is useful for checking a single provider without full validation overhead
func ValidateProviderQuick(providerName string) error {
	result := validation.ValidateProviderCredentials(providerName, true)
	if result.HasErrors() {
		term.StopSpinner()
		fmt.Println()
		fmt.Println(validation.FormatResult(result, true))
		return fmt.Errorf("provider validation failed")
	}
	return nil
}

// EnhancedMustVerifyAuthVars wraps MustVerifyAuthVars with additional validation
// This provides better error messages and catches issues earlier
func EnhancedMustVerifyAuthVars(integratedModels bool, settings *shared.PlanSettings) map[string]string {
	// Get provider names from settings
	var providerNames []string
	if settings != nil {
		opts := settings.GetModelProviderOptions()
		for composite := range opts {
			providerNames = append(providerNames, composite)
		}
	}

	// Run comprehensive validation
	if len(providerNames) > 0 {
		// Extract base provider names (without custom provider suffix)
		baseProviders := extractBaseProviders(providerNames)

		// Only validate if we have non-ollama providers
		needsValidation := false
		for _, provider := range baseProviders {
			if provider != "ollama" {
				needsValidation = true
				break
			}
		}

		if needsValidation {
			ValidateExecutionEnvironment(baseProviders)
		}
	}

	// Call the original credential verification
	return MustVerifyAuthVars(integratedModels)
}

// extractBaseProviders extracts base provider names from composite provider strings
func extractBaseProviders(composites []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, composite := range composites {
		// Composite format is "provider|custom-name" or just "provider"
		// Extract the base provider name
		provider := composite
		if idx := findPipe(composite); idx != -1 {
			provider = composite[:idx]
		}

		// Map provider names to validation provider names
		validationName := mapToValidationProvider(provider)
		if validationName != "" && !seen[validationName] {
			seen[validationName] = true
			result = append(result, validationName)
		}
	}

	return result
}

// findPipe finds the index of the pipe character in a string
func findPipe(s string) int {
	for i, c := range s {
		if c == '|' {
			return i
		}
	}
	return -1
}

// mapToValidationProvider maps Plandex provider names to validation provider names
func mapToValidationProvider(provider string) string {
	mapping := map[string]string{
		"openai":         "openai",
		"openrouter":     "openrouter",
		"anthropic":      "anthropic",
		"anthropic-pro":  "anthropic", // Claude Max uses same credentials
		"google-ai-studio": "google-ai-studio",
		"google-vertex":  "google-vertex",
		"azure-openai":   "azure-openai",
		"deepseek":       "deepseek",
		"perplexity":     "perplexity",
		"aws-bedrock":    "aws-bedrock",
		"ollama":         "", // Skip ollama - no credentials needed
		"custom":         "", // Skip custom providers
	}

	if validationName, exists := mapping[provider]; exists {
		return validationName
	}
	return ""
}

// ShowValidationHelp displays helpful validation information
func ShowValidationHelp() {
	fmt.Println("Configuration Validation Help")
	fmt.Println("=============================")
	fmt.Println()
	fmt.Println("Plandex validates your configuration before execution to catch issues early.")
	fmt.Println()
	fmt.Println("Common issues:")
	fmt.Println("  • Missing API keys - Set environment variables for your chosen providers")
	fmt.Println("  • Invalid file paths - Ensure credential files exist and are readable")
	fmt.Println("  • LiteLLM proxy issues - Check that the proxy is running and accessible")
	fmt.Println("  • Malformed JSON - Validate JSON in credential files or environment variables")
	fmt.Println()
	fmt.Println("For detailed setup instructions, visit:")
	fmt.Println("  https://docs.plandex.ai/models/model-providers")
	fmt.Println()
}
