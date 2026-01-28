package lib

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"plandex-cli/api"
	"plandex-cli/auth"
	"plandex-cli/fs"

	shared "plandex-shared"
)

// =============================================================================
// STARTUP VALIDATION
// =============================================================================
//
// RunStartupValidation performs all synchronous checks that must pass before
// any plan execution begins.  It returns a ValidationResult whose Passed field
// indicates whether execution may proceed.
//
// Checks performed:
//   - Home directory exists and is writable
//   - Auth state file is readable and structurally valid (if present)
//   - Credentials file (if present) is valid JSON
//   - PLANDEX_ENV is one of the recognised values
//   - PLANDEX_TRACE_FILE path (if set) has an accessible parent directory
//   - PLANDEX_DEBUG_LEVEL (if set) is a recognised level
//
// =============================================================================

func RunStartupValidation() *shared.ValidationResult {
	result := shared.NewValidationResult(shared.PhaseSynchronous)

	validateHomeDir(result)
	validateAuthFile(result)
	validateEnvSettings(result)

	// Persist to the global error registry so the journal can surface it.
	if report := result.ToErrorReport(); report != nil {
		shared.StoreError(report)
	}

	return result
}

// validateHomeDir checks that the Plandex home directory exists and is writable.
func validateHomeDir(result *shared.ValidationResult) {
	if fs.HomePlandexDir == "" {
		result.Add(&shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhaseSynchronous,
			Code:     "HOME_DIR_UNSET",
			Message:  "Plandex home directory path is empty",
			Fix:      "Ensure your HOME environment variable is set correctly",
		})
		return
	}

	info, err := os.Stat(fs.HomePlandexDir)
	if err != nil {
		result.Add(&shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhaseSynchronous,
			Code:     "HOME_DIR_MISSING",
			Message:  fmt.Sprintf("Plandex home directory does not exist: %s", fs.HomePlandexDir),
			Fix:      fmt.Sprintf("Run: mkdir -p %s", fs.HomePlandexDir),
			Path:     fs.HomePlandexDir,
		})
		return
	}

	if !info.IsDir() {
		result.Add(&shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhaseSynchronous,
			Code:     "HOME_DIR_NOT_DIR",
			Message:  fmt.Sprintf("Plandex home path exists but is not a directory: %s", fs.HomePlandexDir),
			Fix:      fmt.Sprintf("Remove the file and create a directory: rm %s && mkdir -p %s", fs.HomePlandexDir, fs.HomePlandexDir),
			Path:     fs.HomePlandexDir,
		})
		return
	}

	// Write-check via a temp file.
	testPath := filepath.Join(fs.HomePlandexDir, ".write_check")
	if err := os.WriteFile(testPath, []byte("ok"), 0600); err != nil {
		result.Add(&shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhaseSynchronous,
			Code:     "HOME_DIR_NOT_WRITABLE",
			Message:  fmt.Sprintf("Plandex home directory is not writable: %s", fs.HomePlandexDir),
			Fix:      fmt.Sprintf("Fix permissions: chmod 755 %s", fs.HomePlandexDir),
			Path:     fs.HomePlandexDir,
		})
		return
	}
	os.Remove(testPath)
}

// validateAuthFile checks that the auth and accounts JSON files, if present,
// contain structurally valid JSON.  A missing file is fine (first-time user)
// but a malformed file will cause cryptic errors downstream.
func validateAuthFile(result *shared.ValidationResult) {
	checkJSONFile(result, fs.HomeAuthPath, "auth.json", shared.ClientAuth{})
	checkJSONFile(result, fs.HomeAccountsPath, "accounts.json", []shared.ClientAccount{})
}

// checkJSONFile validates that a file, if it exists, is valid JSON.
func checkJSONFile(result *shared.ValidationResult, path, label string, target interface{}) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return // Not present yet — perfectly fine
		}
		result.Add(&shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhaseSynchronous,
			Code:     "CONFIG_FILE_UNREADABLE",
			Message:  fmt.Sprintf("Cannot read %s: %v", label, err),
			Fix:      fmt.Sprintf("Check permissions on %s", path),
			Path:     path,
		})
		return
	}

	if len(data) == 0 {
		result.Add(&shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityWarn,
			Phase:    shared.PhaseSynchronous,
			Code:     "CONFIG_FILE_EMPTY",
			Message:  fmt.Sprintf("%s exists but is empty — Plandex may need to re-authenticate", label),
			Fix:      "Run: plandex sign-in",
			Path:     path,
		})
		return
	}

	if err := json.Unmarshal(data, &target); err != nil {
		result.Add(&shared.ValidationError{
			Category: shared.CategoryFilesystem,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhaseSynchronous,
			Code:     "CONFIG_FILE_MALFORMED",
			Message:  fmt.Sprintf("%s contains invalid JSON: %v", label, err),
			Fix:      fmt.Sprintf("Delete the file and re-authenticate: rm %s && plandex sign-in", path),
			Path:     path,
		})
	}
}

// validateEnvSettings checks environment variables that control Plandex
// behaviour for recognised / reasonable values.
func validateEnvSettings(result *shared.ValidationResult) {
	// PLANDEX_ENV must be blank, "development", or "production".
	plandexEnv := os.Getenv("PLANDEX_ENV")
	switch plandexEnv {
	case "", "development", "production":
		// ok
	default:
		result.Add(&shared.ValidationError{
			Category: shared.CategoryEnvironment,
			Severity: shared.ValidationSeverityFatal,
			Phase:    shared.PhaseSynchronous,
			Code:     "INVALID_PLANDEX_ENV",
			Message:  fmt.Sprintf("PLANDEX_ENV is set to unrecognised value: %q", plandexEnv),
			Fix:      "Set PLANDEX_ENV to 'development', 'production', or leave it unset",
			EnvVar:   "PLANDEX_ENV",
		})
	}

	// PLANDEX_DEBUG_LEVEL, if set, must be a known level.
	debugLevel := os.Getenv("PLANDEX_DEBUG_LEVEL")
	if debugLevel != "" {
		switch strings.ToLower(debugLevel) {
		case "verbose", "normal", "minimal", "":
			// ok
		default:
			result.Add(&shared.ValidationError{
				Category: shared.CategoryEnvironment,
				Severity: shared.ValidationSeverityWarn,
				Phase:    shared.PhaseSynchronous,
				Code:     "INVALID_DEBUG_LEVEL",
				Message:  fmt.Sprintf("PLANDEX_DEBUG_LEVEL is set to unrecognised value: %q", debugLevel),
				Fix:      "Set PLANDEX_DEBUG_LEVEL to 'verbose', 'normal', 'minimal', or leave it unset",
				EnvVar:   "PLANDEX_DEBUG_LEVEL",
			})
		}
	}

	// PLANDEX_TRACE_FILE, if set, must have a writable parent directory.
	traceFile := os.Getenv("PLANDEX_TRACE_FILE")
	if traceFile != "" {
		dir := filepath.Dir(traceFile)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			result.Add(&shared.ValidationError{
				Category: shared.CategoryFilesystem,
				Severity: shared.ValidationSeverityWarn,
				Phase:    shared.PhaseSynchronous,
				Code:     "TRACE_FILE_BAD_DIR",
				Message:  fmt.Sprintf("PLANDEX_TRACE_FILE parent directory does not exist: %s", dir),
				Fix:      fmt.Sprintf("Create the directory: mkdir -p %s", dir),
				EnvVar:   "PLANDEX_TRACE_FILE",
				Path:     traceFile,
			})
		}
	}
}

// =============================================================================
// DEFERRED PROVIDER VALIDATION
// =============================================================================
//
// RunProviderValidation performs checks that only matter once we know which
// providers the current plan actually requires.  Call this after plan settings
// are loaded but before the first API call.
//
// =============================================================================

// RunProviderValidation checks provider-specific configuration for the given
// set of provider options.  It reports missing API keys, unreadable credential
// files, and known incompatible provider combinations.
func RunProviderValidation(opts shared.ModelProviderOptions, claudeMaxEnabled bool) *shared.ValidationResult {
	result := shared.NewValidationResult(shared.PhaseDeferred)

	// Collect the providers that are actually in play.
	var activeProviders []shared.ModelProvider
	for _, opt := range opts {
		activeProviders = append(activeProviders, opt.Config.Provider)
	}

	// Check for incompatible provider combinations.
	for _, ve := range shared.ValidateProviderCompatibility(activeProviders) {
		result.Add(ve)
	}

	// Check each provider's required credentials.
	for _, opt := range opts {
		cfg := opt.Config
		if cfg.SkipAuth {
			continue
		}
		if cfg.HasClaudeMaxAuth && !claudeMaxEnabled {
			continue
		}

		validateSingleProvider(result, cfg)
	}

	// Persist fatal provider errors to the registry.
	if report := result.ToErrorReport(); report != nil {
		shared.StoreError(report)
	}

	return result
}

// validateSingleProvider checks one provider's auth requirements.
func validateSingleProvider(result *shared.ValidationResult, cfg *shared.ModelProviderConfigSchema) {
	providerLabel := cfg.ToComposite()

	// API key env var
	if cfg.ApiKeyEnvVar != "" {
		val := os.Getenv(cfg.ApiKeyEnvVar)
		if ve := shared.ValidateEnvVarSet(cfg.ApiKeyEnvVar, val, providerLabel); ve != nil {
			result.Add(ve)
		}
	}

	// Extra required auth vars
	for _, extra := range cfg.ExtraAuthVars {
		if !extra.Required {
			continue
		}

		val := os.Getenv(extra.Var)
		if val == "" && extra.Default != "" {
			val = extra.Default
		}

		if val == "" {
			result.Add(&shared.ValidationError{
				Category: shared.CategoryProvider,
				Severity: shared.ValidationSeverityFatal,
				Phase:    shared.PhaseDeferred,
				Code:     "MISSING_EXTRA_AUTH_VAR",
				Message:  fmt.Sprintf("Missing required variable %s for provider %s", extra.Var, providerLabel),
				Fix:      fmt.Sprintf("export %s='your-value'", extra.Var),
				EnvVar:   extra.Var,
				Provider: providerLabel,
			})
			continue
		}

		// If this var is expected to be a file path or JSON, verify readability.
		if extra.MaybeJSONFilePath {
			validateFileOrJSON(result, extra.Var, val, providerLabel)
		}
	}

	// Claude Max credentials check
	if cfg.HasClaudeMaxAuth {
		if auth.Current == nil {
			result.Add(&shared.ValidationError{
				Category: shared.CategoryAuth,
				Severity: shared.ValidationSeverityFatal,
				Phase:    shared.PhaseDeferred,
				Code:     "CLAUDE_MAX_NO_AUTH",
				Message:  "Claude Max subscription requires authentication but no account is connected",
				Fix:      "Run: plandex sign-in",
				Provider: providerLabel,
			})
			return
		}

		credsPath := filepath.Join(fs.HomePlandexDir, auth.Current.UserId, auth.Current.OrgId, "creds.json")
		if _, err := os.Stat(credsPath); os.IsNotExist(err) {
			// Not necessarily fatal — user may not have connected Claude Max yet.
			// But if the provider is required, the credential check downstream will catch it.
			result.Add(&shared.ValidationError{
				Category: shared.CategoryAuth,
				Severity: shared.ValidationSeverityWarn,
				Phase:    shared.PhaseDeferred,
				Code:     "CLAUDE_MAX_CREDS_MISSING",
				Message:  "Claude Max credentials file not found — subscription may not be connected",
				Fix:      "Connect your Claude Max subscription via plandex sign-in",
				Provider: providerLabel,
				Path:     credsPath,
			})
		}
	}

	// AWS Bedrock: if PLANDEX_AWS_PROFILE is set, the profile must reference
	// a reachable credentials source.
	if cfg.HasAWSAuth {
		profile := os.Getenv("PLANDEX_AWS_PROFILE")
		if profile != "" {
			awsDir := filepath.Join(os.Getenv("HOME"), ".aws")
			credFile := filepath.Join(awsDir, "credentials")
			configFile := filepath.Join(awsDir, "config")

			hasCreds := fileExists(credFile)
			hasConfig := fileExists(configFile)

			if !hasCreds && !hasConfig {
				result.Add(&shared.ValidationError{
					Category: shared.CategoryProvider,
					Severity: shared.ValidationSeverityFatal,
					Phase:    shared.PhaseDeferred,
					Code:     "AWS_PROFILE_NO_SOURCE",
					Message:  fmt.Sprintf("PLANDEX_AWS_PROFILE is set to %q but neither ~/.aws/credentials nor ~/.aws/config exists", profile),
					Fix:      "Create ~/.aws/credentials or ~/.aws/config with the required profile, or unset PLANDEX_AWS_PROFILE",
					EnvVar:   "PLANDEX_AWS_PROFILE",
					Provider: providerLabel,
				})
			}
		}
	}
}

// validateFileOrJSON checks that a value which may be a file path, base64 JSON,
// or inline JSON is actually usable.
func validateFileOrJSON(result *shared.ValidationResult, envVar, value, providerLabel string) {
	trimmed := strings.TrimSpace(value)

	// Inline JSON is always fine.
	if strings.HasPrefix(trimmed, "{") {
		return
	}

	// If it looks like a file path (not base64), verify it exists.
	// Base64 strings rarely start with "/" or "." so this heuristic is safe.
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "~") {
		expanded := trimmed
		if strings.HasPrefix(expanded, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				expanded = filepath.Join(home, expanded[1:])
			}
		}
		if !fileExists(expanded) {
			result.Add(&shared.ValidationError{
				Category: shared.CategoryFilesystem,
				Severity: shared.ValidationSeverityFatal,
				Phase:    shared.PhaseDeferred,
				Code:     "CREDENTIAL_FILE_NOT_FOUND",
				Message:  fmt.Sprintf("Credential file referenced by %s does not exist: %s", envVar, expanded),
				Fix:      fmt.Sprintf("Create the file at %s or set %s to valid inline JSON", expanded, envVar),
				EnvVar:   envVar,
				Path:     expanded,
				Provider: providerLabel,
			})
		}
	}
	// Otherwise assume it's base64 — actual decoding is handled at resolve time.
}

// =============================================================================
// MUST ENTRY-POINT FOR DEFERRED VALIDATION
// =============================================================================
//
// MustRunDeferredValidation is called by execution commands (tell, build,
// continue) right before MustVerifyAuthVars.  It fetches the current plan's
// model settings, runs provider validation, and exits with a clear diagnostic
// if any fatal errors are found.
//
// =============================================================================

// MustRunDeferredValidation runs deferred (provider-scoped) validation for the
// current plan.  It exits with a formatted error report on fatal failures.
func MustRunDeferredValidation() {
	if CurrentPlanId == "" {
		// No plan loaded yet — nothing to validate.
		return
	}

	planSettings, apiErr := api.Client.GetSettings(CurrentPlanId, CurrentBranch)
	if apiErr != nil {
		// If we can't fetch settings, downstream MustVerifyAuthVars will
		// surface the same error with its own handling.  Don't duplicate.
		return
	}

	orgUserConfig := MustGetOrgUserConfig()

	opts := planSettings.GetModelProviderOptions()
	claudeMaxEnabled := orgUserConfig != nil && orgUserConfig.UseClaudeSubscription

	result := RunProviderValidation(opts, claudeMaxEnabled)

	// Warnings are informational only; print but do not block.
	if len(result.Warnings) > 0 && len(result.FatalErrors) == 0 {
		fmt.Fprint(os.Stderr, "\n")
		fmt.Fprint(os.Stderr, result.FormatCLI())
		return
	}

	// Fatal errors stop execution.
	if !result.Passed {
		fmt.Fprint(os.Stderr, "\n")
		fmt.Fprint(os.Stderr, result.FormatCLI())

		if report := result.ToErrorReport(); report != nil {
			shared.StoreError(report)
			if shared.GlobalErrorRegistry != nil {
				shared.GlobalErrorRegistry.Persist()
			}
		}

		os.Exit(1)
	}
}
