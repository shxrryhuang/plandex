package validation

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ValidateEnvironment validates general environment configuration
func ValidateEnvironment() *ValidationResult {
	result := &ValidationResult{}

	// Check PORT if set
	if port := os.Getenv("PORT"); port != "" {
		if err := validatePort(port); err != nil {
			result.AddError(err)
		}
	}

	// Check GOENV if set
	if goenv := os.Getenv("GOENV"); goenv != "" {
		if err := validateGOENV(goenv); err != nil {
			result.AddError(err)
		}
	}

	// Check debug configuration
	if debugResult := validateDebugConfig(); !debugResult.IsValid() {
		result.Merge(debugResult)
	}

	// Check for conflicting environment variables
	if conflicts := checkConflictingVars(); len(conflicts) > 0 {
		for _, conflict := range conflicts {
			result.AddError(conflict)
		}
	}

	return result
}

// ValidateLiteLLMProxy validates that the LiteLLM proxy is accessible
func ValidateLiteLLMProxy(ctx context.Context) *ValidationResult {
	result := &ValidationResult{}

	// Test if port 4000 is available (not in use)
	// This check should be done before starting LiteLLM
	conn, err := net.DialTimeout("tcp", "localhost:4000", 1*time.Second)
	if err == nil {
		// Port is already in use
		conn.Close()
		result.AddError(&ValidationError{
			Category: CategoryNetwork,
			Severity: SeverityWarning,
			Summary:  "Port 4000 is already in use",
			Details:  "LiteLLM proxy port (4000) is already occupied by another process.",
			Impact:   "May cause LiteLLM proxy startup failure.",
			Solution: `Check what's using port 4000:
  lsof -i :4000  # macOS/Linux
  netstat -ano | findstr :4000  # Windows

If it's a stale LiteLLM process, kill it:
  kill <pid>

Or configure Plandex to use a different port if supported.`,
		})
		return result
	}

	return result
}

// ValidateLiteLLMProxyHealth checks if LiteLLM proxy is running and healthy
func ValidateLiteLLMProxyHealth(ctx context.Context) *ValidationResult {
	result := &ValidationResult{}

	baseURL := "http://localhost:4000"
	healthURL := baseURL + "/health"

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		result.AddError(&ValidationError{
			Category: CategoryNetwork,
			Severity: SeverityCritical,
			Summary:  "Failed to create LiteLLM health check request",
			Details:  fmt.Sprintf("Error: %v", err),
			Impact:   "Cannot verify LiteLLM proxy status.",
			Solution: "This is an internal error. Check system resources and network configuration.",
			Err:      err,
		})
		return result
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		result.AddError(&ValidationError{
			Category: CategoryNetwork,
			Severity: SeverityCritical,
			Summary:  "LiteLLM proxy is not responding",
			Details:  fmt.Sprintf("Health check failed: %v", err),
			Impact:   "Plandex cannot communicate with AI model providers without LiteLLM proxy.",
			Solution: `Troubleshoot LiteLLM proxy:
  1. Check if the proxy process is running:
     ps aux | grep litellm
  2. Check proxy logs for errors
  3. Verify no firewall is blocking localhost:4000
  4. Try restarting the Plandex server`,
			Err: err,
		})
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.AddError(&ValidationError{
			Category: CategoryNetwork,
			Severity: SeverityCritical,
			Summary:  "LiteLLM proxy health check failed",
			Details:  fmt.Sprintf("Health endpoint returned status %d", resp.StatusCode),
			Impact:   "LiteLLM proxy may not be functioning correctly.",
			Solution: `Check LiteLLM proxy status:
  1. Review proxy logs for errors
  2. Restart the Plandex server
  3. Check system resources (memory, CPU)
  4. Verify LiteLLM configuration`,
		})
		return result
	}

	return result
}

// validatePort validates the PORT environment variable
func validatePort(port string) *ValidationError {
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return &ValidationError{
			Category:    CategoryEnvironment,
			Severity:    SeverityError,
			Summary:     "Invalid PORT format",
			Details:     fmt.Sprintf("PORT must be a number, got: %s", port),
			Impact:      "Server will fail to start with invalid port number.",
			Solution:    "Set PORT to a valid number between 1 and 65535.",
			Example:     `export PORT="8099"  # default`,
			RelatedVars: []string{"PORT"},
			Err:         err,
		}
	}

	if portNum < 1 || portNum > 65535 {
		return &ValidationError{
			Category:    CategoryEnvironment,
			Severity:    SeverityError,
			Summary:     "PORT out of valid range",
			Details:     fmt.Sprintf("PORT must be between 1 and 65535, got: %d", portNum),
			Impact:      "Server cannot bind to invalid port number.",
			Solution:    "Choose a port number between 1 and 65535. Ports below 1024 require root privileges.",
			Example:     `export PORT="8099"  # default\nexport PORT="3000"  # alternative`,
			RelatedVars: []string{"PORT"},
		}
	}

	if portNum < 1024 {
		return &ValidationError{
			Category:    CategoryEnvironment,
			Severity:    SeverityWarning,
			Summary:     "PORT requires elevated privileges",
			Details:     fmt.Sprintf("Port %d is below 1024 and requires root/administrator privileges", portNum),
			Impact:      "Server may fail to start without appropriate permissions.",
			Solution:    "Either run with elevated privileges or use a port >= 1024.",
			Example:     `export PORT="8099"  # recommended for non-root`,
			RelatedVars: []string{"PORT"},
		}
	}

	return nil
}

// validateGOENV validates the GOENV environment variable
func validateGOENV(goenv string) *ValidationError {
	validValues := []string{"development", "production", "test"}
	goenv = strings.ToLower(goenv)

	for _, valid := range validValues {
		if goenv == valid {
			return nil
		}
	}

	return &ValidationError{
		Category:    CategoryEnvironment,
		Severity:    SeverityWarning,
		Summary:     "Invalid GOENV value",
		Details:     fmt.Sprintf("GOENV is set to '%s', expected one of: %s", goenv, strings.Join(validValues, ", ")),
		Impact:      "May cause unexpected behavior or configuration issues.",
		Solution:    "Set GOENV to one of the recognized values or leave it unset (defaults to production).",
		Example:     `export GOENV="production"  # for production use\nexport GOENV="development"  # for development`,
		RelatedVars: []string{"GOENV"},
	}
}

// validateDebugConfig validates debug-related environment variables
func validateDebugConfig() *ValidationResult {
	result := &ValidationResult{}

	debugEnabled := os.Getenv("PLANDEX_DEBUG")
	debugLevel := os.Getenv("PLANDEX_DEBUG_LEVEL")
	traceFile := os.Getenv("PLANDEX_TRACE_FILE")

	// Validate debug level if set
	if debugLevel != "" {
		validLevels := []string{"error", "warn", "info", "debug", "trace"}
		valid := false
		for _, level := range validLevels {
			if strings.ToLower(debugLevel) == level {
				valid = true
				break
			}
		}

		if !valid {
			result.AddError(&ValidationError{
				Category:    CategoryEnvironment,
				Severity:    SeverityWarning,
				Summary:     "Invalid PLANDEX_DEBUG_LEVEL",
				Details:     fmt.Sprintf("Debug level '%s' is not recognized", debugLevel),
				Impact:      "Debug output may not work as expected.",
				Solution:    fmt.Sprintf("Set PLANDEX_DEBUG_LEVEL to one of: %s", strings.Join(validLevels, ", ")),
				Example:     `export PLANDEX_DEBUG_LEVEL="debug"`,
				RelatedVars: []string{"PLANDEX_DEBUG_LEVEL"},
			})
		}
	}

	// Validate trace file if set
	if traceFile != "" {
		// Check if the directory exists and is writable
		dir := traceFile
		if idx := strings.LastIndex(traceFile, "/"); idx != -1 {
			dir = traceFile[:idx]
		}

		if dir != "" {
			if stat, err := os.Stat(dir); err != nil {
				if os.IsNotExist(err) {
					result.AddError(&ValidationError{
						Category:    CategoryFilePath,
						Severity:    SeverityWarning,
						Summary:     "PLANDEX_TRACE_FILE directory does not exist",
						Details:     fmt.Sprintf("Directory '%s' does not exist", dir),
						Impact:      "Trace logging will fail.",
						Solution:    fmt.Sprintf("Create the directory: mkdir -p %s", dir),
						RelatedVars: []string{"PLANDEX_TRACE_FILE"},
						Err:         err,
					})
				}
			} else if !stat.IsDir() {
				result.AddError(&ValidationError{
					Category:    CategoryFilePath,
					Severity:    SeverityWarning,
					Summary:     "PLANDEX_TRACE_FILE parent is not a directory",
					Details:     fmt.Sprintf("'%s' exists but is not a directory", dir),
					Impact:      "Cannot create trace file.",
					Solution:    "Specify a valid directory path for the trace file.",
					RelatedVars: []string{"PLANDEX_TRACE_FILE"},
				})
			}
		}
	}

	// Warn if debug level is set but debug is not enabled
	if debugLevel != "" && debugEnabled == "" {
		result.AddError(&ValidationError{
			Category:    CategoryEnvironment,
			Severity:    SeverityWarning,
			Summary:     "Debug level set but debug not enabled",
			Details:     "PLANDEX_DEBUG_LEVEL is set but PLANDEX_DEBUG is not enabled",
			Impact:      "Debug level setting will have no effect.",
			Solution:    "Enable debug mode: export PLANDEX_DEBUG=1",
			Example:     `export PLANDEX_DEBUG=1\nexport PLANDEX_DEBUG_LEVEL="debug"`,
			RelatedVars: []string{"PLANDEX_DEBUG", "PLANDEX_DEBUG_LEVEL"},
		})
	}

	return result
}

// checkConflictingVars checks for conflicting environment variable configurations
func checkConflictingVars() []*ValidationError {
	var errors []*ValidationError

	// Check for conflicting database configurations
	hasDBURL := os.Getenv("DATABASE_URL") != ""
	hasDBVars := os.Getenv("DB_HOST") != "" || os.Getenv("DB_PORT") != "" ||
		os.Getenv("DB_USER") != "" || os.Getenv("DB_PASSWORD") != "" ||
		os.Getenv("DB_NAME") != ""

	if hasDBURL && hasDBVars {
		errors = append(errors, &ValidationError{
			Category: CategoryEnvironment,
			Severity: SeverityWarning,
			Summary:  "Both DATABASE_URL and DB_* variables are set",
			Details:  "You have both DATABASE_URL and individual DB_* variables configured. DATABASE_URL will take precedence.",
			Impact:   "DB_* variables will be ignored, which may be confusing.",
			Solution: "Use either DATABASE_URL or DB_* variables, but not both. Remove the unused configuration.",
			Example: `# Option 1: Use DATABASE_URL only
export DATABASE_URL="postgres://user:pass@host:5432/db"
# Remove: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME

# Option 2: Use DB_* variables only
export DB_HOST="localhost"
export DB_PORT="5432"
# etc.
# Remove: DATABASE_URL`,
			RelatedVars: []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME"},
		})
	}

	// Check for AWS Bedrock configuration issues
	hasAWSProfile := os.Getenv("PLANDEX_AWS_PROFILE") != ""
	hasAWSKeys := os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != ""

	if hasAWSProfile && hasAWSKeys {
		errors = append(errors, &ValidationError{
			Category: CategoryEnvironment,
			Severity: SeverityWarning,
			Summary:  "Multiple AWS credential sources configured",
			Details:  "Both PLANDEX_AWS_PROFILE and AWS_* environment variables are set.",
			Impact:   "Credential resolution may be ambiguous. Profile-based credentials will be attempted first.",
			Solution: "Use either PLANDEX_AWS_PROFILE or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY, but not both.",
			Example: `# Option 1: Use AWS profile
export PLANDEX_AWS_PROFILE="default"

# Option 2: Use explicit credentials
export AWS_ACCESS_KEY_ID="AKIA..."
export AWS_SECRET_ACCESS_KEY="..."
export AWS_REGION="us-east-1"`,
			RelatedVars: []string{"PLANDEX_AWS_PROFILE", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"},
		})
	}

	return errors
}
