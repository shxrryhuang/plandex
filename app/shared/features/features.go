package features

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Feature flags control optional functionality in Plandex
// This allows new features to be enabled/disabled without code changes

// FeatureFlag represents a feature that can be toggled
type FeatureFlag string

const (
	// ValidationSystem enables the new configuration validation system
	ValidationSystem FeatureFlag = "validation_system"

	// ValidationStartup enables startup validation checks
	ValidationStartup FeatureFlag = "validation_startup"

	// ValidationExecution enables pre-execution validation checks
	ValidationExecution FeatureFlag = "validation_execution"

	// ValidationVerbose enables verbose validation error messages
	ValidationVerbose FeatureFlag = "validation_verbose"

	// ValidationStrict enables strict validation mode (warnings become errors)
	ValidationStrict FeatureFlag = "validation_strict"

	// ValidationFileChecks enables file access validation (slower but thorough)
	ValidationFileChecks FeatureFlag = "validation_file_checks"
)

// FeatureManager manages feature flags
type FeatureManager struct {
	mu       sync.RWMutex
	flags    map[FeatureFlag]bool
	defaults map[FeatureFlag]bool
}

var (
	// Global feature manager instance
	globalManager *FeatureManager
	once          sync.Once
)

// GetManager returns the global feature manager instance
func GetManager() *FeatureManager {
	once.Do(func() {
		globalManager = NewFeatureManager()
		globalManager.LoadFromEnvironment()
	})
	return globalManager
}

// NewFeatureManager creates a new feature manager with default values
func NewFeatureManager() *FeatureManager {
	fm := &FeatureManager{
		flags:    make(map[FeatureFlag]bool),
		defaults: make(map[FeatureFlag]bool),
	}

	// Set default values for each feature
	fm.defaults[ValidationSystem] = false       // Disabled by default for safety
	fm.defaults[ValidationStartup] = false      // Disabled by default
	fm.defaults[ValidationExecution] = false    // Disabled by default
	fm.defaults[ValidationVerbose] = true       // Verbose messages when enabled
	fm.defaults[ValidationStrict] = false       // Warnings don't block by default
	fm.defaults[ValidationFileChecks] = false   // Fast validation by default

	// Initialize flags with defaults
	for flag, value := range fm.defaults {
		fm.flags[flag] = value
	}

	return fm
}

// LoadFromEnvironment loads feature flags from environment variables
func (fm *FeatureManager) LoadFromEnvironment() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Master switch: PLANDEX_ENABLE_VALIDATION
	if masterSwitch := os.Getenv("PLANDEX_ENABLE_VALIDATION"); masterSwitch != "" {
		enabled := parseBool(masterSwitch)
		fm.flags[ValidationSystem] = enabled
		fm.flags[ValidationStartup] = enabled
		fm.flags[ValidationExecution] = enabled
	}

	// Individual feature flags
	envFlags := map[string]FeatureFlag{
		"PLANDEX_VALIDATION_SYSTEM":      ValidationSystem,
		"PLANDEX_VALIDATION_STARTUP":     ValidationStartup,
		"PLANDEX_VALIDATION_EXECUTION":   ValidationExecution,
		"PLANDEX_VALIDATION_VERBOSE":     ValidationVerbose,
		"PLANDEX_VALIDATION_STRICT":      ValidationStrict,
		"PLANDEX_VALIDATION_FILE_CHECKS": ValidationFileChecks,
	}

	for envVar, flag := range envFlags {
		if value := os.Getenv(envVar); value != "" {
			fm.flags[flag] = parseBool(value)
		}
	}
}

// IsEnabled checks if a feature is enabled
func (fm *FeatureManager) IsEnabled(flag FeatureFlag) bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	if enabled, exists := fm.flags[flag]; exists {
		return enabled
	}
	return fm.defaults[flag]
}

// Enable enables a feature
func (fm *FeatureManager) Enable(flag FeatureFlag) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.flags[flag] = true
}

// Disable disables a feature
func (fm *FeatureManager) Disable(flag FeatureFlag) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.flags[flag] = false
}

// SetFlag sets a feature flag to a specific value
func (fm *FeatureManager) SetFlag(flag FeatureFlag, enabled bool) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.flags[flag] = enabled
}

// GetAll returns all feature flags and their states
func (fm *FeatureManager) GetAll() map[FeatureFlag]bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	result := make(map[FeatureFlag]bool)
	for flag, enabled := range fm.flags {
		result[flag] = enabled
	}
	return result
}

// ResetToDefaults resets all flags to their default values
func (fm *FeatureManager) ResetToDefaults() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for flag, defaultValue := range fm.defaults {
		fm.flags[flag] = defaultValue
	}
}

// parseBool parses a boolean value from string
// Accepts: true, false, 1, 0, yes, no, on, off (case-insensitive)
func parseBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "true", "1", "yes", "on", "enabled", "enable":
		return true
	case "false", "0", "no", "off", "disabled", "disable":
		return false
	default:
		// Try standard strconv parsing
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
		return false
	}
}

// Convenience functions for checking specific features

// IsValidationEnabled returns true if the validation system is enabled
func IsValidationEnabled() bool {
	return GetManager().IsEnabled(ValidationSystem)
}

// IsStartupValidationEnabled returns true if startup validation is enabled
func IsStartupValidationEnabled() bool {
	return GetManager().IsEnabled(ValidationSystem) && GetManager().IsEnabled(ValidationStartup)
}

// IsExecutionValidationEnabled returns true if execution validation is enabled
func IsExecutionValidationEnabled() bool {
	return GetManager().IsEnabled(ValidationSystem) && GetManager().IsEnabled(ValidationExecution)
}

// IsVerboseValidationEnabled returns true if verbose validation is enabled
func IsVerboseValidationEnabled() bool {
	return GetManager().IsEnabled(ValidationVerbose)
}

// IsStrictValidationEnabled returns true if strict validation is enabled
func IsStrictValidationEnabled() bool {
	return GetManager().IsEnabled(ValidationStrict)
}

// IsFileChecksEnabled returns true if file access checks are enabled
func IsFileChecksEnabled() bool {
	return GetManager().IsEnabled(ValidationFileChecks)
}
