package model

import (
	"log"
	"sync"
	"time"

	shared "plandex-shared"
)

// =============================================================================
// GRACEFUL DEGRADATION MANAGER
// =============================================================================
//
// The graceful degradation manager automatically reduces system quality when
// providers are struggling, ensuring the system remains functional even under
// adverse conditions.
//
// Degradation strategies:
// - Reduce context size to fit within limits
// - Switch to faster/cheaper models
// - Disable non-essential features
// - Increase timeouts
// - Queue non-urgent requests
//
// Benefits:
// - System remains functional under stress
// - Users get partial results instead of failures
// - Automatic recovery when conditions improve
// - Transparent degradation with user notification
//
// =============================================================================

// DegradationLevel represents the current degradation level
type DegradationLevel string

const (
	DegradationNone     DegradationLevel = "none"     // Normal operation
	DegradationLight    DegradationLevel = "light"    // Minor optimizations
	DegradationModerate DegradationLevel = "moderate" // Significant reductions
	DegradationHeavy    DegradationLevel = "heavy"    // Minimal functionality
	DegradationCritical DegradationLevel = "critical" // Emergency mode
)

// DegradationManager manages graceful degradation
type DegradationManager struct {
	mu sync.RWMutex

	// Current degradation state
	globalLevel     DegradationLevel
	providerLevels  map[string]DegradationLevel
	featureLevels   map[string]DegradationLevel

	// Configuration
	config DegradationConfig

	// Active degradations
	activeDegradations []ActiveDegradation

	// Callbacks
	onDegradationChange func(level DegradationLevel, reason string)
}

// ActiveDegradation tracks an active degradation
type ActiveDegradation struct {
	Id          string           `json:"id"`
	Level       DegradationLevel `json:"level"`
	Reason      string           `json:"reason"`
	Provider    string           `json:"provider,omitempty"`
	Feature     string           `json:"feature,omitempty"`
	StartedAt   time.Time        `json:"startedAt"`
	ExpiresAt   *time.Time       `json:"expiresAt,omitempty"`
	AutoRecover bool             `json:"autoRecover"`
}

// DegradationConfig configures degradation behavior
type DegradationConfig struct {
	// Thresholds for automatic degradation
	LightThreshold    int `json:"lightThreshold"`    // Error rate % to trigger light
	ModerateThreshold int `json:"moderateThreshold"` // Error rate % to trigger moderate
	HeavyThreshold    int `json:"heavyThreshold"`    // Error rate % to trigger heavy
	CriticalThreshold int `json:"criticalThreshold"` // Error rate % to trigger critical

	// Recovery settings
	RecoveryCheckInterval time.Duration `json:"recoveryCheckInterval"`
	RecoverySuccessCount  int           `json:"recoverySuccessCount"` // Successes needed to recover

	// Degradation durations
	MinDegradationDuration time.Duration `json:"minDegradationDuration"`
	MaxDegradationDuration time.Duration `json:"maxDegradationDuration"`

	// Feature-specific settings
	DisableableFeatures []string `json:"disableableFeatures"`
}

// DefaultDegradationConfig provides sensible defaults
var DefaultDegradationConfig = DegradationConfig{
	LightThreshold:         10,
	ModerateThreshold:      25,
	HeavyThreshold:         50,
	CriticalThreshold:      75,
	RecoveryCheckInterval:  30 * time.Second,
	RecoverySuccessCount:   5,
	MinDegradationDuration: 1 * time.Minute,
	MaxDegradationDuration: 30 * time.Minute,
	DisableableFeatures:    []string{"caching", "streaming", "parallel_requests"},
}

// DegradationStrategy defines how to degrade for a specific level
type DegradationStrategy struct {
	Level DegradationLevel `json:"level"`

	// Context modifications
	MaxContextTokens     int     `json:"maxContextTokens,omitempty"`
	ContextReductionPct  float64 `json:"contextReductionPct,omitempty"`

	// Model modifications
	PreferFasterModels   bool   `json:"preferFasterModels,omitempty"`
	PreferCheaperModels  bool   `json:"preferCheaperModels,omitempty"`
	FallbackModel        string `json:"fallbackModel,omitempty"`

	// Timeout modifications
	TimeoutMultiplier    float64 `json:"timeoutMultiplier,omitempty"`

	// Feature disabling
	DisabledFeatures     []string `json:"disabledFeatures,omitempty"`

	// Request modifications
	MaxConcurrentRequests int  `json:"maxConcurrentRequests,omitempty"`
	QueueNonUrgent       bool `json:"queueNonUrgent,omitempty"`

	// Retry modifications
	ReduceRetries        bool `json:"reduceRetries,omitempty"`
	MaxRetries           int  `json:"maxRetries,omitempty"`
}

// DefaultDegradationStrategies provides strategies for each level
var DefaultDegradationStrategies = map[DegradationLevel]DegradationStrategy{
	DegradationNone: {
		Level: DegradationNone,
	},
	DegradationLight: {
		Level:                DegradationLight,
		ContextReductionPct:  0.1, // Reduce context by 10%
		TimeoutMultiplier:    1.5, // Increase timeouts by 50%
		DisabledFeatures:     []string{},
	},
	DegradationModerate: {
		Level:                 DegradationModerate,
		ContextReductionPct:   0.25, // Reduce context by 25%
		PreferFasterModels:    true,
		TimeoutMultiplier:     2.0,
		DisabledFeatures:      []string{"caching"},
		MaxConcurrentRequests: 5,
	},
	DegradationHeavy: {
		Level:                 DegradationHeavy,
		ContextReductionPct:   0.5, // Reduce context by 50%
		PreferFasterModels:    true,
		PreferCheaperModels:   true,
		TimeoutMultiplier:     3.0,
		DisabledFeatures:      []string{"caching", "streaming"},
		MaxConcurrentRequests: 2,
		QueueNonUrgent:        true,
		ReduceRetries:         true,
		MaxRetries:            2,
	},
	DegradationCritical: {
		Level:                 DegradationCritical,
		MaxContextTokens:      4000,
		PreferFasterModels:    true,
		PreferCheaperModels:   true,
		TimeoutMultiplier:     5.0,
		DisabledFeatures:      []string{"caching", "streaming", "parallel_requests"},
		MaxConcurrentRequests: 1,
		QueueNonUrgent:        true,
		ReduceRetries:         true,
		MaxRetries:            1,
	},
}

// GlobalDegradationManager is the singleton instance
var GlobalDegradationManager *DegradationManager

// InitGlobalDegradationManager initializes the global degradation manager
func InitGlobalDegradationManager() {
	GlobalDegradationManager = NewDegradationManager(nil)
}

// InitGlobalDegradationManagerWithConfig initializes with custom config
func InitGlobalDegradationManagerWithConfig(config *DegradationConfig) {
	GlobalDegradationManager = NewDegradationManager(config)
}

// NewDegradationManager creates a new degradation manager
func NewDegradationManager(config *DegradationConfig) *DegradationManager {
	if config == nil {
		config = &DefaultDegradationConfig
	}

	return &DegradationManager{
		globalLevel:        DegradationNone,
		providerLevels:     make(map[string]DegradationLevel),
		featureLevels:      make(map[string]DegradationLevel),
		config:             *config,
		activeDegradations: make([]ActiveDegradation, 0),
	}
}

// SetDegradationChangeCallback sets a callback for degradation changes
func (m *DegradationManager) SetDegradationChangeCallback(callback func(level DegradationLevel, reason string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDegradationChange = callback
}

// =============================================================================
// DEGRADATION QUERIES
// =============================================================================

// GetGlobalLevel returns the current global degradation level
func (m *DegradationManager) GetGlobalLevel() DegradationLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.globalLevel
}

// GetProviderLevel returns the degradation level for a specific provider
func (m *DegradationManager) GetProviderLevel(provider string) DegradationLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level, exists := m.providerLevels[provider]; exists {
		return level
	}
	return m.globalLevel
}

// GetEffectiveLevel returns the effective degradation level considering all factors
func (m *DegradationManager) GetEffectiveLevel(provider string) DegradationLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return the highest (most degraded) level
	maxLevel := m.globalLevel

	if providerLevel, exists := m.providerLevels[provider]; exists {
		if m.levelValue(providerLevel) > m.levelValue(maxLevel) {
			maxLevel = providerLevel
		}
	}

	return maxLevel
}

// GetStrategy returns the degradation strategy for the current level
func (m *DegradationManager) GetStrategy(provider string) DegradationStrategy {
	level := m.GetEffectiveLevel(provider)
	if strategy, exists := DefaultDegradationStrategies[level]; exists {
		return strategy
	}
	return DefaultDegradationStrategies[DegradationNone]
}

// IsFeatureEnabled checks if a feature is enabled at the current degradation level
func (m *DegradationManager) IsFeatureEnabled(provider, feature string) bool {
	strategy := m.GetStrategy(provider)
	for _, disabled := range strategy.DisabledFeatures {
		if disabled == feature {
			return false
		}
	}
	return true
}

// GetActiveDegradations returns all active degradations
func (m *DegradationManager) GetActiveDegradations() []ActiveDegradation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ActiveDegradation, len(m.activeDegradations))
	copy(result, m.activeDegradations)
	return result
}

// =============================================================================
// DEGRADATION TRIGGERS
// =============================================================================

// TriggerDegradation activates a degradation
func (m *DegradationManager) TriggerDegradation(level DegradationLevel, reason string, provider string, duration time.Duration) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateDegradationId()
	now := time.Now()
	var expiresAt *time.Time
	if duration > 0 {
		exp := now.Add(duration)
		expiresAt = &exp
	}

	degradation := ActiveDegradation{
		Id:          id,
		Level:       level,
		Reason:      reason,
		Provider:    provider,
		StartedAt:   now,
		ExpiresAt:   expiresAt,
		AutoRecover: duration > 0,
	}

	m.activeDegradations = append(m.activeDegradations, degradation)

	// Update levels
	if provider != "" {
		m.providerLevels[provider] = level
	} else {
		m.globalLevel = level
	}

	log.Printf("[Degradation] TRIGGERED: level=%s, provider=%s, reason=%s, duration=%v",
		level, provider, reason, duration)

	// Invoke callback
	if m.onDegradationChange != nil {
		go m.onDegradationChange(level, reason)
	}

	return id
}

// TriggerFromFailure automatically determines degradation level from failure
func (m *DegradationManager) TriggerFromFailure(failure *shared.ProviderFailure, errorRate int) {
	var level DegradationLevel
	var duration time.Duration

	switch {
	case errorRate >= m.config.CriticalThreshold:
		level = DegradationCritical
		duration = m.config.MaxDegradationDuration
	case errorRate >= m.config.HeavyThreshold:
		level = DegradationHeavy
		duration = 15 * time.Minute
	case errorRate >= m.config.ModerateThreshold:
		level = DegradationModerate
		duration = 10 * time.Minute
	case errorRate >= m.config.LightThreshold:
		level = DegradationLight
		duration = 5 * time.Minute
	default:
		return // No degradation needed
	}

	reason := "automatic: "
	if failure != nil {
		reason += string(failure.Type)
	} else {
		reason += "high error rate"
	}

	provider := ""
	if failure != nil {
		provider = failure.Provider
	}

	m.TriggerDegradation(level, reason, provider, duration)
}

// RecoverDegradation removes a degradation
func (m *DegradationManager) RecoverDegradation(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find and remove the degradation
	var removed *ActiveDegradation
	newDegradations := make([]ActiveDegradation, 0, len(m.activeDegradations))
	for i := range m.activeDegradations {
		if m.activeDegradations[i].Id == id {
			removed = &m.activeDegradations[i]
		} else {
			newDegradations = append(newDegradations, m.activeDegradations[i])
		}
	}
	m.activeDegradations = newDegradations

	if removed == nil {
		return
	}

	// Recalculate levels
	m.recalculateLevelsLocked()

	log.Printf("[Degradation] RECOVERED: id=%s, level=%s, provider=%s",
		id, removed.Level, removed.Provider)
}

// RecoverProvider recovers all degradations for a provider
func (m *DegradationManager) RecoverProvider(provider string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newDegradations := make([]ActiveDegradation, 0, len(m.activeDegradations))
	for i := range m.activeDegradations {
		if m.activeDegradations[i].Provider != provider {
			newDegradations = append(newDegradations, m.activeDegradations[i])
		}
	}
	m.activeDegradations = newDegradations

	delete(m.providerLevels, provider)

	log.Printf("[Degradation] RECOVERED provider: %s", provider)
}

// RecoverAll recovers all degradations
func (m *DegradationManager) RecoverAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activeDegradations = make([]ActiveDegradation, 0)
	m.globalLevel = DegradationNone
	m.providerLevels = make(map[string]DegradationLevel)
	m.featureLevels = make(map[string]DegradationLevel)

	log.Printf("[Degradation] RECOVERED ALL")

	if m.onDegradationChange != nil {
		go m.onDegradationChange(DegradationNone, "manual recovery")
	}
}

// =============================================================================
// REQUEST MODIFICATIONS
// =============================================================================

// ModifyRequest applies degradation modifications to a request
type RequestModifications struct {
	MaxTokens             int      `json:"maxTokens,omitempty"`
	TimeoutMs             int64    `json:"timeoutMs,omitempty"`
	DisabledFeatures      []string `json:"disabledFeatures,omitempty"`
	MaxRetries            int      `json:"maxRetries,omitempty"`
	ShouldQueue           bool     `json:"shouldQueue,omitempty"`
	PreferredModelHint    string   `json:"preferredModelHint,omitempty"`
}

// GetRequestModifications returns modifications to apply to a request
func (m *DegradationManager) GetRequestModifications(provider string, originalMaxTokens int, originalTimeoutMs int64, isUrgent bool) RequestModifications {
	strategy := m.GetStrategy(provider)

	mods := RequestModifications{
		MaxTokens:        originalMaxTokens,
		TimeoutMs:        originalTimeoutMs,
		DisabledFeatures: strategy.DisabledFeatures,
		MaxRetries:       -1, // No override
	}

	// Apply context reduction
	if strategy.ContextReductionPct > 0 {
		mods.MaxTokens = int(float64(originalMaxTokens) * (1 - strategy.ContextReductionPct))
	}

	// Apply max context limit
	if strategy.MaxContextTokens > 0 && mods.MaxTokens > strategy.MaxContextTokens {
		mods.MaxTokens = strategy.MaxContextTokens
	}

	// Apply timeout multiplier
	if strategy.TimeoutMultiplier > 0 {
		mods.TimeoutMs = int64(float64(originalTimeoutMs) * strategy.TimeoutMultiplier)
	}

	// Apply retry limits
	if strategy.ReduceRetries {
		mods.MaxRetries = strategy.MaxRetries
	}

	// Check if should queue
	if strategy.QueueNonUrgent && !isUrgent {
		mods.ShouldQueue = true
	}

	// Set model hints
	if strategy.PreferFasterModels {
		mods.PreferredModelHint = "faster"
	}
	if strategy.PreferCheaperModels {
		mods.PreferredModelHint = "cheaper"
	}

	return mods
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

func (m *DegradationManager) levelValue(level DegradationLevel) int {
	switch level {
	case DegradationNone:
		return 0
	case DegradationLight:
		return 1
	case DegradationModerate:
		return 2
	case DegradationHeavy:
		return 3
	case DegradationCritical:
		return 4
	default:
		return 0
	}
}

func (m *DegradationManager) recalculateLevelsLocked() {
	// Reset levels
	m.globalLevel = DegradationNone
	m.providerLevels = make(map[string]DegradationLevel)

	// Find highest level for each scope
	for _, deg := range m.activeDegradations {
		// Check expiration
		if deg.ExpiresAt != nil && time.Now().After(*deg.ExpiresAt) {
			continue
		}

		if deg.Provider != "" {
			currentLevel := m.providerLevels[deg.Provider]
			if m.levelValue(deg.Level) > m.levelValue(currentLevel) {
				m.providerLevels[deg.Provider] = deg.Level
			}
		} else {
			if m.levelValue(deg.Level) > m.levelValue(m.globalLevel) {
				m.globalLevel = deg.Level
			}
		}
	}
}

func generateDegradationId() string {
	return shared.GenerateIdWithPrefix("deg")
}

// =============================================================================
// METRICS AND REPORTING
// =============================================================================

// DegradationMetrics provides degradation metrics
type DegradationMetrics struct {
	GlobalLevel        DegradationLevel            `json:"globalLevel"`
	ProviderLevels     map[string]DegradationLevel `json:"providerLevels"`
	ActiveCount        int                         `json:"activeCount"`
	ActiveDegradations []ActiveDegradation         `json:"activeDegradations"`
}

// GetMetrics returns degradation metrics
func (m *DegradationManager) GetMetrics() DegradationMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Clean up expired degradations first
	m.cleanupExpiredLocked()

	metrics := DegradationMetrics{
		GlobalLevel:        m.globalLevel,
		ProviderLevels:     make(map[string]DegradationLevel),
		ActiveCount:        len(m.activeDegradations),
		ActiveDegradations: make([]ActiveDegradation, len(m.activeDegradations)),
	}

	for k, v := range m.providerLevels {
		metrics.ProviderLevels[k] = v
	}

	copy(metrics.ActiveDegradations, m.activeDegradations)

	return metrics
}

func (m *DegradationManager) cleanupExpiredLocked() {
	now := time.Now()
	newDegradations := make([]ActiveDegradation, 0, len(m.activeDegradations))

	for _, deg := range m.activeDegradations {
		if deg.ExpiresAt == nil || now.Before(*deg.ExpiresAt) {
			newDegradations = append(newDegradations, deg)
		}
	}

	if len(newDegradations) != len(m.activeDegradations) {
		m.activeDegradations = newDegradations
		m.recalculateLevelsLocked()
	}
}
