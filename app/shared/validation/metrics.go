package validation

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ValidationMetrics tracks performance metrics for validation
type ValidationMetrics struct {
	mu sync.RWMutex

	// Timing metrics
	TotalValidations int64         `json:"total_validations"`
	TotalDuration    time.Duration `json:"total_duration_ms"`
	AverageDuration  time.Duration `json:"average_duration_ms"`
	MinDuration      time.Duration `json:"min_duration_ms"`
	MaxDuration      time.Duration `json:"max_duration_ms"`

	// Result metrics
	TotalErrors   int64 `json:"total_errors"`
	TotalWarnings int64 `json:"total_warnings"`
	SuccessRate   float64 `json:"success_rate_percent"`

	// Cache metrics
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
	CacheHitRate float64 `json:"cache_hit_rate_percent"`

	// Component-specific metrics
	DatabaseValidationCount  int64         `json:"database_validation_count"`
	DatabaseValidationTime   time.Duration `json:"database_validation_time_ms"`
	ProviderValidationCount  int64         `json:"provider_validation_count"`
	ProviderValidationTime   time.Duration `json:"provider_validation_time_ms"`
	EnvironmentValidationCount int64       `json:"environment_validation_count"`
	EnvironmentValidationTime  time.Duration `json:"environment_validation_time_ms"`
}

// MetricsCollector collects validation metrics
type MetricsCollector struct {
	metrics *ValidationMetrics
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: &ValidationMetrics{
			MinDuration: time.Duration(^uint64(0) >> 1), // Max int64
		},
	}
}

// RecordValidation records a validation execution
func (mc *MetricsCollector) RecordValidation(duration time.Duration, result *ValidationResult) {
	mc.metrics.mu.Lock()
	defer mc.metrics.mu.Unlock()

	mc.metrics.TotalValidations++
	mc.metrics.TotalDuration += duration

	if duration < mc.metrics.MinDuration {
		mc.metrics.MinDuration = duration
	}
	if duration > mc.metrics.MaxDuration {
		mc.metrics.MaxDuration = duration
	}

	mc.metrics.AverageDuration = mc.metrics.TotalDuration / time.Duration(mc.metrics.TotalValidations)

	if result != nil {
		mc.metrics.TotalErrors += int64(len(result.Errors))
		mc.metrics.TotalWarnings += int64(len(result.Warnings))
	}

	// Calculate success rate
	successCount := mc.metrics.TotalValidations
	if result != nil && result.HasErrors() {
		successCount--
	}
	mc.metrics.SuccessRate = float64(successCount) / float64(mc.metrics.TotalValidations) * 100
}

// RecordCacheHit records a cache hit
func (mc *MetricsCollector) RecordCacheHit() {
	mc.metrics.mu.Lock()
	defer mc.metrics.mu.Unlock()

	mc.metrics.CacheHits++
	total := mc.metrics.CacheHits + mc.metrics.CacheMisses
	if total > 0 {
		mc.metrics.CacheHitRate = float64(mc.metrics.CacheHits) / float64(total) * 100
	}
}

// RecordCacheMiss records a cache miss
func (mc *MetricsCollector) RecordCacheMiss() {
	mc.metrics.mu.Lock()
	defer mc.metrics.mu.Unlock()

	mc.metrics.CacheMisses++
	total := mc.metrics.CacheHits + mc.metrics.CacheMisses
	if total > 0 {
		mc.metrics.CacheHitRate = float64(mc.metrics.CacheHits) / float64(total) * 100
	}
}

// RecordComponent records timing for a specific component
func (mc *MetricsCollector) RecordComponent(component string, duration time.Duration) {
	mc.metrics.mu.Lock()
	defer mc.metrics.mu.Unlock()

	switch component {
	case "database":
		mc.metrics.DatabaseValidationCount++
		mc.metrics.DatabaseValidationTime += duration
	case "provider":
		mc.metrics.ProviderValidationCount++
		mc.metrics.ProviderValidationTime += duration
	case "environment":
		mc.metrics.EnvironmentValidationCount++
		mc.metrics.EnvironmentValidationTime += duration
	}
}

// GetMetrics returns a copy of current metrics
func (mc *MetricsCollector) GetMetrics() ValidationMetrics {
	mc.metrics.mu.RLock()
	defer mc.metrics.mu.RUnlock()

	return *mc.metrics
}

// Reset resets all metrics
func (mc *MetricsCollector) Reset() {
	mc.metrics.mu.Lock()
	defer mc.metrics.mu.Unlock()

	mc.metrics = &ValidationMetrics{
		MinDuration: time.Duration(^uint64(0) >> 1),
	}
}

// Global metrics collector
var globalMetrics = NewMetricsCollector()

// GetGlobalMetrics returns the global metrics collector
func GetGlobalMetrics() *MetricsCollector {
	return globalMetrics
}

// InstrumentedValidator wraps a validator with metrics collection
type InstrumentedValidator struct {
	validator *Validator
	collector *MetricsCollector
}

// NewInstrumentedValidator creates a validator with metrics collection
func NewInstrumentedValidator(opts ValidationOptions) *InstrumentedValidator {
	return &InstrumentedValidator{
		validator: NewValidator(opts),
		collector: globalMetrics,
	}
}

// ValidateAll runs validation and collects metrics
func (iv *InstrumentedValidator) ValidateAll(ctx context.Context) *ValidationResult {
	start := time.Now()

	result := iv.validator.ValidateAll(ctx)

	duration := time.Since(start)
	iv.collector.RecordValidation(duration, result)

	return result
}

// ValidateComponent validates a single component and records metrics
func (iv *InstrumentedValidator) ValidateComponent(ctx context.Context, component string) *ValidationResult {
	start := time.Now()

	var result *ValidationResult
	switch component {
	case "database":
		result = ValidateDatabase(ctx)
	case "environment":
		result = ValidateEnvironment()
	case "providers":
		result = ValidateAllProviders(iv.validator.options.CheckFileAccess)
	default:
		result = &ValidationResult{}
	}

	duration := time.Since(start)
	iv.collector.RecordComponent(component, duration)

	return result
}

// MetricsSummary returns a formatted string of metrics
func MetricsSummary() string {
	metrics := globalMetrics.GetMetrics()

	return fmt.Sprintf(`Validation Performance Metrics:

Total Validations: %d
Average Duration:  %dms
Min Duration:      %dms
Max Duration:      %dms
Success Rate:      %.2f%%

Errors:   %d
Warnings: %d

Cache Performance:
  Hits:     %d
  Misses:   %d
  Hit Rate: %.2f%%

Component Breakdown:
  Database:    %d validations, avg %dms
  Provider:    %d validations, avg %dms
  Environment: %d validations, avg %dms
`,
		metrics.TotalValidations,
		metrics.AverageDuration.Milliseconds(),
		metrics.MinDuration.Milliseconds(),
		metrics.MaxDuration.Milliseconds(),
		metrics.SuccessRate,
		metrics.TotalErrors,
		metrics.TotalWarnings,
		metrics.CacheHits,
		metrics.CacheMisses,
		metrics.CacheHitRate,
		metrics.DatabaseValidationCount,
		avgDuration(metrics.DatabaseValidationTime, metrics.DatabaseValidationCount),
		metrics.ProviderValidationCount,
		avgDuration(metrics.ProviderValidationTime, metrics.ProviderValidationCount),
		metrics.EnvironmentValidationCount,
		avgDuration(metrics.EnvironmentValidationTime, metrics.EnvironmentValidationCount),
	)
}

func avgDuration(total time.Duration, count int64) int64 {
	if count == 0 {
		return 0
	}
	return (total / time.Duration(count)).Milliseconds()
}
