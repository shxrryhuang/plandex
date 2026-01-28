package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"plandex-server/model"
	"plandex-server/routes"
	"plandex-server/setup"
	"plandex-shared/features"
	"plandex-shared/validation"
	"time"

	"github.com/gorilla/mux"
)

// mainSafe is the new entry point with optional validation
// This preserves the original behavior when validation is disabled
func mainSafe() {
	// Configure the default logger to include milliseconds in timestamps
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	log.Println("Starting Plandex server...")

	// Check if validation is enabled
	if features.IsValidationEnabled() {
		log.Println("⚙️  Validation system: ENABLED")
		logValidationSettings()
	} else {
		log.Println("⚙️  Validation system: DISABLED (using original startup flow)")
	}

	// Run optional startup validation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := safeValidateStartupConfiguration(ctx); err != nil {
		log.Fatal("Startup validation failed: ", err)
	}

	routes.RegisterHandlePlandex(func(router *mux.Router, path string, isStreaming bool, handler routes.PlandexHandler) *mux.Route {
		return router.HandleFunc(path, handler)
	})

	// Start LiteLLM proxy with conditional error handling
	log.Println("Starting LiteLLM proxy...")
	err := model.EnsureLiteLLM(2)
	if err != nil {
		if features.IsValidationEnabled() {
			// Use enhanced error formatting
			log.Printf("❌ Failed to start LiteLLM proxy: %v\n", err)
			log.Fatal(formatLiteLLMErrorSafe(err))
		} else {
			// Use original error handling
			panic(fmt.Sprintf("Failed to start LiteLLM proxy: %v", err))
		}
	}
	log.Println("✅ LiteLLM proxy started successfully")

	setup.RegisterShutdownHook(func() {
		model.ShutdownLiteLLMServer()
	})

	// Initialize error handling infrastructure
	model.InitGlobalCircuitBreaker()
	log.Println("Initialized global circuit breaker")

	model.InitGlobalStreamRecoveryManager()
	log.Println("Initialized global stream recovery manager")

	model.InitGlobalHealthCheckManager()
	log.Println("Initialized global health check manager")

	model.InitGlobalDegradationManager()
	log.Println("Initialized global degradation manager")

	model.InitGlobalDeadLetterQueue()
	log.Println("Initialized global dead letter queue")

	// Register cleanup for error handling components
	setup.RegisterShutdownHook(func() {
		if model.GlobalCircuitBreaker != nil {
			log.Println("Circuit breaker final metrics:", model.GlobalCircuitBreaker.GetMetrics())
		}
		if model.GlobalStreamRecoveryManager != nil {
			log.Println("Stream recovery final stats:", model.GlobalStreamRecoveryManager.GetStats())
		}
		if model.GlobalHealthCheckManager != nil {
			log.Println("Health check final metrics:", model.GlobalHealthCheckManager.GetMetrics())
		}
		if model.GlobalDegradationManager != nil {
			log.Println("Degradation final metrics:", model.GlobalDegradationManager.GetMetrics())
		}
		if model.GlobalDeadLetterQueue != nil {
			log.Println("Dead letter queue final stats:", model.GlobalDeadLetterQueue.GetStats())
		}
	})

	r := mux.NewRouter()
	routes.AddHealthRoutes(r)
	routes.AddApiRoutes(r)
	routes.AddProxyableApiRoutes(r)
	setup.MustLoadIp()
	setup.MustInitDb()
	setup.StartServer(r, nil, nil)
	os.Exit(0)
}

// safeValidateStartupConfiguration runs validation if enabled, otherwise skips
func safeValidateStartupConfiguration(ctx context.Context) error {
	// Use safe validation wrapper
	err := validation.SafeValidateStartup(ctx)

	if err != nil {
		// Validation is enabled and found errors
		fmt.Fprintln(os.Stderr, "\n"+err.Error())
		return fmt.Errorf("configuration validation failed")
	}

	if features.IsStartupValidationEnabled() {
		log.Println("✅ Startup validation passed")
	}

	return nil
}

// formatLiteLLMErrorSafe provides helpful context for LiteLLM failures (safe version)
func formatLiteLLMErrorSafe(err error) string {
	if !features.IsValidationEnabled() {
		// Original behavior - just return the error
		return err.Error()
	}

	// Use enhanced error formatting
	verr := &validation.ValidationError{
		Category: validation.CategoryNetwork,
		Severity: validation.SeverityCritical,
		Summary:  "Failed to start LiteLLM proxy",
		Details:  fmt.Sprintf("LiteLLM proxy failed to start: %v", err),
		Impact:   "Plandex cannot communicate with AI model providers without LiteLLM proxy.",
		Solution: `Troubleshoot LiteLLM startup:
  1. Check if port 4000 is already in use:
     lsof -i :4000  # macOS/Linux
     netstat -ano | findstr :4000  # Windows

  2. Check system resources (memory, disk space):
     df -h  # disk space
     free -h  # memory (Linux)

  3. Review LiteLLM logs for detailed error messages

  4. Ensure Python and LiteLLM dependencies are installed:
     pip list | grep litellm

  5. Try running LiteLLM manually to see errors:
     litellm --port 4000`,
		Err: err,
	}

	return validation.FormatError(verr, features.IsVerboseValidationEnabled())
}

// logValidationSettings logs the current validation configuration
func logValidationSettings() {
	settings := []struct {
		name    string
		enabled bool
	}{
		{"Startup validation", features.IsStartupValidationEnabled()},
		{"Execution validation", features.IsExecutionValidationEnabled()},
		{"Verbose errors", features.IsVerboseValidationEnabled()},
		{"Strict mode", features.IsStrictValidationEnabled()},
		{"File checks", features.IsFileChecksEnabled()},
	}

	for _, s := range settings {
		status := "disabled"
		icon := "○"
		if s.enabled {
			status = "enabled"
			icon = "●"
		}
		log.Printf("  %s %s: %s", icon, s.name, status)
	}
}
