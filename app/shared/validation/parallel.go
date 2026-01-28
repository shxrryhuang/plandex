package validation

import (
	"context"
	"sync"
)

// ParallelValidator runs validation checks concurrently
type ParallelValidator struct {
	options ValidationOptions
}

// NewParallelValidator creates a parallel validator
func NewParallelValidator(opts ValidationOptions) *ParallelValidator {
	return &ParallelValidator{
		options: opts,
	}
}

// ValidationTask represents a single validation task
type ValidationTask struct {
	Name     string
	Execute  func(ctx context.Context) *ValidationResult
	Priority int // Higher priority runs first
}

// ValidateAllParallel runs all validation checks in parallel
func (pv *ParallelValidator) ValidateAllParallel(ctx context.Context) *ValidationResult {
	result := &ValidationResult{}

	// Create validation tasks
	tasks := pv.createTasks()

	// Run tasks concurrently
	resultsCh := make(chan *ValidationResult, len(tasks))
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		go func(t ValidationTask) {
			defer wg.Done()

			// Check context cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Run task
			taskResult := t.Execute(ctx)
			resultsCh <- taskResult
		}(task)
	}

	// Wait for all tasks to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	for taskResult := range resultsCh {
		if taskResult != nil {
			result.Merge(taskResult)
		}
	}

	return result
}

// createTasks creates validation tasks based on options
func (pv *ParallelValidator) createTasks() []ValidationTask {
	var tasks []ValidationTask

	// Database validation
	if !pv.options.SkipDatabase {
		tasks = append(tasks, ValidationTask{
			Name:     "database",
			Priority: 100, // High priority
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateDatabase(ctx)
			},
		})
	}

	// Environment validation
	if !pv.options.SkipEnvironment {
		tasks = append(tasks, ValidationTask{
			Name:     "environment",
			Priority: 90,
			Execute: func(ctx context.Context) *ValidationResult {
				return ValidateEnvironment()
			},
		})
	}

	// LiteLLM validation
	if !pv.options.SkipLiteLLM {
		if pv.options.Phase == PhaseStartup {
			tasks = append(tasks, ValidationTask{
				Name:     "litellm-proxy",
				Priority: 80,
				Execute: func(ctx context.Context) *ValidationResult {
					return ValidateLiteLLMProxy(ctx)
				},
			})
		} else {
			tasks = append(tasks, ValidationTask{
				Name:     "litellm-health",
				Priority: 80,
				Execute: func(ctx context.Context) *ValidationResult {
					return ValidateLiteLLMProxyHealth(ctx)
				},
			})
		}
	}

	// Provider validation
	if !pv.options.SkipProvider {
		if len(pv.options.ProviderNames) > 0 {
			// Validate specific providers (can be done in parallel)
			for _, provider := range pv.options.ProviderNames {
				providerName := provider // Capture for closure
				tasks = append(tasks, ValidationTask{
					Name:     "provider-" + providerName,
					Priority: 70,
					Execute: func(ctx context.Context) *ValidationResult {
						return ValidateProviderCredentials(providerName, pv.options.CheckFileAccess)
					},
				})
			}
		} else {
			// Validate all providers (done as one task since it's already optimized)
			tasks = append(tasks, ValidationTask{
				Name:     "providers-all",
				Priority: 70,
				Execute: func(ctx context.Context) *ValidationResult {
					return ValidateAllProviders(pv.options.CheckFileAccess)
				},
			})
		}
	}

	// Config file validation
	tasks = append(tasks, ValidationTask{
		Name:     "config-files",
		Priority: 60,
		Execute: func(ctx context.Context) *ValidationResult {
			return ValidateAllConfigFiles()
		},
	})

	return tasks
}

// ValidateWithConcurrency runs validation with controlled concurrency
func ValidateWithConcurrency(ctx context.Context, opts ValidationOptions, maxConcurrency int) *ValidationResult {
	pv := NewParallelValidator(opts)
	tasks := pv.createTasks()

	result := &ValidationResult{}
	resultsCh := make(chan *ValidationResult, len(tasks))
	taskCh := make(chan ValidationTask, len(tasks))

	// Worker pool
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				select {
				case <-ctx.Done():
					return
				default:
					taskResult := task.Execute(ctx)
					resultsCh <- taskResult
				}
			}
		}()
	}

	// Send tasks to workers
	go func() {
		for _, task := range tasks {
			taskCh <- task
		}
		close(taskCh)
	}()

	// Wait for completion
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	for taskResult := range resultsCh {
		if taskResult != nil {
			result.Merge(taskResult)
		}
	}

	return result
}

// FastValidation runs only fast, critical checks in parallel
func FastValidation(ctx context.Context) *ValidationResult {
	opts := ValidationOptions{
		Phase:           PhaseStartup,
		CheckFileAccess: false,
		Verbose:         false,
		Timeout:         10,
		SkipLiteLLM:     true, // Skip network checks for speed
	}

	pv := NewParallelValidator(opts)
	return pv.ValidateAllParallel(ctx)
}

// ThoroughValidation runs all checks including slow ones in parallel
func ThoroughValidation(ctx context.Context) *ValidationResult {
	opts := ValidationOptions{
		Phase:           PhaseExecution,
		CheckFileAccess: true,
		Verbose:         true,
		Timeout:         30,
	}

	pv := NewParallelValidator(opts)
	return pv.ValidateAllParallel(ctx)
}
