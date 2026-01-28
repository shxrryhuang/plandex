package model

import (
	"context"
	"fmt"
	"io"
	"log"
	"plandex-server/types"
	shared "plandex-shared"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/sashabaranov/go-openai"
)

type OnStreamFn func(chunk string, buffer string) (shouldStop bool)

// RetryEventCallbacks provides optional callbacks for retry-related events
// These can be used to integrate with journal logging, metrics, or custom monitoring
type RetryEventCallbacks struct {
	// OnRetryAttempt is called before each retry attempt
	OnRetryAttempt func(data *shared.RetryAttemptData)

	// OnRetryExhaust is called when all retries are exhausted
	OnRetryExhaust func(data *shared.RetryExhaustData)

	// OnCircuitEvent is called when circuit breaker state changes
	OnCircuitEvent func(data *shared.CircuitEventData)

	// OnFallbackEvent is called when a fallback is activated
	OnFallbackEvent func(data *shared.FallbackEventData)
}

// retryContextKey is used to store retry callbacks in context
type retryContextKey struct{}

// WithRetryCallbacks adds retry event callbacks to a context
func WithRetryCallbacks(ctx context.Context, callbacks *RetryEventCallbacks) context.Context {
	return context.WithValue(ctx, retryContextKey{}, callbacks)
}

// getRetryCallbacks retrieves retry callbacks from context if available
func getRetryCallbacks(ctx context.Context) *RetryEventCallbacks {
	if callbacks, ok := ctx.Value(retryContextKey{}).(*RetryEventCallbacks); ok {
		return callbacks
	}
	return nil
}

func CreateChatCompletionWithInternalStream(
	clients map[string]ClientInfo,
	authVars map[string]string,
	modelConfig *shared.ModelRoleConfig,
	settings *shared.PlanSettings,
	orgUserConfig *shared.OrgUserConfig,
	currentOrgId string,
	currentUserId string,
	ctx context.Context,
	req types.ExtendedChatCompletionRequest,
	onStream OnStreamFn,
	reqStarted time.Time,
) (*types.ModelResponse, error) {
	providerComposite := modelConfig.GetProviderComposite(authVars, settings, orgUserConfig)
	_, ok := clients[providerComposite]
	if !ok {
		return nil, fmt.Errorf("client not found for provider composite: %s", providerComposite)
	}

	baseModelConfig := modelConfig.GetBaseModelConfig(authVars, settings, orgUserConfig)

	resolveReq(&req, modelConfig, baseModelConfig, settings)

	// choose the fastest provider by latency/throughput on openrouter
	if baseModelConfig.Provider == shared.ModelProviderOpenRouter {
		req.Model += ":nitro"
	}

	// Force streaming mode since we're using the streaming API
	req.Stream = true

	// Include usage in stream response
	req.StreamOptions = &openai.StreamOptions{
		IncludeUsage: true,
	}

	return withStreamingRetries(ctx, func(numTotalRetry int, didProviderFallback bool, modelErr *shared.ModelError) (resp *types.ModelResponse, fallbackRes shared.FallbackResult, err error) {
		handleClaudeMaxRateLimitedIfNeeded(modelErr, modelConfig, authVars, settings, orgUserConfig, currentOrgId, currentUserId)

		fallbackRes = modelConfig.GetFallbackForModelError(numTotalRetry, didProviderFallback, modelErr, authVars, settings, orgUserConfig)
		resolvedModelConfig := fallbackRes.ModelRoleConfig

		if resolvedModelConfig == nil {
			return nil, fallbackRes, fmt.Errorf("model config is nil")
		}

		providerComposite := resolvedModelConfig.GetProviderComposite(authVars, settings, orgUserConfig)
		opClient, ok := clients[providerComposite]

		if !ok {
			return nil, fallbackRes, fmt.Errorf("client not found for provider composite: %s", providerComposite)
		}

		modelConfig = resolvedModelConfig
		resp, err = processChatCompletionStream(resolvedModelConfig, opClient, authVars, settings, orgUserConfig, ctx, req, onStream, reqStarted)
		if err != nil {
			return nil, fallbackRes, err
		}
		return resp, fallbackRes, nil
	}, func(resp *types.ModelResponse, err error) {
		if resp != nil {
			resp.Stopped = true
			resp.Error = err.Error()
		}
	})
}

func processChatCompletionStream(
	modelConfig *shared.ModelRoleConfig,
	client ClientInfo,
	authVars map[string]string,
	settings *shared.PlanSettings,
	orgUserConfig *shared.OrgUserConfig,
	ctx context.Context,
	req types.ExtendedChatCompletionRequest,
	onStream OnStreamFn,
	reqStarted time.Time,
) (*types.ModelResponse, error) {
	streamCtx, cancel := context.WithCancel(ctx)

	log.Println("processChatCompletionStream - modelConfig", spew.Sdump(map[string]interface{}{
		"model": modelConfig.ModelId,
	}))

	stream, err := createChatCompletionStreamExtended(modelConfig, client, authVars, settings, orgUserConfig, streamCtx, req)

	if err != nil {
		cancel()
		return nil, fmt.Errorf("error creating chat completion stream: %w", err)
	}

	defer stream.Close()
	defer cancel()

	accumulator := types.NewStreamCompletionAccumulator()
	// Create a timer that will trigger if no chunk is received within the specified duration
	timer := time.NewTimer(ACTIVE_STREAM_CHUNK_TIMEOUT)
	defer timer.Stop()
	streamFinished := false

	receivedFirstChunk := false

	// Process stream until EOF or error
	for {
		select {
		case <-streamCtx.Done():
			log.Println("Stream canceled")
			return accumulator.Result(true, streamCtx.Err()), streamCtx.Err()
		case <-timer.C:
			log.Println("Stream timed out due to inactivity")
			if streamFinished {
				log.Println("Stream finished—timed out waiting for usage chunk")
				return accumulator.Result(false, nil), nil
			} else {
				log.Println("Stream timed out due to inactivity")
				return accumulator.Result(true, fmt.Errorf("stream timed out due to inactivity. The model is not responding.")), nil
			}
		default:
			response, err := stream.Recv()
			if err == io.EOF {
				if streamFinished {
					return accumulator.Result(false, nil), nil
				}

				err = fmt.Errorf("model stream ended unexpectedly: %w", err)
				return accumulator.Result(true, err), err
			}
			if err != nil {
				err = fmt.Errorf("error receiving stream chunk: %w", err)
				return accumulator.Result(true, err), err
			}

			if response.ID != "" {
				accumulator.SetGenerationId(response.ID)
			}

			if !receivedFirstChunk {
				receivedFirstChunk = true
				accumulator.SetFirstTokenAt(time.Now())
			}

			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(ACTIVE_STREAM_CHUNK_TIMEOUT)

			// Process the response
			if response.Usage != nil {
				accumulator.SetUsage(response.Usage)
				return accumulator.Result(false, nil), nil
			}

			emptyChoices := false
			var content string

			if len(response.Choices) == 0 {
				// Previously we'd return an error if there were no choices, but some models do this and then keep streaming, so we'll just log it and continue
				log.Println("processChatCompletionStream - no choices in response")
				// err := fmt.Errorf("no choices in response")
				// return accumulator.Result(false, err), err
				emptyChoices = true
			}

			// We'll be more accepting of multiple choices and just take the first one
			// if len(response.Choices) > 1 {
			// 	err = fmt.Errorf("stream finished with more than one choice | The model failed to generate a valid response.")
			// 	return accumulator.Result(true, err), err
			// }

			if !emptyChoices {
				choice := response.Choices[0]

				if choice.FinishReason != "" {
					if choice.FinishReason == "error" {
						err = fmt.Errorf("model stopped with error status | The model is not responding.")
						return accumulator.Result(true, err), err
					} else {
						// Reset the timer for the usage chunk
						if !timer.Stop() {
							select {
							case <-timer.C:
							default:
							}
						}
						timer.Reset(USAGE_CHUNK_TIMEOUT)
						streamFinished = true
						continue
					}
				}

				if req.Tools != nil {
					if choice.Delta.ToolCalls != nil {
						toolCall := choice.Delta.ToolCalls[0]
						content = toolCall.Function.Arguments
					}
				} else {
					if choice.Delta.Content != "" {
						content = choice.Delta.Content
					}
				}
			}

			accumulator.AddContent(content)
			// pass the chunk and the accumulated content to the callback
			if onStream != nil {
				shouldReturn := onStream(content, accumulator.Content())
				if shouldReturn {
					return accumulator.Result(false, nil), nil
				}
			}
		}
	}
}

// withStreamingRetries executes an operation with configurable retry logic,
// exponential backoff, and circuit breaker integration.
//
// The function handles:
// - Exponential backoff with jitter based on failure type
// - Retry-After header respect
// - Circuit breaker checks for provider health
// - Fallback to alternative models/providers
// - Comprehensive logging for debugging
func withStreamingRetries[T any](
	ctx context.Context,
	operation func(numRetry int, didProviderFallback bool, modelErr *shared.ModelError) (resp *T, fallbackRes shared.FallbackResult, err error),
	onContextDone func(resp *T, err error),
) (*T, error) {
	var resp *T
	var numTotalRetry int
	var numFallbackRetry int
	var fallbackRes shared.FallbackResult
	var modelErr *shared.ModelError
	var didProviderFallback bool

	// Track retry state for logging and metrics (HEAD: circuit breaker + callbacks)
	startTime := time.Now()
	var currentProvider string
	var currentModel string
	var failureTypes []string

	// Get retry callbacks from context for journal integration
	callbacks := getRetryCallbacks(ctx)

	// Build a RetryContext to track every attempt with structured metadata (our commit)
	retryCtx := shared.NewRetryContext("model_request", defaultRetryConfig)

	for {
		// Check for context cancellation
		if ctx.Err() != nil {
			if resp != nil {
				onContextDone(resp, ctx.Err())
				return resp, ctx.Err()
			}
			return nil, ctx.Err()
		}

		// Safety guard: refuse to retry irreversible operations.
		if !shared.IsOperationSafe(retryCtx.Safety, retryCtx.Config) {
			log.Printf("withStreamingRetries - operation safety is %s; retry blocked by config", retryCtx.Safety)
			if resp != nil {
				onContextDone(resp, fmt.Errorf("operation is %s; retry not permitted", retryCtx.Safety))
			}
			return resp, fmt.Errorf("operation is %s; retry not permitted", retryCtx.Safety)
		}

		var numRetry int
		if numFallbackRetry > 0 {
			numRetry = numFallbackRetry
		} else {
			numRetry = numTotalRetry
		}
		_ = numRetry // used implicitly via numTotalRetry below

		// Region 2 (merged): HEAD's logging style + our RecordAttemptStart
		log.Printf("[Retry] Attempt %d (total=%d, fallback=%d, provider_fallback=%v)",
			numRetry+1, numTotalRetry, numFallbackRetry, didProviderFallback)

		// Record the start of this attempt (our commit tracking)
		attemptIdx := retryCtx.RecordAttemptStart()

		// Execute the operation
		var err error
		resp, fallbackRes, err = operation(numTotalRetry, didProviderFallback, modelErr)

		// Track current provider and model for circuit breaker and logging
		if fallbackRes.BaseModelConfig != nil {
			currentProvider = string(fallbackRes.BaseModelConfig.Provider)
			currentModel = string(fallbackRes.BaseModelConfig.ModelName)
		}

		// Success - record in circuit breaker (HEAD) and RetryContext (our commit), then return
		if err == nil {
			if GlobalCircuitBreaker != nil && currentProvider != "" {
				GlobalCircuitBreaker.RecordSuccess(currentProvider)
			}
			retryCtx.RecordAttemptSuccess(attemptIdx)
			log.Printf("[Retry] Success after %d attempts (duration=%v, summary=%s)", numTotalRetry+1, time.Since(startTime), retryCtx.Summary())
			return resp, nil
		}

		log.Printf("[Retry] Operation failed: %v", err)

		// Determine retry limits based on fallback state
		isFallback := fallbackRes.IsFallback
		maxRetries := MAX_RETRIES_WITHOUT_FALLBACK
		if isFallback {
			maxRetries = MAX_ADDITIONAL_RETRIES_WITH_FALLBACK
		}

		if fallbackRes.FallbackType == shared.FallbackTypeProvider {
			didProviderFallback = true
		}

		compareRetries := numTotalRetry
		if isFallback {
			compareRetries = numFallbackRetry
		}

		// Region 3 (merged): Classification + circuit breaker recording + our strategy/unrecoverable/StoreWithContext

		log.Printf("Error in streaming operation: %v, isFallback: %t, numTotalRetry: %d, numFallbackRetry: %d, compareRetries: %d, maxRetries: %d\n",
			err, isFallback, numTotalRetry, numFallbackRetry, compareRetries, maxRetries)

		// Classify the error using unified classification
		classifyRes := classifyBasicError(err, fallbackRes.BaseModelConfig.HasClaudeMaxAuth)
		modelErr = &classifyRes

		// Also get the ProviderFailure for circuit breaker and detailed logging
		providerFailure := classifyErrorToProviderFailure(err, currentProvider)

		// Record failure in circuit breaker
		if GlobalCircuitBreaker != nil && currentProvider != "" && providerFailure != nil {
			GlobalCircuitBreaker.RecordFailure(currentProvider, providerFailure)
		}

		// Track failure type for logging
		if providerFailure != nil {
			failureTypes = append(failureTypes, string(providerFailure.Type))
		}

		// Resolve the strategy for this specific failure type (our commit)
		strategy := retryCtx.Config.GetStrategy(modelErr.ProviderFailureType)

		log.Printf("[Retry] Error classified: kind=%s, retriable=%v, type=%s, provider=%s",
			modelErr.Kind, modelErr.Retriable, providerFailure.Type, currentProvider)

		// Handle non-retryable errors
		newFallback := false
		if !modelErr.Retriable {
			log.Printf("[Retry] Non-retriable error (type=%s): %v", modelErr.ProviderFailureType, err)
			spew.Dump(modelErr)

			// Check for unrecoverable condition before giving up (our commit)
			failure := &shared.ProviderFailure{
				Type:      modelErr.ProviderFailureType,
				Category:  shared.FailureCategoryNonRetryable,
				Message:   err.Error(),
				Retryable: false,
			}
			report := shared.ErrorReportFromProviderFailure(failure, nil)
			if unrecov := shared.DetectUnrecoverableCondition(report); unrecov != nil {
				log.Printf("[Retry] Unrecoverable condition detected: %s", unrecov.Reason)
				retryCtx.Unrecoverable = unrecov
				shared.StoreWithContext(report, retryCtx)
				retryCtx.RecordAttemptFailure(attemptIdx, modelErr, &strategy, 0, isFallback, fallbackRes.FallbackType)
				return resp, fmt.Errorf("unrecoverable error (%s): %w", unrecov.Reason, err)
			}

			// Check for large context fallback
			if modelErr.Kind == shared.ErrContextTooLong && fallbackRes.ModelRoleConfig.LargeContextFallback == nil {
				log.Printf("[Retry] Context too long with no fallback - failing")
				shared.StoreWithContext(report, retryCtx)
				retryCtx.RecordAttemptFailure(attemptIdx, modelErr, &strategy, 0, isFallback, fallbackRes.FallbackType)
				return resp, err
			}

			// Check for error fallback
			if modelErr.Kind != shared.ErrContextTooLong && fallbackRes.ModelRoleConfig.ErrorFallback == nil {
				log.Printf("[Retry] Non-retriable error with no fallback - failing")
				shared.StoreWithContext(report, retryCtx)
				retryCtx.RecordAttemptFailure(attemptIdx, modelErr, &strategy, 0, isFallback, fallbackRes.FallbackType)
				return resp, err
			}

			// Has fallback - reset fallback retry counter
			log.Printf("[Retry] Non-retriable error but fallback available - switching to fallback")
			numFallbackRetry = 0
			newFallback = true
			compareRetries = 0
		}

		// Region 4 (merged): Check if retries exhausted with StoreWithContext,
		// then circuit breaker open check with fallback, then ComputeBackoffDelay

		// Check if retries exhausted
		if compareRetries >= maxRetries {
			log.Printf("[Retry] Max retries reached (%d/%d) - failing", compareRetries, maxRetries)

			// Store error with context before giving up (our commit)
			failure := &shared.ProviderFailure{
				Type:      modelErr.ProviderFailureType,
				Category:  shared.FailureCategoryRetryable,
				Message:   err.Error(),
				Retryable: true,
			}
			report := shared.ErrorReportFromProviderFailure(failure, nil)
			shared.StoreWithContext(report, retryCtx)
			retryCtx.RecordAttemptFailure(attemptIdx, modelErr, &strategy, 0, isFallback, fallbackRes.FallbackType)

			// Notify via callback for journal logging (HEAD)
			if callbacks != nil && callbacks.OnRetryExhaust != nil {
				callbacks.OnRetryExhaust(&shared.RetryExhaustData{
					TotalAttempts:   numTotalRetry + 1,
					TotalDurationMs: time.Since(startTime).Milliseconds(),
					FailureTypes:    failureTypes,
					FinalError:      err.Error(),
					Provider:        currentProvider,
					Model:           currentModel,
					FallbackUsed:    isFallback,
					FallbackType:    string(fallbackRes.FallbackType),
					Resolution:      "failed",
				})
			}

			return resp, err
		}

		// Check circuit breaker before retry (HEAD)
		if GlobalCircuitBreaker != nil && currentProvider != "" {
			if GlobalCircuitBreaker.IsOpen(currentProvider) {
				log.Printf("[Retry] Circuit breaker OPEN for %s - attempting fallback", currentProvider)
				// If circuit is open and we haven't tried provider fallback, do it now
				if !didProviderFallback && fallbackRes.ModelRoleConfig != nil {
					provFallback := fallbackRes.ModelRoleConfig.GetProviderFallback(nil, nil, nil)
					if provFallback != nil {
						oldProvider := currentProvider
						oldModel := currentModel

						didProviderFallback = true
						newFallback = true
						numFallbackRetry = 0
						log.Printf("[Retry] Switching to provider fallback due to circuit breaker")

						// Notify via callback for journal logging
						if callbacks != nil && callbacks.OnFallbackEvent != nil {
							callbacks.OnFallbackEvent(&shared.FallbackEventData{
								FromProvider: oldProvider,
								ToProvider:   "(provider_fallback)", // Actual provider determined on next operation
								FromModel:    oldModel,
								ToModel:      string(provFallback.ModelId),
								FallbackType: string(shared.FallbackTypeProvider),
								Reason:       "circuit breaker open",
								FailureType:  string(providerFailure.Type),
								ErrorMessage: err.Error(),
								Success:      true, // Indicates fallback was activated
							})
						}
					}
				}
			}
		}

		// Compute delay using the per-type strategy (our commit: ComputeBackoffDelay is primary)
		var retryDelay time.Duration
		retryAfterHint := time.Duration(modelErr.RetryAfterSeconds) * time.Second

		if modelErr.RetryAfterSeconds > 0 {
			// Provider declared a retry-after.  Check if it's within our cap.
			if !retryCtx.Config.IsProviderRetryAfterAcceptable(modelErr.RetryAfterSeconds) {
				log.Printf("[Retry] Provider retry-after (%ds) exceeds config cap; treating as non-retryable",
					modelErr.RetryAfterSeconds)
				retryCtx.RecordAttemptFailure(attemptIdx, modelErr, &strategy, 0, isFallback, fallbackRes.FallbackType)
				return resp, fmt.Errorf("provider retry-after (%ds) exceeds maximum wait (%dms): %w",
					modelErr.RetryAfterSeconds, retryCtx.Config.MaxProviderRetryAfterMs, err)
			}
		}

		retryDelay = retryCtx.Config.ComputeBackoffDelay(strategy, numTotalRetry, modelErr.RetryAfterSeconds)
		delayMs := retryDelay.Milliseconds()

		log.Printf("[Retry] Retrying in %v (strategy: type=%s, multiplier=%.1f, jitter=%t, retryAfterHint=%v)",
			retryDelay, modelErr.ProviderFailureType, strategy.BackoffMultiplier, strategy.UseJitter, retryAfterHint)

		retryCtx.RecordAttemptFailure(attemptIdx, modelErr, &strategy, delayMs, isFallback, fallbackRes.FallbackType)

		// Notify via callback for journal logging (HEAD)
		willRetry := modelErr.Retriable || newFallback
		if callbacks != nil && callbacks.OnRetryAttempt != nil {
			callbacks.OnRetryAttempt(&shared.RetryAttemptData{
				AttemptNumber: numTotalRetry + 1,
				TotalAttempts: numTotalRetry + 1,
				FailureType:   string(providerFailure.Type),
				ErrorMessage:  err.Error(),
				HTTPCode:      providerFailure.HTTPCode,
				Provider:      currentProvider,
				Model:         currentModel,
				PolicyUsed:    string(modelErr.ProviderFailureType),
				DelayMs:       delayMs,
				WillRetry:     willRetry,
				Retryable:     modelErr.Retriable,
			})
		}

		// Wait with context awareness
		select {
		case <-ctx.Done():
			if resp != nil {
				onContextDone(resp, ctx.Err())
				return resp, ctx.Err()
			}
			return nil, ctx.Err()
		case <-time.After(retryDelay):
			// Continue to next retry
		}

		// Increment retry counters
		if modelErr != nil && modelErr.ShouldIncrementRetry() {
			numTotalRetry++
			if isFallback && !newFallback {
				numFallbackRetry++
			}
		}
	}
}
