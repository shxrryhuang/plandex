package model

import (
	"context"
	"log"
	"sync"
	"time"

	shared "plandex-shared"
)

// =============================================================================
// HEALTH CHECK SYSTEM
// =============================================================================
//
// The health check system provides proactive monitoring of provider health.
// It periodically checks providers and maintains health scores that can be
// used to make routing decisions before sending actual requests.
//
// Benefits:
// - Detect provider issues before they affect user requests
// - Enable smart routing to healthy providers
// - Reduce failed requests by avoiding unhealthy providers
// - Provide visibility into provider status
//
// =============================================================================

// HealthStatus represents the health state of a provider
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthCheckManager manages health checks for all providers
type HealthCheckManager struct {
	mu sync.RWMutex

	// Per-provider health state
	providers map[string]*ProviderHealth

	// Configuration
	config HealthCheckConfig

	// Stop channel for cleanup
	stopCh chan struct{}

	// Optional callback for health changes
	onHealthChange func(provider string, oldStatus, newStatus HealthStatus)
}

// ProviderHealth tracks health state for a single provider
type ProviderHealth struct {
	// Provider identifier
	Provider string `json:"provider"`

	// Current status
	Status HealthStatus `json:"status"`

	// Health score (0-100, higher is healthier)
	Score int `json:"score"`

	// Latency tracking (rolling average in ms)
	AvgLatencyMs int64 `json:"avgLatencyMs"`
	P95LatencyMs int64 `json:"p95LatencyMs"`
	P99LatencyMs int64 `json:"p99LatencyMs"`

	// Success/failure rates
	SuccessRate    float64 `json:"successRate"`    // 0.0-1.0
	RecentRequests int     `json:"recentRequests"` // Requests in current window

	// Timing
	LastCheck    *time.Time `json:"lastCheck,omitempty"`
	LastSuccess  *time.Time `json:"lastSuccess,omitempty"`
	LastFailure  *time.Time `json:"lastFailure,omitempty"`
	StatusSince  time.Time  `json:"statusSince"`

	// Detailed metrics
	ConsecutiveSuccesses int `json:"consecutiveSuccesses"`
	ConsecutiveFailures  int `json:"consecutiveFailures"`

	// Recent latency samples for percentile calculation
	latencySamples []int64
}

// HealthCheckConfig configures health check behavior
type HealthCheckConfig struct {
	// CheckInterval is how often to run health checks
	CheckInterval time.Duration `json:"checkInterval"`

	// Timeout for health check requests
	CheckTimeout time.Duration `json:"checkTimeout"`

	// Thresholds for status transitions
	HealthyThreshold   int `json:"healthyThreshold"`   // Score >= this = healthy
	DegradedThreshold  int `json:"degradedThreshold"`  // Score >= this = degraded
	UnhealthyThreshold int `json:"unhealthyThreshold"` // Score < this = unhealthy

	// Latency thresholds (ms)
	HealthyLatencyMs   int64 `json:"healthyLatencyMs"`
	DegradedLatencyMs  int64 `json:"degradedLatencyMs"`

	// Success rate thresholds
	HealthySuccessRate  float64 `json:"healthySuccessRate"`
	DegradedSuccessRate float64 `json:"degradedSuccessRate"`

	// Sample window for metrics
	MetricsWindow time.Duration `json:"metricsWindow"`

	// Max latency samples to keep
	MaxLatencySamples int `json:"maxLatencySamples"`
}

// DefaultHealthCheckConfig provides sensible defaults
var DefaultHealthCheckConfig = HealthCheckConfig{
	CheckInterval:       30 * time.Second,
	CheckTimeout:        10 * time.Second,
	HealthyThreshold:    80,
	DegradedThreshold:   50,
	UnhealthyThreshold:  50,
	HealthyLatencyMs:    1000,
	DegradedLatencyMs:   3000,
	HealthySuccessRate:  0.95,
	DegradedSuccessRate: 0.80,
	MetricsWindow:       5 * time.Minute,
	MaxLatencySamples:   100,
}

// GlobalHealthCheckManager is the singleton instance
var GlobalHealthCheckManager *HealthCheckManager

// InitGlobalHealthCheckManager initializes the global health check manager
func InitGlobalHealthCheckManager() {
	GlobalHealthCheckManager = NewHealthCheckManager(nil)
}

// InitGlobalHealthCheckManagerWithConfig initializes with custom config
func InitGlobalHealthCheckManagerWithConfig(config *HealthCheckConfig) {
	GlobalHealthCheckManager = NewHealthCheckManager(config)
}

// NewHealthCheckManager creates a new health check manager
func NewHealthCheckManager(config *HealthCheckConfig) *HealthCheckManager {
	if config == nil {
		config = &DefaultHealthCheckConfig
	}

	manager := &HealthCheckManager{
		providers: make(map[string]*ProviderHealth),
		config:    *config,
		stopCh:    make(chan struct{}),
	}

	return manager
}

// SetHealthChangeCallback sets a callback for health status changes
func (m *HealthCheckManager) SetHealthChangeCallback(callback func(provider string, oldStatus, newStatus HealthStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onHealthChange = callback
}

// Start begins the health check loop
func (m *HealthCheckManager) Start(ctx context.Context) {
	go m.healthCheckLoop(ctx)
}

// Stop stops the health check loop
func (m *HealthCheckManager) Stop() {
	close(m.stopCh)
}

// =============================================================================
// HEALTH QUERIES
// =============================================================================

// GetHealth returns the health state for a provider
func (m *HealthCheckManager) GetHealth(provider string) *ProviderHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	health, exists := m.providers[provider]
	if !exists {
		return &ProviderHealth{
			Provider: provider,
			Status:   HealthStatusUnknown,
			Score:    50, // Unknown = middle score
		}
	}

	// Return a copy
	copy := *health
	return &copy
}

// GetStatus returns just the health status for a provider
func (m *HealthCheckManager) GetStatus(provider string) HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	health, exists := m.providers[provider]
	if !exists {
		return HealthStatusUnknown
	}
	return health.Status
}

// IsHealthy returns true if the provider is healthy
func (m *HealthCheckManager) IsHealthy(provider string) bool {
	return m.GetStatus(provider) == HealthStatusHealthy
}

// GetHealthyProviders returns a list of healthy providers
func (m *HealthCheckManager) GetHealthyProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var healthy []string
	for provider, health := range m.providers {
		if health.Status == HealthStatusHealthy {
			healthy = append(healthy, provider)
		}
	}
	return healthy
}

// GetBestProvider returns the healthiest provider from a list
func (m *HealthCheckManager) GetBestProvider(providers []string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var bestProvider string
	var bestScore int = -1

	for _, provider := range providers {
		health, exists := m.providers[provider]
		if !exists {
			// Unknown provider gets middle score
			if bestScore < 50 {
				bestScore = 50
				bestProvider = provider
			}
			continue
		}

		if health.Score > bestScore {
			bestScore = health.Score
			bestProvider = provider
		}
	}

	return bestProvider
}

// =============================================================================
// METRIC RECORDING
// =============================================================================

// RecordRequest records a request result for health calculation
func (m *HealthCheckManager) RecordRequest(provider string, success bool, latencyMs int64, failure *shared.ProviderFailure) {
	m.mu.Lock()
	defer m.mu.Unlock()

	health := m.getOrCreateHealth(provider)
	now := time.Now()

	health.RecentRequests++

	if success {
		health.ConsecutiveSuccesses++
		health.ConsecutiveFailures = 0
		health.LastSuccess = &now
	} else {
		health.ConsecutiveFailures++
		health.ConsecutiveSuccesses = 0
		health.LastFailure = &now
	}

	// Record latency sample
	if latencyMs > 0 {
		health.latencySamples = append(health.latencySamples, latencyMs)
		if len(health.latencySamples) > m.config.MaxLatencySamples {
			health.latencySamples = health.latencySamples[1:]
		}
	}

	// Recalculate score and status
	m.recalculateHealthLocked(health)
}

// RecordLatency records a latency measurement
func (m *HealthCheckManager) RecordLatency(provider string, latencyMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	health := m.getOrCreateHealth(provider)

	health.latencySamples = append(health.latencySamples, latencyMs)
	if len(health.latencySamples) > m.config.MaxLatencySamples {
		health.latencySamples = health.latencySamples[1:]
	}

	m.recalculateHealthLocked(health)
}

// =============================================================================
// HEALTH CHECK LOOP
// =============================================================================

func (m *HealthCheckManager) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.runHealthChecks()
		}
	}
}

func (m *HealthCheckManager) runHealthChecks() {
	m.mu.Lock()
	providers := make([]string, 0, len(m.providers))
	for p := range m.providers {
		providers = append(providers, p)
	}
	m.mu.Unlock()

	for _, provider := range providers {
		m.checkProvider(provider)
	}
}

func (m *HealthCheckManager) checkProvider(provider string) {
	// This is a placeholder for actual health check logic
	// In practice, this would make a lightweight API call to verify the provider is responding

	m.mu.Lock()
	defer m.mu.Unlock()

	health, exists := m.providers[provider]
	if !exists {
		return
	}

	now := time.Now()
	health.LastCheck = &now

	// Recalculate health based on recent metrics
	m.recalculateHealthLocked(health)
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

func (m *HealthCheckManager) getOrCreateHealth(provider string) *ProviderHealth {
	health, exists := m.providers[provider]
	if !exists {
		health = &ProviderHealth{
			Provider:       provider,
			Status:         HealthStatusUnknown,
			Score:          50,
			StatusSince:    time.Now(),
			latencySamples: make([]int64, 0, m.config.MaxLatencySamples),
		}
		m.providers[provider] = health
	}
	return health
}

func (m *HealthCheckManager) recalculateHealthLocked(health *ProviderHealth) {
	oldStatus := health.Status

	// Calculate latency percentiles
	if len(health.latencySamples) > 0 {
		health.AvgLatencyMs = m.calculateAverage(health.latencySamples)
		health.P95LatencyMs = m.calculatePercentile(health.latencySamples, 95)
		health.P99LatencyMs = m.calculatePercentile(health.latencySamples, 99)
	}

	// Calculate success rate
	if health.RecentRequests > 0 {
		totalSuccesses := health.ConsecutiveSuccesses
		// Estimate from consecutive counts (simplified)
		if health.ConsecutiveFailures > 0 {
			health.SuccessRate = float64(totalSuccesses) / float64(totalSuccesses+health.ConsecutiveFailures)
		} else {
			health.SuccessRate = 1.0
		}
	}

	// Calculate health score (0-100)
	score := 50 // Base score

	// Adjust for success rate
	if health.SuccessRate >= m.config.HealthySuccessRate {
		score += 25
	} else if health.SuccessRate >= m.config.DegradedSuccessRate {
		score += 10
	} else if health.SuccessRate < m.config.DegradedSuccessRate {
		score -= 25
	}

	// Adjust for latency
	if health.P95LatencyMs > 0 {
		if health.P95LatencyMs <= m.config.HealthyLatencyMs {
			score += 25
		} else if health.P95LatencyMs <= m.config.DegradedLatencyMs {
			score += 10
		} else {
			score -= 15
		}
	}

	// Adjust for consecutive failures
	if health.ConsecutiveFailures >= 5 {
		score -= 30
	} else if health.ConsecutiveFailures >= 3 {
		score -= 15
	}

	// Adjust for consecutive successes
	if health.ConsecutiveSuccesses >= 10 {
		score += 10
	}

	// Clamp score
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	health.Score = score

	// Determine status from score
	if score >= m.config.HealthyThreshold {
		health.Status = HealthStatusHealthy
	} else if score >= m.config.DegradedThreshold {
		health.Status = HealthStatusDegraded
	} else {
		health.Status = HealthStatusUnhealthy
	}

	// Track status change time
	if health.Status != oldStatus {
		health.StatusSince = time.Now()

		// Invoke callback
		if m.onHealthChange != nil {
			go m.onHealthChange(health.Provider, oldStatus, health.Status)
		}

		log.Printf("[HealthCheck] %s: %s -> %s (score=%d)",
			health.Provider, oldStatus, health.Status, health.Score)
	}
}

func (m *HealthCheckManager) calculateAverage(samples []int64) int64 {
	if len(samples) == 0 {
		return 0
	}
	var sum int64
	for _, s := range samples {
		sum += s
	}
	return sum / int64(len(samples))
}

func (m *HealthCheckManager) calculatePercentile(samples []int64, percentile int) int64 {
	if len(samples) == 0 {
		return 0
	}

	// Simple implementation - for production, use a proper percentile library
	sorted := make([]int64, len(samples))
	copy(sorted, samples)

	// Simple bubble sort (fine for small samples)
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	index := (percentile * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// =============================================================================
// METRICS AND REPORTING
// =============================================================================

// HealthCheckMetrics provides aggregate health metrics
type HealthCheckMetrics struct {
	TotalProviders    int                        `json:"totalProviders"`
	HealthyCount      int                        `json:"healthyCount"`
	DegradedCount     int                        `json:"degradedCount"`
	UnhealthyCount    int                        `json:"unhealthyCount"`
	UnknownCount      int                        `json:"unknownCount"`
	AverageScore      int                        `json:"averageScore"`
	ProviderDetails   map[string]*ProviderHealth `json:"providerDetails"`
}

// GetMetrics returns aggregate health metrics
func (m *HealthCheckManager) GetMetrics() HealthCheckMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := HealthCheckMetrics{
		TotalProviders:  len(m.providers),
		ProviderDetails: make(map[string]*ProviderHealth),
	}

	var totalScore int
	for provider, health := range m.providers {
		switch health.Status {
		case HealthStatusHealthy:
			metrics.HealthyCount++
		case HealthStatusDegraded:
			metrics.DegradedCount++
		case HealthStatusUnhealthy:
			metrics.UnhealthyCount++
		default:
			metrics.UnknownCount++
		}
		totalScore += health.Score

		// Copy for details
		copy := *health
		metrics.ProviderDetails[provider] = &copy
	}

	if metrics.TotalProviders > 0 {
		metrics.AverageScore = totalScore / metrics.TotalProviders
	}

	return metrics
}
