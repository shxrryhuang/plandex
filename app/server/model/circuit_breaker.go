package model

import (
	"log"
	"sync"
	"time"

	shared "plandex-shared"
)

// =============================================================================
// CIRCUIT BREAKER
// =============================================================================
//
// The circuit breaker pattern prevents cascading failures by temporarily
// stopping requests to providers that are experiencing problems.
//
// States:
//   - Closed: Normal operation, requests flow through
//   - Open: Provider is failing, requests are rejected
//   - HalfOpen: Testing if provider has recovered
//
// =============================================================================

// CircuitState represents the state of a circuit breaker
type CircuitState string

const (
	// CircuitClosed allows requests through normally
	CircuitClosed CircuitState = "closed"

	// CircuitOpen rejects requests to protect the system
	CircuitOpen CircuitState = "open"

	// CircuitHalfOpen allows limited requests to test recovery
	CircuitHalfOpen CircuitState = "half_open"
)

// CircuitBreaker tracks provider health and prevents cascading failures
type CircuitBreaker struct {
	mu sync.RWMutex

	// Per-provider circuit state
	providers map[string]*ProviderCircuit

	// Configuration
	config CircuitBreakerConfig
}

// ProviderCircuit tracks a single provider's circuit state
type ProviderCircuit struct {
	// Provider identifier
	Provider string `json:"provider"`

	// Current state
	State CircuitState `json:"state"`

	// Failure tracking
	ConsecutiveFailures int `json:"consecutiveFailures"`
	TotalFailures       int `json:"totalFailures"`
	TotalRequests       int `json:"totalRequests"`
	TotalSuccesses      int `json:"totalSuccesses"`

	// Timing
	LastFailure *time.Time `json:"lastFailure,omitempty"`
	LastSuccess *time.Time `json:"lastSuccess,omitempty"`
	OpenedAt    *time.Time `json:"openedAt,omitempty"`
	ClosedAt    *time.Time `json:"closedAt,omitempty"`

	// Half-open testing
	HalfOpenRequests  int `json:"halfOpenRequests"`
	HalfOpenSuccesses int `json:"halfOpenSuccesses"`

	// Recent failures for analysis (sliding window)
	RecentFailures []CircuitFailure `json:"recentFailures,omitempty"`
}

// CircuitFailure records a single failure event
type CircuitFailure struct {
	Timestamp    time.Time          `json:"timestamp"`
	FailureType  shared.FailureType `json:"failureType"`
	ErrorMessage string             `json:"errorMessage,omitempty"`
	HTTPCode     int                `json:"httpCode,omitempty"`
}

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures to open the circuit
	FailureThreshold int `json:"failureThreshold"`

	// SuccessThreshold is the number of successes in half-open to close the circuit
	SuccessThreshold int `json:"successThreshold"`

	// OpenDuration is how long the circuit stays open before transitioning to half-open
	OpenDuration time.Duration `json:"openDuration"`

	// HalfOpenMaxRequests is the maximum requests allowed in half-open state
	HalfOpenMaxRequests int `json:"halfOpenMaxRequests"`

	// FailureWindowDuration is the sliding window for counting failures
	FailureWindowDuration time.Duration `json:"failureWindowDuration"`

	// FailureWindowMax is the maximum failures in the window to trigger opening
	FailureWindowMax int `json:"failureWindowMax"`

	// ExcludedFailureTypes are failure types that don't count toward circuit breaking
	// (e.g., context_too_long is a client error, not provider instability)
	ExcludedFailureTypes []shared.FailureType `json:"excludedFailureTypes"`
}

// DefaultCircuitBreakerConfig provides sensible defaults
var DefaultCircuitBreakerConfig = CircuitBreakerConfig{
	FailureThreshold:      5,
	SuccessThreshold:      2,
	OpenDuration:          30 * time.Second,
	HalfOpenMaxRequests:   3,
	FailureWindowDuration: 60 * time.Second,
	FailureWindowMax:      10,
	ExcludedFailureTypes: []shared.FailureType{
		shared.FailureTypeContextTooLong,
		shared.FailureTypeInvalidRequest,
		shared.FailureTypeContentPolicy,
		shared.FailureTypeAuthInvalid,
		shared.FailureTypePermissionDenied,
	},
}

// GlobalCircuitBreaker is the singleton circuit breaker instance
var GlobalCircuitBreaker *CircuitBreaker

// InitGlobalCircuitBreaker initializes the global circuit breaker with default config
func InitGlobalCircuitBreaker() {
	GlobalCircuitBreaker = NewCircuitBreaker(nil)
}

// InitGlobalCircuitBreakerWithConfig initializes with custom config
func InitGlobalCircuitBreakerWithConfig(config *CircuitBreakerConfig) {
	GlobalCircuitBreaker = NewCircuitBreaker(config)
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = &DefaultCircuitBreakerConfig
	}

	return &CircuitBreaker{
		providers: make(map[string]*ProviderCircuit),
		config:    *config,
	}
}

// =============================================================================
// STATE QUERIES
// =============================================================================

// IsOpen checks if the circuit is open (should reject requests)
func (cb *CircuitBreaker) IsOpen(provider string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	circuit, exists := cb.providers[provider]
	if !exists {
		return false // Unknown provider = closed circuit
	}

	switch circuit.State {
	case CircuitOpen:
		// Check if we should transition to half-open
		if circuit.OpenedAt != nil && time.Since(*circuit.OpenedAt) > cb.config.OpenDuration {
			// Transition will happen on next state change, but allow the request
			return false
		}
		return true

	case CircuitHalfOpen:
		// Allow limited requests in half-open
		return circuit.HalfOpenRequests >= cb.config.HalfOpenMaxRequests

	default:
		return false
	}
}

// GetState returns the current state for a provider
func (cb *CircuitBreaker) GetState(provider string) *ProviderCircuit {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	circuit, exists := cb.providers[provider]
	if !exists {
		return nil
	}

	// Return a copy to prevent external mutation
	copy := *circuit
	// Also copy the recent failures slice
	copy.RecentFailures = make([]CircuitFailure, len(circuit.RecentFailures))
	_ = copy.RecentFailures // Copying is done above

	return &copy
}

// GetAllStates returns state for all tracked providers
func (cb *CircuitBreaker) GetAllStates() map[string]*ProviderCircuit {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	result := make(map[string]*ProviderCircuit)
	for k, v := range cb.providers {
		copy := *v
		result[k] = &copy
	}
	return result
}

// =============================================================================
// STATE CHANGES
// =============================================================================

// RecordSuccess records a successful request and potentially closes the circuit
func (cb *CircuitBreaker) RecordSuccess(provider string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit := cb.getOrCreateCircuit(provider)
	circuit.TotalRequests++
	circuit.TotalSuccesses++
	circuit.ConsecutiveFailures = 0
	now := time.Now()
	circuit.LastSuccess = &now

	oldState := circuit.State

	switch circuit.State {
	case CircuitHalfOpen:
		circuit.HalfOpenSuccesses++
		// Check if enough successes to close
		if circuit.HalfOpenSuccesses >= cb.config.SuccessThreshold {
			cb.transitionToClosedLocked(circuit)
		}

	case CircuitOpen:
		// Check if we should transition to half-open
		if circuit.OpenedAt != nil && time.Since(*circuit.OpenedAt) > cb.config.OpenDuration {
			cb.transitionToHalfOpenLocked(circuit)
			circuit.HalfOpenSuccesses++
		}
	}

	if oldState != circuit.State {
		log.Printf("[CircuitBreaker] %s: %s -> %s (success)", provider, oldState, circuit.State)
	}
}

// RecordFailure records a failed request and potentially opens the circuit
func (cb *CircuitBreaker) RecordFailure(provider string, failure *shared.ProviderFailure) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if this failure type should be excluded
	if failure != nil && cb.isExcludedFailure(failure.Type) {
		log.Printf("[CircuitBreaker] %s: excluding failure type %s from circuit tracking",
			provider, failure.Type)
		return
	}

	circuit := cb.getOrCreateCircuit(provider)
	circuit.TotalRequests++
	circuit.TotalFailures++
	circuit.ConsecutiveFailures++
	now := time.Now()
	circuit.LastFailure = &now

	// Track recent failures
	if failure != nil {
		cf := CircuitFailure{
			Timestamp:    now,
			FailureType:  failure.Type,
			ErrorMessage: failure.Message,
			HTTPCode:     failure.HTTPCode,
		}
		circuit.RecentFailures = append(circuit.RecentFailures, cf)
	}

	// Prune old failures outside the window
	cb.pruneOldFailuresLocked(circuit)

	oldState := circuit.State

	switch circuit.State {
	case CircuitClosed:
		// Check if we should open
		if circuit.ConsecutiveFailures >= cb.config.FailureThreshold {
			cb.transitionToOpenLocked(circuit, "consecutive failures threshold exceeded")
		} else if len(circuit.RecentFailures) >= cb.config.FailureWindowMax {
			cb.transitionToOpenLocked(circuit, "failure window threshold exceeded")
		}

	case CircuitHalfOpen:
		// Any failure in half-open reopens the circuit
		cb.transitionToOpenLocked(circuit, "failure during half-open testing")

	case CircuitOpen:
		// Check if we should transition to half-open
		if circuit.OpenedAt != nil && time.Since(*circuit.OpenedAt) > cb.config.OpenDuration {
			cb.transitionToHalfOpenLocked(circuit)
		}
	}

	if oldState != circuit.State {
		log.Printf("[CircuitBreaker] %s: %s -> %s (failure: %s)",
			provider, oldState, circuit.State, failure.Type)
	}
}

// Reset resets a provider's circuit to closed state
func (cb *CircuitBreaker) Reset(provider string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit, exists := cb.providers[provider]
	if !exists {
		return
	}

	now := time.Now()
	circuit.State = CircuitClosed
	circuit.ConsecutiveFailures = 0
	circuit.HalfOpenRequests = 0
	circuit.HalfOpenSuccesses = 0
	circuit.OpenedAt = nil
	circuit.ClosedAt = &now
	circuit.RecentFailures = []CircuitFailure{}

	log.Printf("[CircuitBreaker] %s: manually reset to closed", provider)
}

// ResetAll resets all provider circuits
func (cb *CircuitBreaker) ResetAll() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.providers = make(map[string]*ProviderCircuit)
	log.Printf("[CircuitBreaker] all circuits reset")
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

func (cb *CircuitBreaker) getOrCreateCircuit(provider string) *ProviderCircuit {
	circuit, exists := cb.providers[provider]
	if !exists {
		circuit = &ProviderCircuit{
			Provider:       provider,
			State:          CircuitClosed,
			RecentFailures: []CircuitFailure{},
		}
		cb.providers[provider] = circuit
	}
	return circuit
}

func (cb *CircuitBreaker) transitionToOpenLocked(circuit *ProviderCircuit, reason string) {
	now := time.Now()
	circuit.State = CircuitOpen
	circuit.OpenedAt = &now
	circuit.HalfOpenRequests = 0
	circuit.HalfOpenSuccesses = 0

	log.Printf("[CircuitBreaker] %s: circuit OPENED - %s (consecutive=%d, recent=%d)",
		circuit.Provider, reason, circuit.ConsecutiveFailures, len(circuit.RecentFailures))
}

func (cb *CircuitBreaker) transitionToHalfOpenLocked(circuit *ProviderCircuit) {
	circuit.State = CircuitHalfOpen
	circuit.HalfOpenRequests = 0
	circuit.HalfOpenSuccesses = 0

	log.Printf("[CircuitBreaker] %s: circuit HALF-OPEN - testing recovery", circuit.Provider)
}

func (cb *CircuitBreaker) transitionToClosedLocked(circuit *ProviderCircuit) {
	now := time.Now()
	circuit.State = CircuitClosed
	circuit.ClosedAt = &now
	circuit.ConsecutiveFailures = 0
	circuit.HalfOpenRequests = 0
	circuit.HalfOpenSuccesses = 0
	circuit.RecentFailures = []CircuitFailure{}

	log.Printf("[CircuitBreaker] %s: circuit CLOSED - recovered", circuit.Provider)
}

func (cb *CircuitBreaker) pruneOldFailuresLocked(circuit *ProviderCircuit) {
	cutoff := time.Now().Add(-cb.config.FailureWindowDuration)

	var recent []CircuitFailure
	for _, f := range circuit.RecentFailures {
		if f.Timestamp.After(cutoff) {
			recent = append(recent, f)
		}
	}
	circuit.RecentFailures = recent
}

func (cb *CircuitBreaker) isExcludedFailure(failureType shared.FailureType) bool {
	for _, excluded := range cb.config.ExcludedFailureTypes {
		if excluded == failureType {
			return true
		}
	}
	return false
}

// =============================================================================
// METRICS AND REPORTING
// =============================================================================

// CircuitBreakerMetrics provides aggregate metrics
type CircuitBreakerMetrics struct {
	TotalProviders   int                        `json:"totalProviders"`
	OpenCircuits     int                        `json:"openCircuits"`
	HalfOpenCircuits int                        `json:"halfOpenCircuits"`
	ClosedCircuits   int                        `json:"closedCircuits"`
	Providers        map[string]ProviderMetrics `json:"providers"`
}

// ProviderMetrics provides per-provider metrics
type ProviderMetrics struct {
	Provider            string       `json:"provider"`
	State               CircuitState `json:"state"`
	TotalRequests       int          `json:"totalRequests"`
	TotalFailures       int          `json:"totalFailures"`
	TotalSuccesses      int          `json:"totalSuccesses"`
	FailureRate         float64      `json:"failureRate"`
	ConsecutiveFailures int          `json:"consecutiveFailures"`
	RecentFailureCount  int          `json:"recentFailureCount"`
}

// GetMetrics returns aggregate circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	metrics := CircuitBreakerMetrics{
		TotalProviders: len(cb.providers),
		Providers:      make(map[string]ProviderMetrics),
	}

	for provider, circuit := range cb.providers {
		switch circuit.State {
		case CircuitOpen:
			metrics.OpenCircuits++
		case CircuitHalfOpen:
			metrics.HalfOpenCircuits++
		case CircuitClosed:
			metrics.ClosedCircuits++
		}

		var failureRate float64
		if circuit.TotalRequests > 0 {
			failureRate = float64(circuit.TotalFailures) / float64(circuit.TotalRequests)
		}

		metrics.Providers[provider] = ProviderMetrics{
			Provider:            provider,
			State:               circuit.State,
			TotalRequests:       circuit.TotalRequests,
			TotalFailures:       circuit.TotalFailures,
			TotalSuccesses:      circuit.TotalSuccesses,
			FailureRate:         failureRate,
			ConsecutiveFailures: circuit.ConsecutiveFailures,
			RecentFailureCount:  len(circuit.RecentFailures),
		}
	}

	return metrics
}
