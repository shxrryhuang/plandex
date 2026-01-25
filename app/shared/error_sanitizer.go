package shared

import (
	"regexp"
	"strings"
)

// =============================================================================
// ERROR SANITIZER - Removes sensitive data from error reports
// =============================================================================

// SanitizeLevel specifies how aggressively to sanitize data
type SanitizeLevel int

const (
	// SanitizeLevelNone keeps all data (for local debug mode only)
	SanitizeLevelNone SanitizeLevel = iota

	// SanitizeLevelStandard removes API keys, tokens, and secrets
	SanitizeLevelStandard

	// SanitizeLevelStrict also normalizes paths and removes IPs
	SanitizeLevelStrict
)

// SensitivePattern defines a pattern to detect and replace sensitive data
type SensitivePattern struct {
	Name        string
	Pattern     *regexp.Regexp
	Replace     string
	MinLevel    SanitizeLevel // Minimum level at which this pattern is applied
	Description string
}

// sensitivePatterns contains all patterns for detecting sensitive data
var sensitivePatterns = []SensitivePattern{
	// ==========================================================================
	// API KEYS (SanitizeLevelStandard)
	// ==========================================================================

	// OpenAI API keys
	{
		Name:        "openai_key",
		Pattern:     regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
		Replace:     "[REDACTED_OPENAI_KEY]",
		MinLevel:    SanitizeLevelStandard,
		Description: "OpenAI API key",
	},
	// OpenAI project keys
	{
		Name:        "openai_project_key",
		Pattern:     regexp.MustCompile(`sk-proj-[a-zA-Z0-9_-]{20,}`),
		Replace:     "[REDACTED_OPENAI_PROJECT_KEY]",
		MinLevel:    SanitizeLevelStandard,
		Description: "OpenAI project API key",
	},
	// Anthropic API keys
	{
		Name:        "anthropic_key",
		Pattern:     regexp.MustCompile(`sk-ant-[a-zA-Z0-9-]{20,}`),
		Replace:     "[REDACTED_ANTHROPIC_KEY]",
		MinLevel:    SanitizeLevelStandard,
		Description: "Anthropic API key",
	},
	// Google API keys
	{
		Name:        "google_api_key",
		Pattern:     regexp.MustCompile(`AIza[a-zA-Z0-9_-]{35}`),
		Replace:     "[REDACTED_GOOGLE_KEY]",
		MinLevel:    SanitizeLevelStandard,
		Description: "Google API key",
	},
	// Azure API keys (typically 32 hex chars)
	{
		Name:        "azure_api_key",
		Pattern:     regexp.MustCompile(`[a-f0-9]{32}`),
		Replace:     "[REDACTED_AZURE_KEY]",
		MinLevel:    SanitizeLevelStandard,
		Description: "Azure API key (32 hex chars)",
	},
	// Generic API key patterns in config/env format
	{
		Name:        "generic_api_key",
		Pattern:     regexp.MustCompile(`(?i)(api[_-]?key|apikey|api_secret|secret_key|access_key)[=:]["']?([a-zA-Z0-9_-]{16,})["']?`),
		Replace:     "$1=[REDACTED]",
		MinLevel:    SanitizeLevelStandard,
		Description: "Generic API key in config format",
	},

	// ==========================================================================
	// TOKENS AND SECRETS (SanitizeLevelStandard)
	// ==========================================================================

	// Bearer tokens
	{
		Name:        "bearer_token",
		Pattern:     regexp.MustCompile(`Bearer\s+[a-zA-Z0-9._-]{20,}`),
		Replace:     "Bearer [REDACTED]",
		MinLevel:    SanitizeLevelStandard,
		Description: "Bearer authentication token",
	},
	// JWT tokens
	{
		Name:        "jwt_token",
		Pattern:     regexp.MustCompile(`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`),
		Replace:     "[REDACTED_JWT]",
		MinLevel:    SanitizeLevelStandard,
		Description: "JWT token",
	},
	// Basic auth credentials
	{
		Name:        "basic_auth",
		Pattern:     regexp.MustCompile(`Basic\s+[a-zA-Z0-9+/=]{20,}`),
		Replace:     "Basic [REDACTED]",
		MinLevel:    SanitizeLevelStandard,
		Description: "Basic authentication credentials",
	},
	// Password patterns in URLs or configs
	{
		Name:        "password_in_url",
		Pattern:     regexp.MustCompile(`(?i)(password|passwd|pwd)[=:]["']?([^"'\s&]{4,})["']?`),
		Replace:     "$1=[REDACTED]",
		MinLevel:    SanitizeLevelStandard,
		Description: "Password in URL or config",
	},
	// Connection strings with credentials
	{
		Name:        "connection_string",
		Pattern:     regexp.MustCompile(`(?i)(mongodb|postgres|mysql|redis)://[^:]+:[^@]+@`),
		Replace:     "$1://[REDACTED]@",
		MinLevel:    SanitizeLevelStandard,
		Description: "Database connection string with credentials",
	},
	// AWS credentials
	{
		Name:        "aws_access_key",
		Pattern:     regexp.MustCompile(`AKIA[A-Z0-9]{16}`),
		Replace:     "[REDACTED_AWS_KEY]",
		MinLevel:    SanitizeLevelStandard,
		Description: "AWS access key ID",
	},
	{
		Name:        "aws_secret_key",
		Pattern:     regexp.MustCompile(`(?i)(aws_secret_access_key|secret_access_key)[=:]["']?([a-zA-Z0-9+/]{40})["']?`),
		Replace:     "$1=[REDACTED]",
		MinLevel:    SanitizeLevelStandard,
		Description: "AWS secret access key",
	},
	// GitHub tokens
	{
		Name:        "github_token",
		Pattern:     regexp.MustCompile(`gh[ps]_[a-zA-Z0-9]{36,}`),
		Replace:     "[REDACTED_GITHUB_TOKEN]",
		MinLevel:    SanitizeLevelStandard,
		Description: "GitHub personal access token",
	},
	{
		Name:        "github_oauth_token",
		Pattern:     regexp.MustCompile(`gho_[a-zA-Z0-9]{36,}`),
		Replace:     "[REDACTED_GITHUB_OAUTH]",
		MinLevel:    SanitizeLevelStandard,
		Description: "GitHub OAuth token",
	},

	// ==========================================================================
	// PATHS (SanitizeLevelStrict)
	// ==========================================================================

	// macOS/Linux home directory paths
	{
		Name:        "home_path_unix",
		Pattern:     regexp.MustCompile(`/Users/([^/\s]+)/`),
		Replace:     "~/",
		MinLevel:    SanitizeLevelStrict,
		Description: "macOS home directory path",
	},
	{
		Name:        "home_path_linux",
		Pattern:     regexp.MustCompile(`/home/([^/\s]+)/`),
		Replace:     "~/",
		MinLevel:    SanitizeLevelStrict,
		Description: "Linux home directory path",
	},
	// Windows user paths
	{
		Name:        "home_path_windows",
		Pattern:     regexp.MustCompile(`C:\\Users\\([^\\]+)\\`),
		Replace:     `C:\Users\[USER]\`,
		MinLevel:    SanitizeLevelStrict,
		Description: "Windows home directory path",
	},
	// Temp file paths that might contain session info
	{
		Name:        "temp_path",
		Pattern:     regexp.MustCompile(`/tmp/[a-zA-Z0-9._-]+`),
		Replace:     "/tmp/[TEMP]",
		MinLevel:    SanitizeLevelStrict,
		Description: "Temporary file path",
	},

	// ==========================================================================
	// NETWORK (SanitizeLevelStrict)
	// ==========================================================================

	// IPv4 addresses - we'll handle localhost exclusion in the sanitize function
	{
		Name:        "ipv4_address",
		Pattern:     regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`),
		Replace:     "[REDACTED_IP]",
		MinLevel:    SanitizeLevelStrict,
		Description: "IPv4 address",
	},
	// Email addresses
	{
		Name:        "email_address",
		Pattern:     regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Replace:     "[REDACTED_EMAIL]",
		MinLevel:    SanitizeLevelStrict,
		Description: "Email address",
	},
}

// SanitizeString applies all sensitive data patterns to a string
func SanitizeString(s string, level SanitizeLevel) string {
	if level == SanitizeLevelNone {
		return s
	}

	result := s
	for _, pattern := range sensitivePatterns {
		if level >= pattern.MinLevel {
			// Special handling for IP addresses - don't redact localhost
			if pattern.Name == "ipv4_address" {
				result = pattern.Pattern.ReplaceAllStringFunc(result, func(match string) string {
					// Don't redact localhost or 0.0.0.0
					if match == "127.0.0.1" || match == "0.0.0.0" {
						return match
					}
					return pattern.Replace
				})
			} else {
				result = pattern.Pattern.ReplaceAllString(result, pattern.Replace)
			}
		}
	}

	return result
}

// SanitizeError creates a sanitized copy of an ErrorReport
func SanitizeError(report *ErrorReport, level SanitizeLevel) *ErrorReport {
	if report == nil || level == SanitizeLevelNone {
		return report
	}

	// Create a deep copy to avoid mutating the original
	sanitized := &ErrorReport{
		Id:        report.Id,
		Timestamp: report.Timestamp,
		Tags:      append([]string{}, report.Tags...),
	}

	// Sanitize root cause
	if report.RootCause != nil {
		sanitized.RootCause = &RootCause{
			Category:        report.RootCause.Category,
			Type:            report.RootCause.Type,
			Code:            report.RootCause.Code,
			Message:         SanitizeString(report.RootCause.Message, level),
			OriginalError:   SanitizeString(report.RootCause.OriginalError, level),
			HTTPCode:        report.RootCause.HTTPCode,
			Provider:        report.RootCause.Provider,
			StackTrace:      SanitizeString(report.RootCause.StackTrace, level),
			ProviderFailure: sanitizeProviderFailure(report.RootCause.ProviderFailure, level),
		}
	}

	// Sanitize step context
	if report.StepContext != nil {
		sanitized.StepContext = sanitizeStepContext(report.StepContext, level)
	}

	// Sanitize recovery actions
	if report.Recovery != nil {
		sanitized.Recovery = sanitizeRecovery(report.Recovery, level)
	}

	// Sanitize related errors (recursively)
	if len(report.RelatedErrors) > 0 {
		sanitized.RelatedErrors = make([]*ErrorReport, len(report.RelatedErrors))
		for i, related := range report.RelatedErrors {
			sanitized.RelatedErrors[i] = SanitizeError(related, level)
		}
	}

	return sanitized
}

func sanitizeProviderFailure(pf *ProviderFailure, level SanitizeLevel) *ProviderFailure {
	if pf == nil {
		return nil
	}

	return &ProviderFailure{
		Type:              pf.Type,
		Category:          pf.Category,
		HTTPCode:          pf.HTTPCode,
		Message:           SanitizeString(pf.Message, level),
		Provider:          pf.Provider,
		Retryable:         pf.Retryable,
		RetryAfterSeconds: pf.RetryAfterSeconds,
		MaxRetries:        pf.MaxRetries,
		RequiresAction:    SanitizeString(pf.RequiresAction, level),
		FallbackSuggested: pf.FallbackSuggested,
	}
}

func sanitizeStepContext(sc *StepContext, level SanitizeLevel) *StepContext {
	if sc == nil {
		return nil
	}

	sanitized := &StepContext{
		PlanId:           sc.PlanId,
		Branch:           sc.Branch,
		JournalEntryId:   sc.JournalEntryId,
		JournalEntrySeq:  sc.JournalEntrySeq,
		EntryType:        sc.EntryType,
		Phase:            sc.Phase,
		Operation:        SanitizeString(sc.Operation, level),
		TransactionId:    sc.TransactionId,
		TransactionState: sc.TransactionState,
		OperationIndex:   sc.OperationIndex,
		FilePath:         SanitizeString(sc.FilePath, level),
		LastCheckpoint:   sc.LastCheckpoint,
	}

	if sc.ModelContext != nil {
		sanitized.ModelContext = &ModelContext{
			Model:           sc.ModelContext.Model,
			Provider:        sc.ModelContext.Provider,
			TokensIn:        sc.ModelContext.TokensIn,
			TokensOut:       sc.ModelContext.TokensOut,
			StreamProgress:  sc.ModelContext.StreamProgress,
			PartialResponse: SanitizeString(sc.ModelContext.PartialResponse, level),
			RequestDuration: sc.ModelContext.RequestDuration,
			RetryAttempt:    sc.ModelContext.RetryAttempt,
		}
	}

	if len(sc.PreviousSteps) > 0 {
		sanitized.PreviousSteps = make([]StepSummary, len(sc.PreviousSteps))
		copy(sanitized.PreviousSteps, sc.PreviousSteps)
	}

	return sanitized
}

func sanitizeRecovery(r *RecoveryAction, level SanitizeLevel) *RecoveryAction {
	if r == nil {
		return nil
	}

	sanitized := &RecoveryAction{
		CanAutoRecover:     r.CanAutoRecover,
		AutoRecoveryAction: SanitizeString(r.AutoRecoveryAction, level),
		RollbackRequired:   r.RollbackRequired,
		RollbackTarget:     r.RollbackTarget,
	}

	if r.RetryStrategy != nil {
		sanitized.RetryStrategy = &RetryStrategy{
			ShouldRetry:       r.RetryStrategy.ShouldRetry,
			MaxAttempts:       r.RetryStrategy.MaxAttempts,
			InitialDelayMs:    r.RetryStrategy.InitialDelayMs,
			MaxDelayMs:        r.RetryStrategy.MaxDelayMs,
			BackoffMultiplier: r.RetryStrategy.BackoffMultiplier,
			UseJitter:         r.RetryStrategy.UseJitter,
			RespectRetryAfter: r.RetryStrategy.RespectRetryAfter,
			TryFallbackFirst:  r.RetryStrategy.TryFallbackFirst,
			Notes:             SanitizeString(r.RetryStrategy.Notes, level),
		}
	}

	if len(r.ManualActions) > 0 {
		sanitized.ManualActions = make([]ManualAction, len(r.ManualActions))
		for i, action := range r.ManualActions {
			sanitized.ManualActions[i] = ManualAction{
				Description: SanitizeString(action.Description, level),
				Priority:    action.Priority,
				Command:     SanitizeString(action.Command, level),
				Link:        action.Link, // Keep links as they're usually documentation
			}
		}
	}

	if len(r.AlternativeApproaches) > 0 {
		sanitized.AlternativeApproaches = make([]string, len(r.AlternativeApproaches))
		for i, alt := range r.AlternativeApproaches {
			sanitized.AlternativeApproaches[i] = SanitizeString(alt, level)
		}
	}

	if len(r.DocumentationLinks) > 0 {
		sanitized.DocumentationLinks = append([]string{}, r.DocumentationLinks...)
	}

	return sanitized
}

// =============================================================================
// UTILITY FUNCTIONS
// =============================================================================

// MaskAPIKey returns a masked version of an API key for display
// Shows first 4 and last 4 characters: sk-1234...abcd
func MaskAPIKey(key string) string {
	if len(key) <= 12 {
		return "[HIDDEN]"
	}
	return key[:7] + "..." + key[len(key)-4:]
}

// ContainsSensitiveData checks if a string contains any sensitive patterns
func ContainsSensitiveData(s string) bool {
	for _, pattern := range sensitivePatterns {
		if pattern.Pattern.MatchString(s) {
			return true
		}
	}
	return false
}

// GetSensitivePatternNames returns the names of all patterns that match in a string
func GetSensitivePatternNames(s string) []string {
	var names []string
	for _, pattern := range sensitivePatterns {
		if pattern.Pattern.MatchString(s) {
			names = append(names, pattern.Name)
		}
	}
	return names
}

// SanitizeMap sanitizes all string values in a map
func SanitizeMap(m map[string]interface{}, level SanitizeLevel) map[string]interface{} {
	if level == SanitizeLevelNone {
		return m
	}

	result := make(map[string]interface{})
	for k, v := range m {
		switch val := v.(type) {
		case string:
			result[k] = SanitizeString(val, level)
		case map[string]interface{}:
			result[k] = SanitizeMap(val, level)
		case []interface{}:
			result[k] = sanitizeSlice(val, level)
		default:
			result[k] = v
		}
	}
	return result
}

func sanitizeSlice(s []interface{}, level SanitizeLevel) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case string:
			result[i] = SanitizeString(val, level)
		case map[string]interface{}:
			result[i] = SanitizeMap(val, level)
		case []interface{}:
			result[i] = sanitizeSlice(val, level)
		default:
			result[i] = v
		}
	}
	return result
}

// =============================================================================
// SANITIZE LEVEL HELPERS
// =============================================================================

// ParseSanitizeLevel parses a string into a SanitizeLevel
func ParseSanitizeLevel(s string) SanitizeLevel {
	switch strings.ToLower(s) {
	case "none", "0":
		return SanitizeLevelNone
	case "standard", "1":
		return SanitizeLevelStandard
	case "strict", "2":
		return SanitizeLevelStrict
	default:
		return SanitizeLevelStandard
	}
}

// String returns the string representation of a SanitizeLevel
func (l SanitizeLevel) String() string {
	switch l {
	case SanitizeLevelNone:
		return "none"
	case SanitizeLevelStandard:
		return "standard"
	case SanitizeLevelStrict:
		return "strict"
	default:
		return "unknown"
	}
}
