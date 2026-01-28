package validation

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFileValidator validates configuration files (.env, etc.)
type ConfigFileValidator struct {
	filePath string
	required bool
}

// ValidateConfigFile checks if a .env or config file exists and is valid
func ValidateConfigFile(filePath string, required bool) *ValidationResult {
	result := &ValidationResult{}
	validator := &ConfigFileValidator{
		filePath: filePath,
		required: required,
	}

	return validator.validate(result)
}

func (v *ConfigFileValidator) validate(result *ValidationResult) *ValidationResult {
	// Check if file exists
	if _, err := os.Stat(v.filePath); os.IsNotExist(err) {
		if v.required {
			result.AddError(&ValidationError{
				Category: CategoryFilePath,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("Configuration file not found: %s", v.filePath),
				Details:  fmt.Sprintf("The required configuration file '%s' does not exist.", v.filePath),
				Impact:   "Configuration will not be loaded from file.",
				Solution: fmt.Sprintf(`Create the configuration file:
  1. Create the file:
     touch %s

  2. Add your configuration:
     # Example .env file
     DATABASE_URL=postgresql://user:pass@localhost:5432/plandex
     OPENAI_API_KEY=sk-...
     PORT=8080

  3. Restart the server`, v.filePath),
				Example: `# .env file example
DATABASE_URL=postgresql://user:pass@localhost:5432/plandex
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
PORT=8080
GOENV=production`,
				RelatedVars: []string{filepath.Base(v.filePath)},
				Err:         err,
			})
		} else {
			result.AddError(&ValidationError{
				Category: CategoryFilePath,
				Severity: SeverityWarning,
				Summary:  fmt.Sprintf("Optional configuration file not found: %s", v.filePath),
				Details:  fmt.Sprintf("The optional configuration file '%s' does not exist.", v.filePath),
				Impact:   "Using environment variables instead of file-based configuration.",
				Solution: fmt.Sprintf("If you want to use file-based configuration, create %s", v.filePath),
			})
		}
		return result
	}

	// Check if file is readable
	file, err := os.Open(v.filePath)
	if err != nil {
		result.AddError(&ValidationError{
			Category: CategoryPermission,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("Cannot read configuration file: %s", v.filePath),
			Details:  fmt.Sprintf("File exists but cannot be read: %v", err),
			Impact:   "Configuration cannot be loaded from file.",
			Solution: fmt.Sprintf(`Fix file permissions:
  chmod 644 %s

Or check file ownership:
  ls -la %s`, v.filePath, v.filePath),
			RelatedVars: []string{filepath.Base(v.filePath)},
			Err:         err,
		})
		return result
	}
	defer file.Close()

	// Parse and validate the file
	scanner := bufio.NewScanner(file)
	lineNum := 0
	emptyLines := 0
	commentLines := 0
	validLines := 0
	invalidLines := 0
	var issues []string

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			emptyLines++
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "#") {
			commentLines++
			continue
		}

		// Check for valid KEY=VALUE format
		if !strings.Contains(line, "=") {
			invalidLines++
			issues = append(issues, fmt.Sprintf("Line %d: Missing '=' separator: %s", lineNum, line))
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Validate key format
		if key == "" {
			invalidLines++
			issues = append(issues, fmt.Sprintf("Line %d: Empty key", lineNum))
			continue
		}

		// Warn about empty values
		if value == "" {
			issues = append(issues, fmt.Sprintf("Line %d: Empty value for key '%s'", lineNum, key))
		}

		// Check for spaces in key (not allowed)
		if strings.Contains(key, " ") {
			invalidLines++
			issues = append(issues, fmt.Sprintf("Line %d: Key contains spaces: '%s'", lineNum, key))
			continue
		}

		// Check for quotes in value (common mistake)
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			issues = append(issues, fmt.Sprintf("Line %d: Value for '%s' is quoted (may be intentional)", lineNum, key))
		}

		validLines++
	}

	if err := scanner.Err(); err != nil {
		result.AddError(&ValidationError{
			Category: CategoryFormat,
			Severity: SeverityError,
			Summary:  "Error reading configuration file",
			Details:  fmt.Sprintf("Scanner error: %v", err),
			Impact:   "Configuration may be incomplete.",
			Solution: "Check file format and encoding (should be UTF-8)",
			Err:      err,
		})
		return result
	}

	// Report invalid lines
	if invalidLines > 0 {
		result.AddError(&ValidationError{
			Category: CategoryFormat,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("Configuration file has %d invalid line(s)", invalidLines),
			Details:  fmt.Sprintf("Found %d lines with format errors:\n%s", invalidLines, strings.Join(issues, "\n")),
			Impact:   "Invalid configuration entries will be ignored.",
			Solution: `Fix the format errors. Valid format is:
  KEY=VALUE

Common issues:
  - Missing = separator
  - Spaces in key names
  - Empty keys`,
			Example: `# Valid format:
DATABASE_URL=postgresql://localhost:5432/plandex
OPENAI_API_KEY=sk-...
PORT=8080

# Invalid format:
DATABASE URL=postgresql://...  # No spaces in keys
=some_value                     # Empty key
DATABASE_URL                    # Missing = and value`,
		})
	}

	// Warn about issues
	if len(issues) > invalidLines && invalidLines == 0 {
		// Only warnings (empty values, quoted values, etc.)
		result.AddError(&ValidationError{
			Category: CategoryFormat,
			Severity: SeverityWarning,
			Summary:  fmt.Sprintf("Configuration file has %d potential issue(s)", len(issues)),
			Details:  fmt.Sprintf("Issues found:\n%s", strings.Join(issues, "\n")),
			Impact:   "Configuration may not work as expected.",
			Solution: "Review and verify the flagged entries.",
		})
	}

	// Warn if file is mostly empty
	if validLines == 0 && commentLines+emptyLines > 0 {
		result.AddError(&ValidationError{
			Category: CategoryFormat,
			Severity: SeverityWarning,
			Summary:  "Configuration file is empty or contains only comments",
			Details:  fmt.Sprintf("File has %d lines but 0 valid configuration entries", lineNum),
			Impact:   "No configuration will be loaded from file.",
			Solution: "Add configuration entries to the file or remove it if not needed.",
		})
	}

	return result
}

// LoadEnvFile loads environment variables from a .env file
func LoadEnvFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open env file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		if !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, "\"'")

		// Set environment variable if not already set
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

// ValidateEnvFileAndLoad validates and loads a .env file
func ValidateEnvFileAndLoad(filePath string) *ValidationResult {
	result := ValidateConfigFile(filePath, false)

	if !result.HasErrors() {
		if err := LoadEnvFile(filePath); err != nil {
			result.AddError(&ValidationError{
				Category: CategoryFormat,
				Severity: SeverityError,
				Summary:  "Failed to load configuration file",
				Details:  fmt.Sprintf("Error loading %s: %v", filePath, err),
				Impact:   "Configuration not applied.",
				Solution: "Check file format and permissions.",
				Err:      err,
			})
		}
	}

	return result
}

// FindConfigFiles searches for common configuration files
func FindConfigFiles() []string {
	var configFiles []string
	commonFiles := []string{
		".env",
		".env.local",
		".env.development",
		".env.production",
		"plandex.env",
		"config.env",
	}

	for _, file := range commonFiles {
		if _, err := os.Stat(file); err == nil {
			configFiles = append(configFiles, file)
		}
	}

	return configFiles
}

// ValidateAllConfigFiles validates all found configuration files
func ValidateAllConfigFiles() *ValidationResult {
	result := &ValidationResult{}
	files := FindConfigFiles()

	if len(files) == 0 {
		result.AddError(&ValidationError{
			Category: CategoryFilePath,
			Severity: SeverityWarning,
			Summary:  "No configuration files found",
			Details:  "Looked for .env, .env.local, .env.production, etc.",
			Impact:   "Using only environment variables for configuration.",
			Solution: `Create a .env file if you want file-based configuration:
  echo "DATABASE_URL=postgresql://localhost:5432/plandex" > .env
  echo "OPENAI_API_KEY=sk-..." >> .env`,
		})
		return result
	}

	for _, file := range files {
		fileResult := ValidateConfigFile(file, false)
		result.Merge(fileResult)
	}

	return result
}
