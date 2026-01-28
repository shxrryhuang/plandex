package validation

import (
	"context"
	"os"
	"testing"
	"time"
)

// Test config file validation
func TestConfigFileValidation(t *testing.T) {
	t.Run("Missing required file", func(t *testing.T) {
		result := ValidateConfigFile("/nonexistent/.env", true)
		if !result.HasErrors() {
			t.Error("Should have error for missing required file")
		}
	})

	t.Run("Missing optional file", func(t *testing.T) {
		result := ValidateConfigFile("/nonexistent/.env", false)
		if !result.HasWarnings() {
			t.Error("Should have warning for missing optional file")
		}
	})

	t.Run("Valid config file", func(t *testing.T) {
		// Create temporary test file
		tmpfile, err := os.CreateTemp("", "test.env")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		// Write valid content
		content := `# Comment
DATABASE_URL=postgresql://localhost:5432/test
API_KEY=secret123
PORT=8080
`
		if _, err := tmpfile.WriteString(content); err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()

		result := ValidateConfigFile(tmpfile.Name(), true)
		if result.HasErrors() {
			t.Errorf("Should not have errors for valid file: %v", result.Errors)
		}
	})

	t.Run("Invalid format", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "test.env")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		// Write invalid content
		content := `INVALID LINE WITHOUT EQUALS
ANOTHER BAD LINE
KEY=value
`
		if _, err := tmpfile.WriteString(content); err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()

		result := ValidateConfigFile(tmpfile.Name(), true)
		if !result.HasErrors() {
			t.Error("Should have errors for invalid format")
		}
	})
}

// Test validation caching
func TestValidationCache(t *testing.T) {
	cache := NewValidationCache(true)

	t.Run("Cache miss", func(t *testing.T) {
		_, found := cache.Get("test-key")
		if found {
			t.Error("Should not find non-existent key")
		}
	})

	t.Run("Cache set and get", func(t *testing.T) {
		result := &ValidationResult{
			Errors: []*ValidationError{
				{Summary: "Test error"},
			},
		}

		cache.Set("test-key", result, 5*time.Second)

		cached, found := cache.Get("test-key")
		if !found {
			t.Error("Should find cached result")
		}
		if cached == nil || len(cached.Errors) != 1 {
			t.Error("Cached result should match original")
		}
	})

	t.Run("Cache expiration", func(t *testing.T) {
		result := &ValidationResult{}
		cache.Set("expire-key", result, 1*time.Millisecond)

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		_, found := cache.Get("expire-key")
		if found {
			t.Error("Should not find expired entry")
		}
	})

	t.Run("Cache clear", func(t *testing.T) {
		cache.Set("clear-key", &ValidationResult{}, 10*time.Second)
		cache.Clear()

		if cache.Size() != 0 {
			t.Error("Cache should be empty after clear")
		}
	})

	t.Run("Disabled cache", func(t *testing.T) {
		disabledCache := NewValidationCache(false)
		disabledCache.Set("key", &ValidationResult{}, 10*time.Second)

		_, found := disabledCache.Get("key")
		if found {
			t.Error("Disabled cache should not store entries")
		}
	})
}

// Test parallel validation
func TestParallelValidation(t *testing.T) {
	t.Run("Fast validation", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()

		result := FastValidation(ctx)

		duration := time.Since(start)

		if result == nil {
			t.Error("Should return result")
		}

		// Fast validation should complete quickly
		if duration > 2*time.Second {
			t.Errorf("Fast validation took too long: %v", duration)
		}
	})

	t.Run("Parallel validator", func(t *testing.T) {
		opts := ValidationOptions{
			Phase:   PhaseStartup,
			Timeout: 30,
		}

		pv := NewParallelValidator(opts)
		ctx := context.Background()

		result := pv.ValidateAllParallel(ctx)

		if result == nil {
			t.Error("Should return result")
		}
	})

	t.Run("Concurrency control", func(t *testing.T) {
		opts := ValidationOptions{
			Phase:   PhaseStartup,
			Timeout: 30,
		}

		ctx := context.Background()
		result := ValidateWithConcurrency(ctx, opts, 2)

		if result == nil {
			t.Error("Should return result")
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		opts := ValidationOptions{
			Phase:   PhaseStartup,
			Timeout: 30,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		pv := NewParallelValidator(opts)
		result := pv.ValidateAllParallel(ctx)

		// Should still return a result even if cancelled
		if result == nil {
			t.Error("Should return result even when cancelled")
		}
	})
}

// Test metrics collection
func TestMetricsCollection(t *testing.T) {
	collector := NewMetricsCollector()

	t.Run("Record validation", func(t *testing.T) {
		result := &ValidationResult{
			Errors: []*ValidationError{{Summary: "Test"}},
		}

		collector.RecordValidation(100*time.Millisecond, result)

		metrics := collector.GetMetrics()
		if metrics.TotalValidations != 1 {
			t.Error("Should have 1 validation")
		}
		if metrics.TotalErrors != 1 {
			t.Error("Should have 1 error")
		}
	})

	t.Run("Average duration", func(t *testing.T) {
		collector.Reset()

		collector.RecordValidation(100*time.Millisecond, &ValidationResult{})
		collector.RecordValidation(200*time.Millisecond, &ValidationResult{})

		metrics := collector.GetMetrics()
		if metrics.AverageDuration != 150*time.Millisecond {
			t.Errorf("Average should be 150ms, got %v", metrics.AverageDuration)
		}
	})

	t.Run("Min/max duration", func(t *testing.T) {
		collector.Reset()

		collector.RecordValidation(50*time.Millisecond, &ValidationResult{})
		collector.RecordValidation(200*time.Millisecond, &ValidationResult{})
		collector.RecordValidation(100*time.Millisecond, &ValidationResult{})

		metrics := collector.GetMetrics()
		if metrics.MinDuration != 50*time.Millisecond {
			t.Errorf("Min should be 50ms, got %v", metrics.MinDuration)
		}
		if metrics.MaxDuration != 200*time.Millisecond {
			t.Errorf("Max should be 200ms, got %v", metrics.MaxDuration)
		}
	})

	t.Run("Success rate", func(t *testing.T) {
		collector.Reset()

		// 2 successes
		collector.RecordValidation(100*time.Millisecond, &ValidationResult{})
		collector.RecordValidation(100*time.Millisecond, &ValidationResult{})

		// 1 failure
		collector.RecordValidation(100*time.Millisecond, &ValidationResult{
			Errors: []*ValidationError{{Summary: "Error"}},
		})

		metrics := collector.GetMetrics()
		expectedRate := 66.66666666666666
		if metrics.SuccessRate < expectedRate-0.01 || metrics.SuccessRate > expectedRate+0.01 {
			t.Errorf("Success rate should be ~66.67%%, got %.2f%%", metrics.SuccessRate)
		}
	})

	t.Run("Cache metrics", func(t *testing.T) {
		collector.Reset()

		collector.RecordCacheHit()
		collector.RecordCacheHit()
		collector.RecordCacheMiss()

		metrics := collector.GetMetrics()
		if metrics.CacheHits != 2 {
			t.Errorf("Should have 2 cache hits, got %d", metrics.CacheHits)
		}
		if metrics.CacheMisses != 1 {
			t.Errorf("Should have 1 cache miss, got %d", metrics.CacheMisses)
		}
		expectedRate := 66.66666666666666
		if metrics.CacheHitRate < expectedRate-0.01 || metrics.CacheHitRate > expectedRate+0.01 {
			t.Errorf("Cache hit rate should be ~66.67%%, got %.2f%%", metrics.CacheHitRate)
		}
	})

	t.Run("Component metrics", func(t *testing.T) {
		collector.Reset()

		collector.RecordComponent("database", 50*time.Millisecond)
		collector.RecordComponent("database", 100*time.Millisecond)
		collector.RecordComponent("provider", 75*time.Millisecond)

		metrics := collector.GetMetrics()
		if metrics.DatabaseValidationCount != 2 {
			t.Errorf("Should have 2 database validations, got %d", metrics.DatabaseValidationCount)
		}
		if metrics.DatabaseValidationTime != 150*time.Millisecond {
			t.Errorf("Database time should be 150ms, got %v", metrics.DatabaseValidationTime)
		}
		if metrics.ProviderValidationCount != 1 {
			t.Errorf("Should have 1 provider validation, got %d", metrics.ProviderValidationCount)
		}
	})
}

// Test instrumented validator
func TestInstrumentedValidator(t *testing.T) {
	t.Run("Validation with metrics", func(t *testing.T) {
		opts := ValidationOptions{
			Phase:        PhaseStartup,
			SkipDatabase: true,
			SkipProvider: true,
			SkipLiteLLM:  true,
		}

		iv := NewInstrumentedValidator(opts)
		ctx := context.Background()

		// Reset global metrics
		GetGlobalMetrics().Reset()

		result := iv.ValidateAll(ctx)

		if result == nil {
			t.Error("Should return result")
		}

		metrics := GetGlobalMetrics().GetMetrics()
		if metrics.TotalValidations != 1 {
			t.Errorf("Should have 1 validation, got %d", metrics.TotalValidations)
		}
	})

	t.Run("Component validation", func(t *testing.T) {
		opts := ValidationOptions{
			Phase: PhaseStartup,
		}

		iv := NewInstrumentedValidator(opts)
		ctx := context.Background()

		// Reset metrics
		GetGlobalMetrics().Reset()

		result := iv.ValidateComponent(ctx, "environment")

		if result == nil {
			t.Error("Should return result")
		}

		metrics := GetGlobalMetrics().GetMetrics()
		if metrics.EnvironmentValidationCount != 1 {
			t.Errorf("Should have 1 environment validation, got %d", metrics.EnvironmentValidationCount)
		}
	})
}

// Test report generation
func TestReportGeneration(t *testing.T) {
	t.Run("Create report", func(t *testing.T) {
		result := &ValidationResult{
			Errors: []*ValidationError{
				{
					Category: CategoryDatabase,
					Severity: SeverityError,
					Summary:  "Test error",
					Details:  "Test details",
				},
			},
			Warnings: []*ValidationError{
				{
					Category: CategoryEnvironment,
					Severity: SeverityWarning,
					Summary:  "Test warning",
				},
			},
		}

		opts := ValidationOptions{Phase: PhaseStartup}
		report := NewValidationReport(result, 150*time.Millisecond, opts)

		if report.Status != "fail" {
			t.Errorf("Status should be 'fail', got '%s'", report.Status)
		}
		if report.ErrorCount != 1 {
			t.Errorf("Should have 1 error, got %d", report.ErrorCount)
		}
		if report.WarningCount != 1 {
			t.Errorf("Should have 1 warning, got %d", report.WarningCount)
		}
	})

	t.Run("Report to JSON", func(t *testing.T) {
		result := &ValidationResult{}
		opts := ValidationOptions{Phase: PhaseStartup}
		report := NewValidationReport(result, 100*time.Millisecond, opts)

		json, err := report.ToJSON()
		if err != nil {
			t.Errorf("Failed to convert to JSON: %v", err)
		}
		if len(json) == 0 {
			t.Error("JSON should not be empty")
		}
	})

	t.Run("Report summary", func(t *testing.T) {
		result := &ValidationResult{}
		opts := ValidationOptions{Phase: PhaseStartup}
		report := NewValidationReport(result, 100*time.Millisecond, opts)

		summary := report.Summary()
		if summary == "" {
			t.Error("Summary should not be empty")
		}
		if !containsString(summary, "PASSED") {
			t.Error("Summary should contain PASSED")
		}
	})

	t.Run("Save JSON report", func(t *testing.T) {
		result := &ValidationResult{}
		opts := ValidationOptions{Phase: PhaseStartup}
		report := NewValidationReport(result, 100*time.Millisecond, opts)

		tmpfile, err := os.CreateTemp("", "report-*.json")
		if err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		err = report.SaveJSON(tmpfile.Name())
		if err != nil {
			t.Errorf("Failed to save JSON: %v", err)
		}

		// Verify file exists and is not empty
		stat, err := os.Stat(tmpfile.Name())
		if err != nil {
			t.Errorf("Failed to stat file: %v", err)
		}
		if stat.Size() == 0 {
			t.Error("JSON file should not be empty")
		}
	})
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
