package validation

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ProviderConfig holds provider-specific configuration
type ProviderConfig struct {
	Name         string
	EnvVars      []string
	Required     []string
	Optional     []string
	FileVars     []string // Variables that might be file paths
	Instructions string   // Provider-specific setup instructions
	DocsURL      string   // Link to provider docs
}

// ValidateProviderCredentials validates credentials for a specific provider
func ValidateProviderCredentials(providerName string, checkFileAccess bool) *ValidationResult {
	result := &ValidationResult{}

	config, exists := getProviderConfig(providerName)
	if !exists {
		result.AddError(&ValidationError{
			Category: CategoryProvider,
			Severity: SeverityWarning,
			Summary:  fmt.Sprintf("Unknown provider: %s", providerName),
			Details:  "Provider validation configuration not found.",
			Solution: "Check provider name or skip validation for custom providers.",
		})
		return result
	}

	var missing []string
	var invalid []string

	// Check required variables
	for _, envVar := range config.Required {
		value := os.Getenv(envVar)
		if value == "" {
			missing = append(missing, envVar)
			continue
		}

		// Validate format if it's a file variable
		isFileVar := contains(config.FileVars, envVar)
		if isFileVar {
			if err := validateFileOrJSONVar(envVar, value, checkFileAccess); err != nil {
				result.AddError(err)
				invalid = append(invalid, envVar)
			}
		}
	}

	if len(missing) > 0 {
		result.AddError(&ValidationError{
			Category:    CategoryProvider,
			Severity:    SeverityError,
			Summary:     fmt.Sprintf("Missing required credentials for %s", config.Name),
			Details:     fmt.Sprintf("The following required environment variables are not set: %s", strings.Join(missing, ", ")),
			Impact:      fmt.Sprintf("Cannot use %s models without these credentials.", config.Name),
			Solution:    config.Instructions,
			RelatedVars: missing,
			Example:     getProviderExample(providerName),
		})
	}

	// Check optional variables if they're set
	for _, envVar := range config.Optional {
		value := os.Getenv(envVar)
		if value != "" {
			isFileVar := contains(config.FileVars, envVar)
			if isFileVar {
				if err := validateFileOrJSONVar(envVar, value, checkFileAccess); err != nil {
					result.AddError(err)
					invalid = append(invalid, envVar)
				}
			}
		}
	}

	return result
}

// ValidateAllProviders checks which providers have valid credentials
func ValidateAllProviders(checkFileAccess bool) *ValidationResult {
	result := &ValidationResult{}

	providers := []string{
		"openai",
		"anthropic",
		"openrouter",
		"google-ai-studio",
		"google-vertex",
		"azure-openai",
		"deepseek",
		"perplexity",
		"aws-bedrock",
	}

	validProviders := []string{}
	invalidProviders := []string{}

	for _, provider := range providers {
		providerResult := ValidateProviderCredentials(provider, checkFileAccess)
		if providerResult.IsValid() {
			validProviders = append(validProviders, provider)
		} else {
			// Only add errors, not all provider validations
			if providerResult.HasErrors() {
				for _, err := range providerResult.Errors {
					// Don't add missing credential errors - those are expected
					if !strings.Contains(err.Summary, "Missing required credentials") {
						result.AddError(err)
					}
				}
			}
			invalidProviders = append(invalidProviders, provider)
		}
	}

	// If no providers are valid, add a helpful error
	if len(validProviders) == 0 {
		result.AddError(&ValidationError{
			Category: CategoryProvider,
			Severity: SeverityError,
			Summary:  "No provider credentials configured",
			Details:  "No valid credentials found for any AI model provider.",
			Impact:   "Cannot execute plans without at least one configured provider.",
			Solution: `Configure credentials for at least one provider:

Quick start with OpenRouter (supports multiple models):
  1. Sign up at https://openrouter.ai
  2. Generate an API key
  3. Set: export OPENROUTER_API_KEY=your_key

Or configure a specific provider:
  - OpenAI: export OPENAI_API_KEY=your_key
  - Anthropic: export ANTHROPIC_API_KEY=your_key
  - Google: export GEMINI_API_KEY=your_key

See https://docs.plandex.ai/models/model-providers for full details.`,
			Example: `export OPENROUTER_API_KEY="sk-or-v1-..."
export OPENAI_API_KEY="sk-proj-..."
export ANTHROPIC_API_KEY="sk-ant-..."`,
		})
	}

	return result
}

// validateFileOrJSONVar validates a variable that could be a file path or JSON content
func validateFileOrJSONVar(varName, value string, checkFileAccess bool) *ValidationError {
	trimmed := strings.TrimSpace(value)

	// Check if it's JSON
	if strings.HasPrefix(trimmed, "{") {
		var js map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &js); err != nil {
			return &ValidationError{
				Category: CategoryFormat,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("Invalid JSON format in %s", varName),
				Details:  fmt.Sprintf("Value appears to be JSON but is malformed: %v", err),
				Impact:   "Provider cannot be used with invalid JSON credentials.",
				Solution: "Fix the JSON format or provide a valid file path instead.",
				Example: `Valid JSON format:
  export %s='{"key": "value", "nested": {"data": "here"}}'

Or use a file path:
  export %s="/path/to/credentials.json"`,
				RelatedVars: []string{varName},
				Err:         err,
			}
		}
		return nil // Valid JSON
	}

	// Check if it's base64 encoded
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil {
		decodedStr := string(decoded)
		if strings.HasPrefix(strings.TrimSpace(decodedStr), "{") {
			var js map[string]interface{}
			if err := json.Unmarshal(decoded, &js); err != nil {
				return &ValidationError{
					Category:    CategoryFormat,
					Severity:    SeverityError,
					Summary:     fmt.Sprintf("Invalid base64-encoded JSON in %s", varName),
					Details:     "Value is base64 encoded but contains malformed JSON",
					Impact:      "Provider cannot be used with invalid credentials.",
					Solution:    "Fix the JSON content before base64 encoding, or provide a file path.",
					RelatedVars: []string{varName},
					Err:         err,
				}
			}
			return nil // Valid base64-encoded JSON
		}
	}

	// Assume it's a file path - validate if checkFileAccess is true
	if checkFileAccess {
		if _, err := os.Stat(value); os.IsNotExist(err) {
			return &ValidationError{
				Category: CategoryFilePath,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("File not found for %s", varName),
				Details:  fmt.Sprintf("File path '%s' does not exist", value),
				Impact:   "Provider cannot load credentials from missing file.",
				Solution: fmt.Sprintf(`Create the credentials file or fix the path:
  1. Verify the path is correct
  2. Ensure the file exists: ls -l "%s"
  3. Check file permissions are readable

Or provide credentials directly as JSON:
  export %s='{"key": "value"}'`, value, varName),
				RelatedVars: []string{varName},
				Err:         err,
			}
		} else if err != nil {
			return &ValidationError{
				Category:    CategoryFilePath,
				Severity:    SeverityError,
				Summary:     fmt.Sprintf("Cannot access file for %s", varName),
				Details:     fmt.Sprintf("Error accessing file '%s': %v", value, err),
				Impact:      "Provider cannot read credentials file.",
				Solution:    "Check file permissions and ensure the file is readable.",
				RelatedVars: []string{varName},
				Err:         err,
			}
		}

		// Try to read and validate the file content
		content, err := os.ReadFile(value)
		if err != nil {
			return &ValidationError{
				Category:    CategoryFilePath,
				Severity:    SeverityError,
				Summary:     fmt.Sprintf("Cannot read file for %s", varName),
				Details:     fmt.Sprintf("Error reading file '%s': %v", value, err),
				Impact:      "Provider cannot load credentials from unreadable file.",
				Solution:    fmt.Sprintf("Fix file permissions: chmod 644 %s", value),
				RelatedVars: []string{varName},
				Err:         err,
			}
		}

		// Validate JSON content if the file contains JSON
		if strings.HasPrefix(strings.TrimSpace(string(content)), "{") {
			var js map[string]interface{}
			if err := json.Unmarshal(content, &js); err != nil {
				return &ValidationError{
					Category:    CategoryFormat,
					Severity:    SeverityError,
					Summary:     fmt.Sprintf("Invalid JSON in credentials file for %s", varName),
					Details:     fmt.Sprintf("File '%s' contains invalid JSON: %v", value, err),
					Impact:      "Provider cannot parse malformed JSON credentials.",
					Solution:    "Fix the JSON format in the credentials file.",
					RelatedVars: []string{varName},
					Err:         err,
				}
			}
		}
	}

	return nil
}

// getProviderConfig returns the validation configuration for a provider
func getProviderConfig(provider string) (ProviderConfig, bool) {
	configs := map[string]ProviderConfig{
		"openai": {
			Name:     "OpenAI",
			Required: []string{"OPENAI_API_KEY"},
			Optional: []string{"OPENAI_ORG_ID"},
			Instructions: `Configure OpenAI credentials:
  1. Get an API key from https://platform.openai.com/api-keys
  2. Set: export OPENAI_API_KEY=your_key`,
			DocsURL: "https://platform.openai.com/docs",
		},
		"anthropic": {
			Name:     "Anthropic",
			Required: []string{"ANTHROPIC_API_KEY"},
			Instructions: `Configure Anthropic credentials:
  1. Get an API key from https://console.anthropic.com/settings/keys
  2. Set: export ANTHROPIC_API_KEY=your_key`,
			DocsURL: "https://docs.anthropic.com",
		},
		"openrouter": {
			Name:     "OpenRouter",
			Required: []string{"OPENROUTER_API_KEY"},
			Instructions: `Configure OpenRouter credentials:
  1. Sign up at https://openrouter.ai/sign-up
  2. Buy credits at https://openrouter.ai/settings/credits
  3. Generate a key at https://openrouter.ai/settings/keys
  4. Set: export OPENROUTER_API_KEY=your_key`,
			DocsURL: "https://openrouter.ai/docs",
		},
		"google-ai-studio": {
			Name:     "Google AI Studio",
			Required: []string{"GEMINI_API_KEY"},
			Instructions: `Configure Google AI Studio credentials:
  1. Get an API key from https://aistudio.google.com/app/apikey
  2. Set: export GEMINI_API_KEY=your_key`,
			DocsURL: "https://ai.google.dev/gemini-api/docs",
		},
		"google-vertex": {
			Name:     "Google Vertex AI",
			Required: []string{"GOOGLE_APPLICATION_CREDENTIALS", "VERTEXAI_PROJECT", "VERTEXAI_LOCATION"},
			FileVars: []string{"GOOGLE_APPLICATION_CREDENTIALS"},
			Instructions: `Configure Google Vertex AI credentials:
  1. Create a service account in Google Cloud Console
  2. Download the JSON key file
  3. Set environment variables:
     export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
     export VERTEXAI_PROJECT=your-project-id
     export VERTEXAI_LOCATION=us-central1`,
			DocsURL: "https://cloud.google.com/vertex-ai/docs",
		},
		"azure-openai": {
			Name:     "Azure OpenAI",
			Required: []string{"AZURE_OPENAI_API_KEY", "AZURE_API_BASE"},
			Optional: []string{"AZURE_API_VERSION", "AZURE_DEPLOYMENTS_MAP"},
			FileVars: []string{"AZURE_DEPLOYMENTS_MAP"},
			Instructions: `Configure Azure OpenAI credentials:
  1. Get your API key and endpoint from Azure Portal
  2. Set environment variables:
     export AZURE_OPENAI_API_KEY=your_key
     export AZURE_API_BASE=https://your-resource.openai.azure.com
     export AZURE_API_VERSION=2025-04-01-preview (optional)`,
			DocsURL: "https://learn.microsoft.com/azure/ai-services/openai/",
		},
		"deepseek": {
			Name:     "DeepSeek",
			Required: []string{"DEEPSEEK_API_KEY"},
			Instructions: `Configure DeepSeek credentials:
  1. Get an API key from https://platform.deepseek.com
  2. Set: export DEEPSEEK_API_KEY=your_key`,
			DocsURL: "https://platform.deepseek.com/docs",
		},
		"perplexity": {
			Name:     "Perplexity",
			Required: []string{"PERPLEXITY_API_KEY"},
			Instructions: `Configure Perplexity credentials:
  1. Get an API key from https://www.perplexity.ai/settings/api
  2. Set: export PERPLEXITY_API_KEY=your_key`,
			DocsURL: "https://docs.perplexity.ai",
		},
		"aws-bedrock": {
			Name:     "AWS Bedrock",
			Required: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"},
			Optional: []string{"AWS_SESSION_TOKEN", "AWS_INFERENCE_PROFILE_ARN"},
			Instructions: `Configure AWS Bedrock credentials:
  Option 1: Using AWS profile
    export PLANDEX_AWS_PROFILE=your-profile

  Option 2: Using environment variables
    export AWS_ACCESS_KEY_ID=your_key_id
    export AWS_SECRET_ACCESS_KEY=your_secret_key
    export AWS_REGION=us-east-1`,
			DocsURL: "https://docs.aws.amazon.com/bedrock/",
		},
	}

	config, exists := configs[provider]
	return config, exists
}

// getProviderExample returns an example configuration for a provider
func getProviderExample(provider string) string {
	examples := map[string]string{
		"openai": `export OPENAI_API_KEY="sk-proj-..."
export OPENAI_ORG_ID="org-..." # optional`,
		"anthropic": `export ANTHROPIC_API_KEY="sk-ant-..."`,
		"openrouter": `export OPENROUTER_API_KEY="sk-or-v1-..."`,
		"google-ai-studio": `export GEMINI_API_KEY="AIza..."`,
		"google-vertex": `export GOOGLE_APPLICATION_CREDENTIALS="/path/to/credentials.json"
export VERTEXAI_PROJECT="my-project-id"
export VERTEXAI_LOCATION="us-central1"`,
		"azure-openai": `export AZURE_OPENAI_API_KEY="your-key"
export AZURE_API_BASE="https://your-resource.openai.azure.com"
export AZURE_API_VERSION="2025-04-01-preview" # optional`,
		"deepseek": `export DEEPSEEK_API_KEY="sk-..."`,
		"perplexity": `export PERPLEXITY_API_KEY="pplx-..."`,
		"aws-bedrock": `# Option 1: Using profile
export PLANDEX_AWS_PROFILE="default"

# Option 2: Using credentials
export AWS_ACCESS_KEY_ID="AKIA..."
export AWS_SECRET_ACCESS_KEY="..."
export AWS_REGION="us-east-1"`,
	}

	if example, exists := examples[provider]; exists {
		return example
	}
	return ""
}

// contains checks if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
