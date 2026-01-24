package shared

import (
	"testing"
)

func TestClassifyProviderFailure_RateLimiting(t *testing.T) {
	tests := []struct {
		name          string
		httpCode      int
		errorCode     string
		message       string
		provider      string
		wantType      FailureType
		wantRetryable bool
	}{
		{
			name:          "OpenAI rate limit",
			httpCode:      429,
			errorCode:     "rate_limit_exceeded",
			message:       "Rate limit reached for gpt-4 in organization org-xxx",
			provider:      "openai",
			wantType:      FailureTypeRateLimit,
			wantRetryable: true,
		},
		{
			name:          "Anthropic rate limit",
			httpCode:      429,
			errorCode:     "rate_limit_error",
			message:       "Number of request tokens has exceeded your per-minute rate limit",
			provider:      "anthropic",
			wantType:      FailureTypeRateLimit,
			wantRetryable: true,
		},
		{
			name:          "Google rate limit",
			httpCode:      429,
			errorCode:     "RESOURCE_EXHAUSTED",
			message:       "Quota exceeded for aiplatform.googleapis.com/generate_content_requests_per_minute",
			provider:      "google",
			wantType:      FailureTypeRateLimit,
			wantRetryable: true,
		},
		{
			name:          "Azure rate limit",
			httpCode:      429,
			errorCode:     "429",
			message:       "Requests to the ChatCompletions_Create Operation have exceeded rate limit. Try again in 59 seconds.",
			provider:      "azure",
			wantType:      FailureTypeRateLimit,
			wantRetryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyProviderFailure(tt.httpCode, tt.errorCode, tt.message, tt.provider)
			if result.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", result.Retryable, tt.wantRetryable)
			}
		})
	}
}

func TestClassifyProviderFailure_QuotaExhausted(t *testing.T) {
	tests := []struct {
		name          string
		httpCode      int
		message       string
		provider      string
		wantType      FailureType
		wantRetryable bool
	}{
		{
			name:          "OpenAI quota exhausted",
			httpCode:      429,
			message:       "You exceeded your current quota, please check your plan and billing details.",
			provider:      "openai",
			wantType:      FailureTypeQuotaExhausted,
			wantRetryable: false,
		},
		{
			name:          "OpenRouter insufficient credits",
			httpCode:      402,
			message:       "Insufficient credits. Please add credits at openrouter.ai/account.",
			provider:      "openrouter",
			wantType:      FailureTypeQuotaExhausted,
			wantRetryable: false,
		},
		{
			name:          "Together insufficient balance",
			httpCode:      402,
			message:       "Insufficient balance. Please add credits.",
			provider:      "together",
			wantType:      FailureTypeQuotaExhausted,
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyProviderFailure(tt.httpCode, "", tt.message, tt.provider)
			if result.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", result.Retryable, tt.wantRetryable)
			}
		})
	}
}

func TestClassifyProviderFailure_ContextTooLong(t *testing.T) {
	tests := []struct {
		name     string
		httpCode int
		message  string
		provider string
	}{
		{
			name:     "OpenAI context length exceeded",
			httpCode: 400,
			message:  "This model's maximum context length is 8192 tokens. However, your messages resulted in 12847 tokens.",
			provider: "openai",
		},
		{
			name:     "Anthropic prompt too long",
			httpCode: 400,
			message:  "prompt is too long: 234567 tokens > 200000 maximum",
			provider: "anthropic",
		},
		{
			name:     "Too many tokens generic",
			httpCode: 400,
			message:  "Request has too many tokens",
			provider: "generic",
		},
		{
			name:     "Payload too large",
			httpCode: 413,
			message:  "Payload too large",
			provider: "generic",
		},
		{
			name:     "Input too long",
			httpCode: 400,
			message:  "The input is too long for this model",
			provider: "generic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyProviderFailure(tt.httpCode, "", tt.message, tt.provider)
			if result.Type != FailureTypeContextTooLong {
				t.Errorf("Type = %v, want %v", result.Type, FailureTypeContextTooLong)
			}
			if result.Retryable {
				t.Errorf("Retryable = %v, want false", result.Retryable)
			}
			if result.Category != FailureCategoryNonRetryable {
				t.Errorf("Category = %v, want %v", result.Category, FailureCategoryNonRetryable)
			}
		})
	}
}

func TestClassifyProviderFailure_Authentication(t *testing.T) {
	tests := []struct {
		name     string
		httpCode int
		message  string
		provider string
		wantType FailureType
	}{
		{
			name:     "OpenAI invalid API key",
			httpCode: 401,
			message:  "Incorrect API key provided: sk-xxx",
			provider: "openai",
			wantType: FailureTypeAuthInvalid,
		},
		{
			name:     "Anthropic invalid key",
			httpCode: 401,
			message:  "Invalid API Key",
			provider: "anthropic",
			wantType: FailureTypeAuthInvalid,
		},
		{
			name:     "Google unauthenticated",
			httpCode: 401,
			message:  "Request had invalid authentication credentials",
			provider: "google",
			wantType: FailureTypeAuthInvalid,
		},
		{
			name:     "Azure invalid subscription",
			httpCode: 401,
			message:  "Access denied due to invalid subscription key",
			provider: "azure",
			wantType: FailureTypeAuthInvalid,
		},
		{
			name:     "Anthropic permission denied",
			httpCode: 403,
			message:  "Your API key does not have permission to use the specified resource",
			provider: "anthropic",
			wantType: FailureTypePermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyProviderFailure(tt.httpCode, "", tt.message, tt.provider)
			if result.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Retryable {
				t.Error("Retryable should be false for auth errors")
			}
		})
	}
}

func TestClassifyProviderFailure_ServerErrors(t *testing.T) {
	tests := []struct {
		name          string
		httpCode      int
		message       string
		provider      string
		wantType      FailureType
		wantRetryable bool
	}{
		{
			name:          "OpenAI 500",
			httpCode:      500,
			message:       "The server had an error while processing your request",
			provider:      "openai",
			wantType:      FailureTypeServerError,
			wantRetryable: true,
		},
		{
			name:          "Anthropic overloaded 529",
			httpCode:      529,
			message:       "Anthropic's API is temporarily overloaded",
			provider:      "anthropic",
			wantType:      FailureTypeOverloaded,
			wantRetryable: true,
		},
		{
			name:          "OpenAI 503 overloaded",
			httpCode:      503,
			message:       "The engine is currently overloaded, please try again later",
			provider:      "openai",
			wantType:      FailureTypeOverloaded,
			wantRetryable: true,
		},
		{
			name:          "Gateway timeout",
			httpCode:      504,
			message:       "Request timed out",
			provider:      "openai",
			wantType:      FailureTypeTimeout,
			wantRetryable: true,
		},
		{
			name:          "Bad gateway",
			httpCode:      502,
			message:       "Bad gateway",
			provider:      "openrouter",
			wantType:      FailureTypeServerError,
			wantRetryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyProviderFailure(tt.httpCode, "", tt.message, tt.provider)
			if result.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", result.Retryable, tt.wantRetryable)
			}
		})
	}
}

func TestClassifyProviderFailure_ContentPolicy(t *testing.T) {
	tests := []struct {
		name     string
		httpCode int
		message  string
		provider string
	}{
		{
			name:     "OpenAI content policy",
			httpCode: 400,
			message:  "Your request was rejected as a result of our safety system",
			provider: "openai",
		},
		{
			name:     "Azure content filter",
			httpCode: 400,
			message:  "The response was filtered due to the prompt triggering Azure OpenAI's content management policy",
			provider: "azure",
		},
		{
			name:     "Google blocked content",
			httpCode: 400,
			message:  "User input or prompt contains blocked content",
			provider: "google",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyProviderFailure(tt.httpCode, "", tt.message, tt.provider)
			if result.Type != FailureTypeContentPolicy {
				t.Errorf("Type = %v, want %v", result.Type, FailureTypeContentPolicy)
			}
			if result.Retryable {
				t.Error("Retryable should be false for content policy errors")
			}
		})
	}
}

func TestClassifyProviderFailure_ModelNotFound(t *testing.T) {
	tests := []struct {
		name     string
		httpCode int
		message  string
		provider string
	}{
		{
			name:     "OpenAI model not found",
			httpCode: 404,
			message:  "The model 'gpt-5' does not exist",
			provider: "openai",
		},
		{
			name:     "Azure deployment not found",
			httpCode: 404,
			message:  "The API deployment for this resource does not exist",
			provider: "azure",
		},
		{
			name:     "OpenRouter model not found",
			httpCode: 404,
			message:  "Model 'nonexistent/model' not found",
			provider: "openrouter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyProviderFailure(tt.httpCode, "", tt.message, tt.provider)
			if result.Type != FailureTypeModelNotFound {
				t.Errorf("Type = %v, want %v", result.Type, FailureTypeModelNotFound)
			}
			if result.Retryable {
				t.Error("Retryable should be false for model not found errors")
			}
		})
	}
}

func TestGetRetryStrategy(t *testing.T) {
	tests := []struct {
		failureType   FailureType
		wantRetry     bool
		wantAttempts  int
		wantRespectRA bool // Respect Retry-After
	}{
		{
			failureType:   FailureTypeRateLimit,
			wantRetry:     true,
			wantAttempts:  5,
			wantRespectRA: true,
		},
		{
			failureType:   FailureTypeOverloaded,
			wantRetry:     true,
			wantAttempts:  5,
			wantRespectRA: true,
		},
		{
			failureType:   FailureTypeServerError,
			wantRetry:     true,
			wantAttempts:  3,
			wantRespectRA: false,
		},
		{
			failureType:   FailureTypeTimeout,
			wantRetry:     true,
			wantAttempts:  2,
			wantRespectRA: false,
		},
		{
			failureType:   FailureTypeAuthInvalid,
			wantRetry:     false,
			wantAttempts:  0,
			wantRespectRA: false,
		},
		{
			failureType:   FailureTypeContextTooLong,
			wantRetry:     false,
			wantAttempts:  0,
			wantRespectRA: false,
		},
		{
			failureType:   FailureTypeQuotaExhausted,
			wantRetry:     false,
			wantAttempts:  0,
			wantRespectRA: false,
		},
		{
			failureType:   FailureTypeContentPolicy,
			wantRetry:     false,
			wantAttempts:  0,
			wantRespectRA: false,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.failureType), func(t *testing.T) {
			strategy := GetRetryStrategy(tt.failureType)
			if strategy.ShouldRetry != tt.wantRetry {
				t.Errorf("ShouldRetry = %v, want %v", strategy.ShouldRetry, tt.wantRetry)
			}
			if strategy.MaxAttempts != tt.wantAttempts {
				t.Errorf("MaxAttempts = %v, want %v", strategy.MaxAttempts, tt.wantAttempts)
			}
			if strategy.RespectRetryAfter != tt.wantRespectRA {
				t.Errorf("RespectRetryAfter = %v, want %v", strategy.RespectRetryAfter, tt.wantRespectRA)
			}
		})
	}
}

func TestGetProviderFailureExamples(t *testing.T) {
	examples := GetProviderFailureExamples()

	if len(examples) == 0 {
		t.Error("GetProviderFailureExamples() should return examples")
	}

	// Count examples by provider
	providerCounts := make(map[string]int)
	for _, ex := range examples {
		providerCounts[ex.Provider]++
	}

	// Verify we have examples from major providers
	expectedProviders := []string{"openai", "anthropic", "google", "azure", "openrouter"}
	for _, p := range expectedProviders {
		if providerCounts[p] == 0 {
			t.Errorf("No examples found for provider %s", p)
		}
	}

	// Verify each example has required fields
	for i, ex := range examples {
		if ex.Provider == "" {
			t.Errorf("Example %d missing provider", i)
		}
		if ex.Type == "" {
			t.Errorf("Example %d missing type", i)
		}
		if ex.Category == "" {
			t.Errorf("Example %d missing category", i)
		}
		if ex.Message == "" {
			t.Errorf("Example %d missing message", i)
		}
	}
}

func TestIsContextTooLongMessage(t *testing.T) {
	positives := []string{
		"maximum context length is 8192",
		"context length exceeded",
		"too many tokens in request",
		"payload too large",
		"input is too long",
		"prompt is too long: 234567 tokens",
	}

	negatives := []string{
		"rate limit exceeded",
		"invalid api key",
		"server error",
		"normal message",
	}

	for _, msg := range positives {
		if !isContextTooLongMessage(msg) {
			t.Errorf("isContextTooLongMessage(%q) = false, want true", msg)
		}
	}

	for _, msg := range negatives {
		if isContextTooLongMessage(msg) {
			t.Errorf("isContextTooLongMessage(%q) = true, want false", msg)
		}
	}
}

func TestIsContentPolicyMessage(t *testing.T) {
	positives := []string{
		"content policy violation",
		"rejected by our safety system",
		"content filter triggered",
		"contains blocked content",
	}

	negatives := []string{
		"rate limit exceeded",
		"invalid api key",
		"server error",
		"normal message",
	}

	for _, msg := range positives {
		if !isContentPolicyMessage(msg) {
			t.Errorf("isContentPolicyMessage(%q) = false, want true", msg)
		}
	}

	for _, msg := range negatives {
		if isContentPolicyMessage(msg) {
			t.Errorf("isContentPolicyMessage(%q) = true, want false", msg)
		}
	}
}

func TestIsQuotaExhaustedMessage(t *testing.T) {
	positives := []string{
		"exceeded your current quota",
		"insufficient_quota",
		"insufficient credits",
		"insufficient balance",
	}

	negatives := []string{
		"rate limit exceeded",
		"invalid api key",
		"server error",
		"try again later",
	}

	for _, msg := range positives {
		if !isQuotaExhaustedMessage(msg) {
			t.Errorf("isQuotaExhaustedMessage(%q) = false, want true", msg)
		}
	}

	for _, msg := range negatives {
		if isQuotaExhaustedMessage(msg) {
			t.Errorf("isQuotaExhaustedMessage(%q) = true, want false", msg)
		}
	}
}

func TestIsOverloadedMessage(t *testing.T) {
	positives := []string{
		"server is overloaded",
		"model overload detected",
		"resource has been exhausted",
		"temporarily unavailable",
		"due to high demand",
	}

	negatives := []string{
		"rate limit exceeded",
		"invalid api key",
		"normal error",
	}

	for _, msg := range positives {
		if !isOverloadedMessage(msg) {
			t.Errorf("isOverloadedMessage(%q) = false, want true", msg)
		}
	}

	for _, msg := range negatives {
		if isOverloadedMessage(msg) {
			t.Errorf("isOverloadedMessage(%q) = true, want false", msg)
		}
	}
}

func TestFailureCategoryConstants(t *testing.T) {
	if FailureCategoryRetryable != "retryable" {
		t.Errorf("FailureCategoryRetryable = %s, want retryable", FailureCategoryRetryable)
	}
	if FailureCategoryNonRetryable != "non_retryable" {
		t.Errorf("FailureCategoryNonRetryable = %s, want non_retryable", FailureCategoryNonRetryable)
	}
	if FailureCategoryConditional != "conditional" {
		t.Errorf("FailureCategoryConditional = %s, want conditional", FailureCategoryConditional)
	}
}

func TestRetryableFailureTypes(t *testing.T) {
	retryableTypes := []FailureType{
		FailureTypeRateLimit,
		FailureTypeOverloaded,
		FailureTypeServerError,
		FailureTypeTimeout,
		FailureTypeConnectionError,
		FailureTypeStreamInterrupted,
		FailureTypeCacheError,
	}

	for _, ft := range retryableTypes {
		strategy := GetRetryStrategy(ft)
		if !strategy.ShouldRetry {
			t.Errorf("GetRetryStrategy(%s).ShouldRetry = false, want true", ft)
		}
	}
}

func TestNonRetryableFailureTypes(t *testing.T) {
	nonRetryableTypes := []FailureType{
		FailureTypeAuthInvalid,
		FailureTypePermissionDenied,
		FailureTypeContextTooLong,
		FailureTypeInvalidRequest,
		FailureTypeContentPolicy,
		FailureTypeQuotaExhausted,
		FailureTypeModelNotFound,
		FailureTypeModelDeprecated,
		FailureTypeUnsupportedFeature,
		FailureTypeAccountSuspended,
	}

	for _, ft := range nonRetryableTypes {
		strategy := GetRetryStrategy(ft)
		if strategy.ShouldRetry {
			t.Errorf("GetRetryStrategy(%s).ShouldRetry = true, want false", ft)
		}
	}
}

// TestRealWorldScenarios tests classification of realistic error scenarios
func TestRealWorldScenarios(t *testing.T) {
	scenarios := []struct {
		name          string
		httpCode      int
		message       string
		provider      string
		wantRetryable bool
		wantType      FailureType
	}{
		{
			name:          "High traffic OpenAI rate limit",
			httpCode:      429,
			message:       "Rate limit reached for requests per min (RPM): Limit 200, Used 200, Requested 1. Please try again in 6ms.",
			provider:      "openai",
			wantRetryable: true,
			wantType:      FailureTypeRateLimit,
		},
		{
			name:          "Anthropic during peak usage",
			httpCode:      529,
			message:       "Anthropic's API is temporarily overloaded. Please try again shortly.",
			provider:      "anthropic",
			wantRetryable: true,
			wantType:      FailureTypeOverloaded,
		},
		{
			name:          "Expired API key",
			httpCode:      401,
			message:       "Invalid API key. Your API key may have been rotated or revoked.",
			provider:      "openai",
			wantRetryable: false,
			wantType:      FailureTypeAuthInvalid,
		},
		{
			name:          "Large codebase context",
			httpCode:      400,
			message:       "This model's maximum context length is 128000 tokens. However, your messages resulted in 150234 tokens. Please reduce the length of the messages.",
			provider:      "openai",
			wantRetryable: false,
			wantType:      FailureTypeContextTooLong,
		},
		{
			name:          "Free tier exhausted",
			httpCode:      429,
			message:       "You exceeded your current quota, please check your plan and billing details. For more information on this error, read the docs.",
			provider:      "openai",
			wantRetryable: false,
			wantType:      FailureTypeQuotaExhausted,
		},
		{
			name:          "Deprecated model",
			httpCode:      404,
			message:       "The model `text-davinci-003` has been deprecated",
			provider:      "openai",
			wantRetryable: false,
			wantType:      FailureTypeModelNotFound,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			result := ClassifyProviderFailure(s.httpCode, "", s.message, s.provider)
			if result.Retryable != s.wantRetryable {
				t.Errorf("Retryable = %v, want %v", result.Retryable, s.wantRetryable)
			}
			if result.Type != s.wantType {
				t.Errorf("Type = %v, want %v", result.Type, s.wantType)
			}
		})
	}
}
