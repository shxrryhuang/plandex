package validation

import (
	"fmt"
	"strings"
)

// ValidationErrorCategory represents the type of validation error
type ValidationErrorCategory string

const (
	CategoryDatabase    ValidationErrorCategory = "database"
	CategoryProvider    ValidationErrorCategory = "provider"
	CategoryEnvironment ValidationErrorCategory = "environment"
	CategoryFilePath    ValidationErrorCategory = "file_path"
	CategoryNetwork     ValidationErrorCategory = "network"
	CategoryPermission  ValidationErrorCategory = "permission"
	CategoryFormat      ValidationErrorCategory = "format"
)

// ValidationErrorSeverity indicates how critical the error is
type ValidationErrorSeverity string

const (
	SeverityCritical ValidationErrorSeverity = "critical" // Blocks startup
	SeverityError    ValidationErrorSeverity = "error"    // Blocks execution
	SeverityWarning  ValidationErrorSeverity = "warning"  // Informational only
)

// ValidationError represents a single configuration validation failure
type ValidationError struct {
	// Category of the error (database, provider, etc.)
	Category ValidationErrorCategory

	// Severity of the error
	Severity ValidationErrorSeverity

	// Short, user-friendly summary
	Summary string

	// Detailed description of what went wrong
	Details string

	// Why this matters and what will fail without fixing
	Impact string

	// Clear, actionable steps to fix the issue
	Solution string

	// Example of correct configuration
	Example string

	// Related environment variables or config keys
	RelatedVars []string

	// Underlying error, if any
	Err error
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Category, e.Severity, e.Summary)
}

// ValidationResult contains all validation errors found
type ValidationResult struct {
	// All errors found during validation
	Errors []*ValidationError

	// Warnings that don't block execution
	Warnings []*ValidationError
}

// HasErrors returns true if there are any errors
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any warnings
func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// IsValid returns true if there are no errors (warnings are OK)
func (r *ValidationResult) IsValid() bool {
	return !r.HasErrors()
}

// AddError adds a validation error
func (r *ValidationResult) AddError(err *ValidationError) {
	if err.Severity == SeverityWarning {
		r.Warnings = append(r.Warnings, err)
	} else {
		r.Errors = append(r.Errors, err)
	}
}

// Merge combines results from multiple validations
func (r *ValidationResult) Merge(other *ValidationResult) {
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
}

// FormatError formats a single validation error for display
func FormatError(err *ValidationError, verbose bool) string {
	var b strings.Builder

	// Header with emoji and severity
	emoji := getEmojiForCategory(err.Category)
	b.WriteString(fmt.Sprintf("%s %s: %s\n", emoji, strings.ToUpper(string(err.Severity)), err.Summary))

	if err.Details != "" {
		b.WriteString(fmt.Sprintf("\n📋 Details:\n  %s\n", err.Details))
	}

	if err.Impact != "" {
		b.WriteString(fmt.Sprintf("\n⚠️  Impact:\n  %s\n", err.Impact))
	}

	b.WriteString(fmt.Sprintf("\n✅ Solution:\n  %s\n", err.Solution))

	if err.Example != "" {
		b.WriteString(fmt.Sprintf("\n💡 Example:\n%s\n", indentText(err.Example, "  ")))
	}

	if len(err.RelatedVars) > 0 {
		b.WriteString(fmt.Sprintf("\n🔑 Related variables:\n  %s\n", strings.Join(err.RelatedVars, ", ")))
	}

	if verbose && err.Err != nil {
		b.WriteString(fmt.Sprintf("\n🐛 Underlying error:\n  %s\n", err.Err.Error()))
	}

	return b.String()
}

// FormatResult formats all validation errors for display
func FormatResult(result *ValidationResult, verbose bool) string {
	if result.IsValid() && !result.HasWarnings() {
		return "✅ All validation checks passed!\n"
	}

	var b strings.Builder

	if result.HasErrors() {
		b.WriteString("❌ Configuration validation failed\n")
		b.WriteString(strings.Repeat("=", 80) + "\n\n")

		for i, err := range result.Errors {
			b.WriteString(FormatError(err, verbose))
			if i < len(result.Errors)-1 {
				b.WriteString("\n" + strings.Repeat("-", 80) + "\n\n")
			}
		}
	}

	if result.HasWarnings() {
		if result.HasErrors() {
			b.WriteString("\n" + strings.Repeat("=", 80) + "\n\n")
		}
		b.WriteString("⚠️  Configuration warnings\n")
		b.WriteString(strings.Repeat("=", 80) + "\n\n")

		for i, warning := range result.Warnings {
			b.WriteString(FormatError(warning, verbose))
			if i < len(result.Warnings)-1 {
				b.WriteString("\n" + strings.Repeat("-", 80) + "\n\n")
			}
		}
	}

	if result.HasErrors() {
		b.WriteString("\n" + strings.Repeat("=", 80) + "\n")
		b.WriteString(fmt.Sprintf("Found %d error(s)", len(result.Errors)))
		if result.HasWarnings() {
			b.WriteString(fmt.Sprintf(" and %d warning(s)", len(result.Warnings)))
		}
		b.WriteString("\nPlease fix the errors above before continuing.\n")
	}

	return b.String()
}

// getEmojiForCategory returns an appropriate emoji for each error category
func getEmojiForCategory(category ValidationErrorCategory) string {
	switch category {
	case CategoryDatabase:
		return "🗄️"
	case CategoryProvider:
		return "🔌"
	case CategoryEnvironment:
		return "⚙️"
	case CategoryFilePath:
		return "📁"
	case CategoryNetwork:
		return "🌐"
	case CategoryPermission:
		return "🔒"
	case CategoryFormat:
		return "📝"
	default:
		return "❌"
	}
}

// indentText adds indentation to each line of text
func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

// Common error constructors for convenience

func NewDatabaseError(summary, details, solution string, err error) *ValidationError {
	return &ValidationError{
		Category: CategoryDatabase,
		Severity: SeverityCritical,
		Summary:  summary,
		Details:  details,
		Impact:   "Plandex server cannot start without a valid database connection.",
		Solution: solution,
		Err:      err,
	}
}

func NewProviderError(summary, details, solution string, relatedVars []string) *ValidationError {
	return &ValidationError{
		Category:    CategoryProvider,
		Severity:    SeverityError,
		Summary:     summary,
		Details:     details,
		Impact:      "Plans cannot be executed without valid provider credentials.",
		Solution:    solution,
		RelatedVars: relatedVars,
	}
}

func NewEnvironmentError(summary, details, solution, example string, relatedVars []string) *ValidationError {
	return &ValidationError{
		Category:    CategoryEnvironment,
		Severity:    SeverityError,
		Summary:     summary,
		Details:     details,
		Impact:      "Environment configuration is invalid or incomplete.",
		Solution:    solution,
		Example:     example,
		RelatedVars: relatedVars,
	}
}

func NewFilePathError(summary, details, solution string, relatedVars []string, err error) *ValidationError {
	return &ValidationError{
		Category:    CategoryFilePath,
		Severity:    SeverityError,
		Summary:     summary,
		Details:     details,
		Impact:      "Required file is missing or inaccessible.",
		Solution:    solution,
		RelatedVars: relatedVars,
		Err:         err,
	}
}

func NewWarning(category ValidationErrorCategory, summary, details, solution string) *ValidationError {
	return &ValidationError{
		Category: category,
		Severity: SeverityWarning,
		Summary:  summary,
		Details:  details,
		Solution: solution,
	}
}
