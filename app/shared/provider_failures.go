package shared

import "strings"

// =============================================================================
// PROVIDER FAILURE CLASSIFICATION
// =============================================================================
//
// This file classifies AI provider failures into retryable vs non-retryable
// categories with concrete examples from each major provider.
//
// =============================================================================

// FailureCategory represents the broad category of a provider failure
type FailureCategory string

const (
	// FailureCategoryRetryable - Failures that may succeed on retry
	FailureCategoryRetryable FailureCategory = "retryable"

	// FailureCategoryNonRetryable - Failures that will never succeed on retry
	FailureCategoryNonRetryable FailureCategory = "non_retryable"

	// FailureCategoryConditional - Failures that may be retryable depending on context
	FailureCategoryConditional FailureCategory = "conditional"
)

// FailureType represents specific types of provider failures
type FailureType string

const (
	// ==========================================================================
	// RETRYABLE FAILURES
	// ==========================================================================

	// FailureTypeRateLimit - Too many requests (HTTP 429)
	// Retry: Yes, with exponential backoff respecting Retry-After header
	FailureTypeRateLimit FailureType = "rate_limit"

	// FailureTypeOverloaded - Server is temporarily overloaded (HTTP 503, 529)
	// Retry: Yes, with exponential backoff
	FailureTypeOverloaded FailureType = "overloaded"

	// FailureTypeServerError - Temporary server error (HTTP 500, 502, 503, 504)
	// Retry: Yes, with exponential backoff
	FailureTypeServerError FailureType = "server_error"

	// FailureTypeTimeout - Request timed out
	// Retry: Yes, possibly with longer timeout
	FailureTypeTimeout FailureType = "timeout"

	// FailureTypeConnectionError - Network connectivity issues
	// Retry: Yes, after brief delay
	FailureTypeConnectionError FailureType = "connection_error"

	// FailureTypeStreamInterrupted - Streaming response was interrupted
	// Retry: Yes, from the beginning or checkpoint
	FailureTypeStreamInterrupted FailureType = "stream_interrupted"

	// FailureTypeCacheError - Cache-related error (e.g., cache control not supported)
	// Retry: Yes, without cache parameters
	FailureTypeCacheError FailureType = "cache_error"

	// ==========================================================================
	// NON-RETRYABLE FAILURES
	// ==========================================================================

	// FailureTypeAuthInvalid - Invalid API key or credentials (HTTP 401)
	// Retry: No, requires user action to fix credentials
	FailureTypeAuthInvalid FailureType = "auth_invalid"

	// FailureTypePermissionDenied - Valid auth but insufficient permissions (HTTP 403)
	// Retry: No, requires permission changes
	FailureTypePermissionDenied FailureType = "permission_denied"

	// FailureTypeContextTooLong - Input exceeds model's context window (HTTP 400, 413)
	// Retry: No, requires reducing input size
	FailureTypeContextTooLong FailureType = "context_too_long"

	// FailureTypeInvalidRequest - Malformed request (HTTP 400)
	// Retry: No, requires fixing request format
	FailureTypeInvalidRequest FailureType = "invalid_request"

	// FailureTypeContentPolicy - Content violates provider's policy (HTTP 400)
	// Retry: No, requires modifying content
	FailureTypeContentPolicy FailureType = "content_policy"

	// FailureTypeQuotaExhausted - Account quota permanently exhausted (HTTP 429 with specific message)
	// Retry: No, requires upgrading plan or waiting for quota reset
	FailureTypeQuotaExhausted FailureType = "quota_exhausted"

	// FailureTypeModelNotFound - Requested model doesn't exist (HTTP 404)
	// Retry: No, requires using a valid model ID
	FailureTypeModelNotFound FailureType = "model_not_found"

	// FailureTypeModelDeprecated - Model is deprecated/retired
	// Retry: No, requires using a newer model
	FailureTypeModelDeprecated FailureType = "model_deprecated"

	// FailureTypeUnsupportedFeature - Feature not supported (HTTP 501)
	// Retry: No, requires changing approach
	FailureTypeUnsupportedFeature FailureType = "unsupported_feature"

	// FailureTypeAccountSuspended - Account has been suspended
	// Retry: No, requires contacting provider
	FailureTypeAccountSuspended FailureType = "account_suspended"

	// ==========================================================================
	// CONDITIONAL FAILURES (depends on context)
	// ==========================================================================

	// FailureTypeBillingError - Billing/payment issue
	// Retry: Conditional, may resolve if payment processed
	FailureTypeBillingError FailureType = "billing_error"

	// FailureTypeProviderUnavailable - Specific provider unavailable
	// Retry: Conditional, can fallback to different provider
	FailureTypeProviderUnavailable FailureType = "provider_unavailable"
)

// ProviderFailure represents a classified provider failure
type ProviderFailure struct {
	// Type identifies the specific failure type
	Type FailureType `json:"type"`

	// Category indicates if failure is retryable
	Category FailureCategory `json:"category"`

	// HTTPCode is the HTTP status code (0 if not applicable)
	HTTPCode int `json:"httpCode"`

	// Message is the error message
	Message string `json:"message"`

	// Provider identifies which provider returned the error
	Provider string `json:"provider"`

	// Retryable indicates if the failure should be retried
	Retryable bool `json:"retryable"`

	// RetryAfterSeconds is the suggested wait time before retry (0 = use backoff)
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`

	// MaxRetries is the suggested maximum retry attempts (0 = use default)
	MaxRetries int `json:"maxRetries,omitempty"`

	// RequiresAction describes user action needed for non-retryable failures
	RequiresAction string `json:"requiresAction,omitempty"`

	// FallbackSuggested indicates a provider fallback might help
	FallbackSuggested bool `json:"fallbackSuggested,omitempty"`
}

// =============================================================================
// CONCRETE EXAMPLES BY PROVIDER
// =============================================================================

// ProviderFailureExample documents a real-world failure example
type ProviderFailureExample struct {
	Provider    string          `json:"provider"`
	Type        FailureType     `json:"type"`
	Category    FailureCategory `json:"category"`
	HTTPCode    int             `json:"httpCode"`
	ErrorCode   string          `json:"errorCode,omitempty"`
	Message     string          `json:"message"`
	RawResponse string          `json:"rawResponse,omitempty"`
	Retryable   bool            `json:"retryable"`
	Notes       string          `json:"notes,omitempty"`
}

// GetProviderFailureExamples returns documented examples of provider failures
func GetProviderFailureExamples() []ProviderFailureExample {
	return []ProviderFailureExample{
		// =====================================================================
		// OPENAI EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:    "openai",
			Type:        FailureTypeRateLimit,
			Category:    FailureCategoryRetryable,
			HTTPCode:    429,
			ErrorCode:   "rate_limit_exceeded",
			Message:     "Rate limit reached for gpt-4 in organization org-xxx on requests per min (RPM): Limit 10, Used 10, Requested 1.",
			RawResponse: `{"error":{"message":"Rate limit reached for gpt-4...","type":"requests","param":null,"code":"rate_limit_exceeded"}}`,
			Retryable:   true,
			Notes:       "Respect Retry-After header. Use exponential backoff. Usually clears in seconds to minutes.",
		},
		{
			Provider:    "openai",
			Type:        FailureTypeServerError,
			Category:    FailureCategoryRetryable,
			HTTPCode:    500,
			ErrorCode:   "server_error",
			Message:     "The server had an error while processing your request. Sorry about that!",
			RawResponse: `{"error":{"message":"The server had an error...","type":"server_error","param":null,"code":"server_error"}}`,
			Retryable:   true,
			Notes:       "Temporary OpenAI infrastructure issue. Retry with backoff.",
		},
		{
			Provider:  "openai",
			Type:      FailureTypeOverloaded,
			Category:  FailureCategoryRetryable,
			HTTPCode:  503,
			ErrorCode: "overloaded",
			Message:   "The engine is currently overloaded, please try again later.",
			Retryable: true,
			Notes:     "High demand period. Retry after 5-30 seconds.",
		},
		{
			Provider:  "openai",
			Type:      FailureTypeTimeout,
			Category:  FailureCategoryRetryable,
			HTTPCode:  504,
			ErrorCode: "gateway_timeout",
			Message:   "Request timed out. Please try again.",
			Retryable: true,
			Notes:     "Long-running request exceeded timeout. Consider streaming or smaller input.",
		},

		// NON-RETRYABLE
		{
			Provider:    "openai",
			Type:        FailureTypeAuthInvalid,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    401,
			ErrorCode:   "invalid_api_key",
			Message:     "Incorrect API key provided: sk-xxx. You can find your API key at https://platform.openai.com/account/api-keys.",
			RawResponse: `{"error":{"message":"Incorrect API key provided...","type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`,
			Retryable:   false,
			Notes:       "User must provide valid API key.",
		},
		{
			Provider:    "openai",
			Type:        FailureTypeContextTooLong,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    400,
			ErrorCode:   "context_length_exceeded",
			Message:     "This model's maximum context length is 8192 tokens. However, your messages resulted in 12847 tokens. Please reduce the length of the messages.",
			RawResponse: `{"error":{"message":"This model's maximum context length is 8192 tokens...","type":"invalid_request_error","param":"messages","code":"context_length_exceeded"}}`,
			Retryable:   false,
			Notes:       "Must reduce input size or use model with larger context window.",
		},
		{
			Provider:    "openai",
			Type:        FailureTypeQuotaExhausted,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    429,
			ErrorCode:   "insufficient_quota",
			Message:     "You exceeded your current quota, please check your plan and billing details.",
			RawResponse: `{"error":{"message":"You exceeded your current quota...","type":"insufficient_quota","param":null,"code":"insufficient_quota"}}`,
			Retryable:   false,
			Notes:       "Different from rate limit. User must add credits or upgrade plan.",
		},
		{
			Provider:  "openai",
			Type:      FailureTypeContentPolicy,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  400,
			ErrorCode: "content_policy_violation",
			Message:   "Your request was rejected as a result of our safety system. Your prompt may contain text that is not allowed by our safety system.",
			Retryable: false,
			Notes:     "Content flagged by moderation. Must modify the prompt.",
		},
		{
			Provider:    "openai",
			Type:        FailureTypeModelNotFound,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    404,
			ErrorCode:   "model_not_found",
			Message:     "The model `gpt-5` does not exist",
			RawResponse: `{"error":{"message":"The model 'gpt-5' does not exist","type":"invalid_request_error","param":"model","code":"model_not_found"}}`,
			Retryable:   false,
			Notes:       "Model ID is invalid. Check available models.",
		},
		{
			Provider:    "openai",
			Type:        FailureTypeInvalidRequest,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    400,
			ErrorCode:   "invalid_request_error",
			Message:     "'messages' is a required property",
			RawResponse: `{"error":{"message":"'messages' is a required property","type":"invalid_request_error","param":"messages","code":null}}`,
			Retryable:   false,
			Notes:       "Request schema validation failed. Fix request format.",
		},

		// =====================================================================
		// ANTHROPIC EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:    "anthropic",
			Type:        FailureTypeRateLimit,
			Category:    FailureCategoryRetryable,
			HTTPCode:    429,
			ErrorCode:   "rate_limit_error",
			Message:     "Number of request tokens has exceeded your per-minute rate limit",
			RawResponse: `{"type":"error","error":{"type":"rate_limit_error","message":"Number of request tokens has exceeded your per-minute rate limit"}}`,
			Retryable:   true,
			Notes:       "Check retry-after header. Usually 60 seconds or less.",
		},
		{
			Provider:    "anthropic",
			Type:        FailureTypeOverloaded,
			Category:    FailureCategoryRetryable,
			HTTPCode:    529,
			ErrorCode:   "overloaded_error",
			Message:     "Anthropic's API is temporarily overloaded",
			RawResponse: `{"type":"error","error":{"type":"overloaded_error","message":"Anthropic's API is temporarily overloaded"}}`,
			Retryable:   true,
			Notes:       "Anthropic-specific 529 code. Retry after 10-60 seconds.",
		},
		{
			Provider:    "anthropic",
			Type:        FailureTypeServerError,
			Category:    FailureCategoryRetryable,
			HTTPCode:    500,
			ErrorCode:   "api_error",
			Message:     "An unexpected error has occurred internal to Anthropic's systems",
			RawResponse: `{"type":"error","error":{"type":"api_error","message":"An unexpected error has occurred internal to Anthropic's systems"}}`,
			Retryable:   true,
			Notes:       "Internal Anthropic error. Retry with exponential backoff.",
		},

		// NON-RETRYABLE
		{
			Provider:    "anthropic",
			Type:        FailureTypeAuthInvalid,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    401,
			ErrorCode:   "authentication_error",
			Message:     "Invalid API Key",
			RawResponse: `{"type":"error","error":{"type":"authentication_error","message":"Invalid API Key"}}`,
			Retryable:   false,
			Notes:       "API key is invalid or expired.",
		},
		{
			Provider:    "anthropic",
			Type:        FailureTypePermissionDenied,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    403,
			ErrorCode:   "permission_error",
			Message:     "Your API key does not have permission to use the specified resource",
			RawResponse: `{"type":"error","error":{"type":"permission_error","message":"Your API key does not have permission to use the specified resource"}}`,
			Retryable:   false,
			Notes:       "API key lacks permissions for requested model/feature.",
		},
		{
			Provider:    "anthropic",
			Type:        FailureTypeContextTooLong,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    400,
			ErrorCode:   "invalid_request_error",
			Message:     "prompt is too long: 234567 tokens > 200000 maximum",
			RawResponse: `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 234567 tokens > 200000 maximum"}}`,
			Retryable:   false,
			Notes:       "Must reduce input or use a model with larger context.",
		},
		{
			Provider:    "anthropic",
			Type:        FailureTypeInvalidRequest,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    400,
			ErrorCode:   "invalid_request_error",
			Message:     "messages: roles must alternate between \"user\" and \"assistant\"",
			RawResponse: `{"type":"error","error":{"type":"invalid_request_error","message":"messages: roles must alternate between \"user\" and \"assistant\""}}`,
			Retryable:   false,
			Notes:       "Message format invalid. Fix conversation structure.",
		},

		// =====================================================================
		// GOOGLE (VERTEX AI / GEMINI) EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:    "google",
			Type:        FailureTypeRateLimit,
			Category:    FailureCategoryRetryable,
			HTTPCode:    429,
			ErrorCode:   "RESOURCE_EXHAUSTED",
			Message:     "Quota exceeded for aiplatform.googleapis.com/generate_content_requests_per_minute",
			RawResponse: `{"error":{"code":429,"message":"Quota exceeded for aiplatform.googleapis.com/generate_content_requests_per_minute","status":"RESOURCE_EXHAUSTED"}}`,
			Retryable:   true,
			Notes:       "Per-minute quota. Wait and retry.",
		},
		{
			Provider:    "google",
			Type:        FailureTypeServerError,
			Category:    FailureCategoryRetryable,
			HTTPCode:    503,
			ErrorCode:   "UNAVAILABLE",
			Message:     "The model is temporarily unavailable. Please try again later.",
			RawResponse: `{"error":{"code":503,"message":"The model is temporarily unavailable","status":"UNAVAILABLE"}}`,
			Retryable:   true,
			Notes:       "Temporary unavailability. Retry with backoff.",
		},

		// NON-RETRYABLE
		{
			Provider:    "google",
			Type:        FailureTypeAuthInvalid,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    401,
			ErrorCode:   "UNAUTHENTICATED",
			Message:     "Request had invalid authentication credentials",
			RawResponse: `{"error":{"code":401,"message":"Request had invalid authentication credentials","status":"UNAUTHENTICATED"}}`,
			Retryable:   false,
			Notes:       "Invalid service account or API key.",
		},
		{
			Provider:    "google",
			Type:        FailureTypeContentPolicy,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    400,
			ErrorCode:   "INVALID_ARGUMENT",
			Message:     "User input or prompt contains blocked content",
			RawResponse: `{"error":{"code":400,"message":"User input or prompt contains blocked content","status":"INVALID_ARGUMENT"}}`,
			Retryable:   false,
			Notes:       "Content blocked by safety filters.",
		},
		{
			Provider:    "google",
			Type:        FailureTypeQuotaExhausted,
			Category:    FailureCategoryNonRetryable,
			HTTPCode:    429,
			ErrorCode:   "RESOURCE_EXHAUSTED",
			Message:     "Quota exceeded for aiplatform.googleapis.com/base_model_generate_content_requests_per_day",
			RawResponse: `{"error":{"code":429,"message":"Quota exceeded for aiplatform.googleapis.com/base_model_generate_content_requests_per_day","status":"RESOURCE_EXHAUSTED"}}`,
			Retryable:   false,
			Notes:       "Daily quota exhausted. Different from per-minute rate limit.",
		},

		// =====================================================================
		// AZURE OPENAI EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:  "azure",
			Type:      FailureTypeRateLimit,
			Category:  FailureCategoryRetryable,
			HTTPCode:  429,
			ErrorCode: "429",
			Message:   "Requests to the ChatCompletions_Create Operation have exceeded rate limit. Try again in 59 seconds.",
			Retryable: true,
			Notes:     "Azure includes retry time in message. Parse and respect it.",
		},
		{
			Provider:  "azure",
			Type:      FailureTypeOverloaded,
			Category:  FailureCategoryRetryable,
			HTTPCode:  503,
			ErrorCode: "ServiceUnavailable",
			Message:   "The service is temporarily unable to process your request. Please try again later.",
			Retryable: true,
			Notes:     "Temporary Azure infrastructure issue.",
		},

		// NON-RETRYABLE
		{
			Provider:  "azure",
			Type:      FailureTypeAuthInvalid,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  401,
			ErrorCode: "401",
			Message:   "Access denied due to invalid subscription key or wrong API endpoint.",
			Retryable: false,
			Notes:     "Check Azure subscription key and endpoint URL.",
		},
		{
			Provider:  "azure",
			Type:      FailureTypeModelNotFound,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  404,
			ErrorCode: "DeploymentNotFound",
			Message:   "The API deployment for this resource does not exist.",
			Retryable: false,
			Notes:     "Deployment name is wrong or model not deployed.",
		},
		{
			Provider:  "azure",
			Type:      FailureTypeContentPolicy,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  400,
			ErrorCode: "content_filter",
			Message:   "The response was filtered due to the prompt triggering Azure OpenAI's content management policy.",
			Retryable: false,
			Notes:     "Azure content filters are stricter. May need to modify prompt.",
		},

		// =====================================================================
		// OPENROUTER EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:  "openrouter",
			Type:      FailureTypeRateLimit,
			Category:  FailureCategoryRetryable,
			HTTPCode:  429,
			ErrorCode: "rate_limit",
			Message:   "Rate limit exceeded. Please slow down your requests.",
			Retryable: true,
			Notes:     "OpenRouter rate limits. May automatically failover to different provider.",
		},
		{
			Provider:  "openrouter",
			Type:      FailureTypeProviderUnavailable,
			Category:  FailureCategoryConditional,
			HTTPCode:  502,
			ErrorCode: "provider_returned_error",
			Message:   "The upstream provider returned an error. OpenRouter may automatically retry with a different provider.",
			Retryable: true,
			Notes:     "OpenRouter proxies to underlying providers. Retrying may hit different provider.",
		},

		// NON-RETRYABLE
		{
			Provider:  "openrouter",
			Type:      FailureTypeQuotaExhausted,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  402,
			ErrorCode: "insufficient_credits",
			Message:   "Insufficient credits. Please add credits at openrouter.ai/account.",
			Retryable: false,
			Notes:     "Account needs more credits.",
		},
		{
			Provider:  "openrouter",
			Type:      FailureTypeModelNotFound,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  404,
			ErrorCode: "model_not_found",
			Message:   "Model 'nonexistent/model' not found",
			Retryable: false,
			Notes:     "Model ID doesn't exist on OpenRouter.",
		},

		// =====================================================================
		// COHERE EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:  "cohere",
			Type:      FailureTypeRateLimit,
			Category:  FailureCategoryRetryable,
			HTTPCode:  429,
			ErrorCode: "too_many_requests",
			Message:   "You have exceeded the rate limit. Please try again later.",
			Retryable: true,
			Notes:     "Standard rate limiting. Back off and retry.",
		},

		// NON-RETRYABLE
		{
			Provider:  "cohere",
			Type:      FailureTypeAuthInvalid,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  401,
			ErrorCode: "invalid_token",
			Message:   "invalid api token",
			Retryable: false,
			Notes:     "Invalid Cohere API key.",
		},
		{
			Provider:  "cohere",
			Type:      FailureTypeContextTooLong,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  400,
			ErrorCode: "invalid_request",
			Message:   "total number of tokens cannot exceed 4096",
			Retryable: false,
			Notes:     "Must reduce input size.",
		},

		// =====================================================================
		// MISTRAL EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:  "mistral",
			Type:      FailureTypeRateLimit,
			Category:  FailureCategoryRetryable,
			HTTPCode:  429,
			ErrorCode: "rate_limit_exceeded",
			Message:   "Rate limit exceeded. Please retry after 60 seconds.",
			Retryable: true,
			Notes:     "Mistral rate limit. Check retry-after header.",
		},

		// NON-RETRYABLE
		{
			Provider:  "mistral",
			Type:      FailureTypeAuthInvalid,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  401,
			ErrorCode: "unauthorized",
			Message:   "Unauthorized - Invalid API key",
			Retryable: false,
			Notes:     "Invalid Mistral API key.",
		},

		// =====================================================================
		// TOGETHER AI EXAMPLES
		// =====================================================================

		// RETRYABLE
		{
			Provider:  "together",
			Type:      FailureTypeServerError,
			Category:  FailureCategoryRetryable,
			HTTPCode:  500,
			ErrorCode: "internal_error",
			Message:   "An internal error occurred. Please try again.",
			Retryable: true,
			Notes:     "Temporary Together AI infrastructure issue.",
		},

		// NON-RETRYABLE
		{
			Provider:  "together",
			Type:      FailureTypeQuotaExhausted,
			Category:  FailureCategoryNonRetryable,
			HTTPCode:  402,
			ErrorCode: "insufficient_balance",
			Message:   "Insufficient balance. Please add credits.",
			Retryable: false,
			Notes:     "Account needs more credits.",
		},
	}
}

// =============================================================================
// CLASSIFICATION FUNCTIONS
// =============================================================================

// ClassifyProviderFailure determines if a failure is retryable based on
// HTTP code, error message, and provider-specific patterns
func ClassifyProviderFailure(httpCode int, errorCode, message, provider string) *ProviderFailure {
	failure := &ProviderFailure{
		HTTPCode: httpCode,
		Message:  message,
		Provider: provider,
	}

	// First, check for non-retryable patterns in the message
	if isContextTooLongMessage(message) {
		failure.Type = FailureTypeContextTooLong
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Reduce input size or use a model with larger context window"
		return failure
	}

	if isContentPolicyMessage(message) {
		failure.Type = FailureTypeContentPolicy
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Modify the prompt to comply with content policy"
		return failure
	}

	if isQuotaExhaustedMessage(message) {
		failure.Type = FailureTypeQuotaExhausted
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Add credits or upgrade plan"
		return failure
	}

	// Classify by HTTP status code
	switch httpCode {
	case 400:
		failure.Type = FailureTypeInvalidRequest
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Fix request format or parameters"

	case 401:
		failure.Type = FailureTypeAuthInvalid
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Provide valid API credentials"

	case 402:
		failure.Type = FailureTypeQuotaExhausted
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Add payment method or credits"

	case 403:
		failure.Type = FailureTypePermissionDenied
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Request access to the resource"

	case 404:
		failure.Type = FailureTypeModelNotFound
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Use a valid model ID"

	case 413:
		failure.Type = FailureTypeContextTooLong
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Reduce input size"

	case 429:
		// 429 can be rate limit (retryable) or quota exhausted (non-retryable)
		if isQuotaExhaustedMessage(message) {
			failure.Type = FailureTypeQuotaExhausted
			failure.Category = FailureCategoryNonRetryable
			failure.Retryable = false
			failure.RequiresAction = "Add credits or wait for quota reset"
		} else {
			failure.Type = FailureTypeRateLimit
			failure.Category = FailureCategoryRetryable
			failure.Retryable = true
			failure.MaxRetries = 5
		}

	case 500:
		failure.Type = FailureTypeServerError
		failure.Category = FailureCategoryRetryable
		failure.Retryable = true
		failure.MaxRetries = 3

	case 501:
		failure.Type = FailureTypeUnsupportedFeature
		failure.Category = FailureCategoryNonRetryable
		failure.Retryable = false
		failure.RequiresAction = "Feature not supported by this provider"

	case 502:
		failure.Type = FailureTypeServerError
		failure.Category = FailureCategoryRetryable
		failure.Retryable = true
		failure.FallbackSuggested = true
		failure.MaxRetries = 3

	case 503:
		if isOverloadedMessage(message) {
			failure.Type = FailureTypeOverloaded
		} else {
			failure.Type = FailureTypeServerError
		}
		failure.Category = FailureCategoryRetryable
		failure.Retryable = true
		failure.MaxRetries = 5

	case 504:
		failure.Type = FailureTypeTimeout
		failure.Category = FailureCategoryRetryable
		failure.Retryable = true
		failure.MaxRetries = 2

	case 529: // Anthropic-specific overloaded
		failure.Type = FailureTypeOverloaded
		failure.Category = FailureCategoryRetryable
		failure.Retryable = true
		failure.MaxRetries = 5

	default:
		if httpCode >= 500 {
			failure.Type = FailureTypeServerError
			failure.Category = FailureCategoryRetryable
			failure.Retryable = true
			failure.MaxRetries = 3
		} else {
			failure.Type = FailureTypeInvalidRequest
			failure.Category = FailureCategoryNonRetryable
			failure.Retryable = false
		}
	}

	return failure
}

// =============================================================================
// HELPER FUNCTIONS FOR MESSAGE PATTERN MATCHING
// =============================================================================

func isContextTooLongMessage(msg string) bool {
	lower := strings.ToLower(msg)
	patterns := []string{
		"maximum context length",
		"context length exceeded",
		"exceed context limit",
		"decrease input length",
		"too many tokens",
		"payload too large",
		"payload is too large",
		"input is too large",
		"input too large",
		"input is too long",
		"input too long",
		"prompt is too long",
		"exceeds the model's context",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func isContentPolicyMessage(msg string) bool {
	lower := strings.ToLower(msg)
	patterns := []string{
		"content policy",
		"safety system",
		"content filter",
		"blocked content",
		"violates our policy",
		"content management policy",
		"safety filters",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func isQuotaExhaustedMessage(msg string) bool {
	lower := strings.ToLower(msg)

	// Google uses "quota exceeded" for per-minute rate limits, so we need to
	// distinguish between rate limits and true quota exhaustion
	// Per-minute/second quotas are rate limits, daily quotas are quota exhaustion
	if strings.Contains(lower, "per_minute") || strings.Contains(lower, "per-minute") ||
		strings.Contains(lower, "per_second") || strings.Contains(lower, "per-second") ||
		strings.Contains(lower, "requests_per_min") {
		return false // This is a rate limit, not quota exhaustion
	}

	patterns := []string{
		"exceeded your current quota",
		"insufficient_quota",
		"insufficient credits",
		"insufficient balance",
		"quota exceeded",
		"billing",
		"payment required",
		"per_day", // Daily quotas are quota exhaustion
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func isOverloadedMessage(msg string) bool {
	lower := strings.ToLower(msg)
	patterns := []string{
		"overloaded",
		"overload",
		"resource has been exhausted",
		"temporarily unavailable",
		"high demand",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// =============================================================================
// RETRY STRATEGY RECOMMENDATIONS
// =============================================================================

// RetryStrategy provides guidance on how to retry a failure
type RetryStrategy struct {
	ShouldRetry       bool    `json:"shouldRetry"`
	MaxAttempts       int     `json:"maxAttempts"`
	InitialDelayMs    int     `json:"initialDelayMs"`
	MaxDelayMs        int     `json:"maxDelayMs"`
	BackoffMultiplier float64 `json:"backoffMultiplier"`
	UseJitter         bool    `json:"useJitter"`
	RespectRetryAfter bool    `json:"respectRetryAfter"`
	TryFallbackFirst  bool    `json:"tryFallbackFirst"`
	Notes             string  `json:"notes"`
}

// GetRetryStrategy returns the recommended retry strategy for a failure type
func GetRetryStrategy(failureType FailureType) RetryStrategy {
	switch failureType {
	case FailureTypeRateLimit:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       5,
			InitialDelayMs:    1000,
			MaxDelayMs:        60000,
			BackoffMultiplier: 2.0,
			UseJitter:         true,
			RespectRetryAfter: true,
			TryFallbackFirst:  false,
			Notes:             "Respect Retry-After header if present. Use exponential backoff with jitter.",
		}

	case FailureTypeOverloaded:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       5,
			InitialDelayMs:    5000,
			MaxDelayMs:        120000,
			BackoffMultiplier: 2.0,
			UseJitter:         true,
			RespectRetryAfter: true,
			TryFallbackFirst:  true,
			Notes:             "Server overloaded. Consider provider fallback if configured.",
		}

	case FailureTypeServerError:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       3,
			InitialDelayMs:    1000,
			MaxDelayMs:        30000,
			BackoffMultiplier: 2.0,
			UseJitter:         true,
			RespectRetryAfter: false,
			TryFallbackFirst:  true,
			Notes:             "Temporary server error. May indicate infrastructure issue.",
		}

	case FailureTypeTimeout:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       2,
			InitialDelayMs:    0, // Immediate retry
			MaxDelayMs:        0,
			BackoffMultiplier: 1.0,
			UseJitter:         false,
			RespectRetryAfter: false,
			TryFallbackFirst:  false,
			Notes:             "Timeout. Consider increasing timeout or using streaming.",
		}

	case FailureTypeConnectionError:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       3,
			InitialDelayMs:    500,
			MaxDelayMs:        5000,
			BackoffMultiplier: 1.5,
			UseJitter:         true,
			RespectRetryAfter: false,
			TryFallbackFirst:  false,
			Notes:             "Network error. Check connectivity.",
		}

	case FailureTypeStreamInterrupted:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       2,
			InitialDelayMs:    1000,
			MaxDelayMs:        5000,
			BackoffMultiplier: 1.5,
			UseJitter:         false,
			RespectRetryAfter: false,
			TryFallbackFirst:  false,
			Notes:             "Stream interrupted. Retry from beginning unless checkpointing available.",
		}

	case FailureTypeCacheError:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       1,
			InitialDelayMs:    0,
			MaxDelayMs:        0,
			BackoffMultiplier: 1.0,
			UseJitter:         false,
			RespectRetryAfter: false,
			TryFallbackFirst:  false,
			Notes:             "Retry without cache parameters.",
		}

	case FailureTypeProviderUnavailable:
		return RetryStrategy{
			ShouldRetry:       true,
			MaxAttempts:       3,
			InitialDelayMs:    1000,
			MaxDelayMs:        10000,
			BackoffMultiplier: 2.0,
			UseJitter:         true,
			RespectRetryAfter: false,
			TryFallbackFirst:  true,
			Notes:             "Provider unavailable. Use fallback provider if configured.",
		}

	// Non-retryable failures
	case FailureTypeAuthInvalid,
		FailureTypePermissionDenied,
		FailureTypeContextTooLong,
		FailureTypeInvalidRequest,
		FailureTypeContentPolicy,
		FailureTypeQuotaExhausted,
		FailureTypeModelNotFound,
		FailureTypeModelDeprecated,
		FailureTypeUnsupportedFeature,
		FailureTypeAccountSuspended:
		return RetryStrategy{
			ShouldRetry:       false,
			MaxAttempts:       0,
			InitialDelayMs:    0,
			MaxDelayMs:        0,
			BackoffMultiplier: 0,
			UseJitter:         false,
			RespectRetryAfter: false,
			TryFallbackFirst:  false,
			Notes:             "Non-retryable error. Requires user action.",
		}

	default:
		return RetryStrategy{
			ShouldRetry:       false,
			MaxAttempts:       0,
			InitialDelayMs:    0,
			MaxDelayMs:        0,
			BackoffMultiplier: 0,
			UseJitter:         false,
			RespectRetryAfter: false,
			TryFallbackFirst:  false,
			Notes:             "Unknown failure type. Default to non-retryable.",
		}
	}
}

// =============================================================================
// SUMMARY TABLES
// =============================================================================
//
// RETRYABLE FAILURES:
// +------------------------+------+--------------------------------------------+
// | Type                   | HTTP | Retry Strategy                             |
// +------------------------+------+--------------------------------------------+
// | rate_limit             | 429  | Exponential backoff, respect Retry-After   |
// | overloaded             | 503  | Exponential backoff, consider fallback     |
// | server_error           | 5xx  | Exponential backoff, 3 attempts            |
// | timeout                | 504  | Immediate retry, 2 attempts                |
// | connection_error       | -    | Short backoff, check connectivity          |
// | stream_interrupted     | -    | Retry from start, 2 attempts               |
// | cache_error            | -    | Single retry without cache params          |
// | provider_unavailable   | 502  | Try fallback provider                      |
// +------------------------+------+--------------------------------------------+
//
// NON-RETRYABLE FAILURES:
// +------------------------+------+--------------------------------------------+
// | Type                   | HTTP | Required Action                            |
// +------------------------+------+--------------------------------------------+
// | auth_invalid           | 401  | Fix API credentials                        |
// | permission_denied      | 403  | Request access to resource                 |
// | context_too_long       | 400  | Reduce input size                          |
// | invalid_request        | 400  | Fix request format                         |
// | content_policy         | 400  | Modify content                             |
// | quota_exhausted        | 429  | Add credits/upgrade plan                   |
// | model_not_found        | 404  | Use valid model ID                         |
// | model_deprecated       | -    | Migrate to newer model                     |
// | unsupported_feature    | 501  | Change approach                            |
// | account_suspended      | 403  | Contact provider                           |
// +------------------------+------+--------------------------------------------+
//
// IMPORTANT: 429 status code requires message inspection to distinguish
// between rate limit (retryable) and quota exhausted (non-retryable).
//
// =============================================================================
