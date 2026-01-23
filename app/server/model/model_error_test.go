package model

import (
	"errors"
	"net/http"
	"testing"

	shared "plandex-shared"
)

func TestClassifyErrMsg(t *testing.T) {
	tests := []struct {
		name         string
		msg          string
		expectedKind shared.ModelErrKind
		expectNil    bool
	}{
		{
			name:         "maximum context length",
			msg:          "maximum context length exceeded",
			expectedKind: shared.ErrContextTooLong,
		},
		{
			name:         "context length exceeded uppercase",
			msg:          "CONTEXT LENGTH EXCEEDED for this model",
			expectedKind: shared.ErrContextTooLong,
		},
		{
			name:         "too many tokens",
			msg:          "Request has too many tokens",
			expectedKind: shared.ErrContextTooLong,
		},
		{
			name:         "payload too large",
			msg:          "payload too large for processing",
			expectedKind: shared.ErrContextTooLong,
		},
		{
			name:         "input is too long",
			msg:          "The input is too long for this model",
			expectedKind: shared.ErrContextTooLong,
		},
		{
			name:         "model overloaded",
			msg:          "model_overloaded error occurred",
			expectedKind: shared.ErrOverloaded,
		},
		{
			name:         "server is overloaded",
			msg:          "The server is overloaded, please try again",
			expectedKind: shared.ErrOverloaded,
		},
		{
			name:         "resource exhausted",
			msg:          "resource has been exhausted",
			expectedKind: shared.ErrOverloaded,
		},
		{
			name:         "cache control error",
			msg:          "cache control not supported",
			expectedKind: shared.ErrCacheSupport,
		},
		{
			name:      "unrecognized error",
			msg:       "some random error message",
			expectNil: true,
		},
		{
			name:      "empty message",
			msg:       "",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyErrMsg(tt.msg)
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil result, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Kind != tt.expectedKind {
				t.Errorf("Kind = %v, want %v", result.Kind, tt.expectedKind)
			}
		})
	}
}

func TestClassifyModelError(t *testing.T) {
	tests := []struct {
		name           string
		code           int
		message        string
		headers        http.Header
		isClaudeMax    bool
		expectedKind   shared.ModelErrKind
		expectRetriable bool
	}{
		{
			name:           "rate limited 429",
			code:           429,
			message:        "rate limit exceeded",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrRateLimited,
			expectRetriable: true,
		},
		{
			name:           "rate limited 529",
			code:           529,
			message:        "overloaded",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrRateLimited,
			expectRetriable: true,
		},
		{
			name:           "payload too large 413",
			code:           413,
			message:        "request entity too large",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrContextTooLong,
			expectRetriable: false,
		},
		{
			name:           "not implemented 501",
			code:           501,
			message:        "not implemented",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrOther,
			expectRetriable: false,
		},
		{
			name:           "http version not supported 505",
			code:           505,
			message:        "http version not supported",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrOther,
			expectRetriable: false,
		},
		{
			name:           "server error 500",
			code:           500,
			message:        "internal server error",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrOther,
			expectRetriable: true,
		},
		{
			name:           "bad request 400",
			code:           400,
			message:        "bad request",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrOther,
			expectRetriable: false,
		},
		{
			name:           "claude max 429 quota exhausted",
			code:           429,
			message:        "quota exceeded",
			headers:        http.Header{},
			isClaudeMax:    true,
			expectedKind:   shared.ErrSubscriptionQuotaExhausted,
			expectRetriable: false,
		},
		{
			name:           "context too long in message",
			code:           400,
			message:        "maximum context length exceeded",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrContextTooLong,
			expectRetriable: false,
		},
		{
			name:           "openrouter provider error",
			code:           400,
			message:        "provider returned error",
			headers:        http.Header{},
			isClaudeMax:    false,
			expectedKind:   shared.ErrOther,
			expectRetriable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyModelError(tt.code, tt.message, tt.headers, tt.isClaudeMax)
			if result.Kind != tt.expectedKind {
				t.Errorf("Kind = %v, want %v", result.Kind, tt.expectedKind)
			}
			if result.Retriable != tt.expectRetriable {
				t.Errorf("Retriable = %v, want %v", result.Retriable, tt.expectRetriable)
			}
		})
	}
}

func TestExtractRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		body     string
		expected int
	}{
		{
			name:     "retry-after header seconds",
			headers:  http.Header{"Retry-After": []string{"30"}},
			body:     "",
			expected: 30,
		},
		{
			name:     "json retry_after_ms",
			headers:  http.Header{},
			body:     `{"error": "rate limit", "retry_after_ms": 5000}`,
			expected: 5,
		},
		{
			name:     "text retry after seconds",
			headers:  http.Header{},
			body:     "Please retry after 10 seconds",
			expected: 10,
		},
		{
			name:     "try again in seconds",
			headers:  http.Header{},
			body:     "Try again in 15 seconds",
			expected: 15,
		},
		{
			name:     "retry in ms json",
			headers:  http.Header{},
			body:     `"retry_after_ms": 3000`,
			expected: 3,
		},
		{
			name:     "no retry info",
			headers:  http.Header{},
			body:     "generic error message",
			expected: 0,
		},
		{
			name:     "empty body and headers",
			headers:  http.Header{},
			body:     "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRetryAfter(tt.headers, tt.body)
			if result != tt.expected {
				t.Errorf("extractRetryAfter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNormalizeUnit(t *testing.T) {
	tests := []struct {
		name     string
		numStr   string
		unit     string
		expected int
	}{
		{"milliseconds", "5000", "ms", 5},
		{"seconds", "30", "seconds", 30},
		{"second", "1", "second", 1},
		{"secs", "45", "secs", 45},
		{"sec", "20", "sec", 20},
		{"s", "10", "s", 10},
		{"no unit defaults to seconds", "15", "", 15},
		{"unknown unit defaults to seconds", "25", "unknown", 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeUnit(tt.numStr, tt.unit)
			if result != tt.expected {
				t.Errorf("normalizeUnit(%q, %q) = %v, want %v", tt.numStr, tt.unit, result, tt.expected)
			}
		})
	}
}

func TestHTTPError(t *testing.T) {
	err := &HTTPError{
		StatusCode: 429,
		Body:       "rate limit exceeded",
		Header:     http.Header{"X-Request-Id": []string{"abc123"}},
	}

	expected := "status code: 429, body: rate limit exceeded"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestIsNonRetriableBasicErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "context deadline exceeded",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "context canceled",
			err:      errors.New("context canceled"),
			expected: true,
		},
		{
			name:     "token limit exceeded 400",
			err:      errors.New("status code: 400, reduce the length of the messages"),
			expected: true,
		},
		{
			name:     "unauthorized 401",
			err:      errors.New("status code: 401, invalid api key"),
			expected: true,
		},
		{
			name:     "quota exceeded 429",
			err:      errors.New("status code: 429, exceeded your current quota"),
			expected: true,
		},
		{
			name:     "retriable error",
			err:      errors.New("status code: 500, internal server error"),
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNonRetriableBasicErr(tt.err)
			if result != tt.expected {
				t.Errorf("isNonRetriableBasicErr() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestClassifyBasicError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		isClaudeMax    bool
		expectedKind   shared.ModelErrKind
		expectRetriable bool
	}{
		{
			name:           "http error 429",
			err:            &HTTPError{StatusCode: 429, Body: "rate limited", Header: http.Header{}},
			isClaudeMax:    false,
			expectedKind:   shared.ErrRateLimited,
			expectRetriable: true,
		},
		{
			name:           "http error 413",
			err:            &HTTPError{StatusCode: 413, Body: "too large", Header: http.Header{}},
			isClaudeMax:    false,
			expectedKind:   shared.ErrContextTooLong,
			expectRetriable: false,
		},
		{
			name:           "context too long in message",
			err:            errors.New("maximum context length exceeded"),
			isClaudeMax:    false,
			expectedKind:   shared.ErrContextTooLong,
			expectRetriable: false,
		},
		{
			name:           "overloaded in message",
			err:            errors.New("model_overloaded"),
			isClaudeMax:    false,
			expectedKind:   shared.ErrOverloaded,
			expectRetriable: true,
		},
		{
			name:           "context deadline exceeded",
			err:            errors.New("context deadline exceeded"),
			isClaudeMax:    false,
			expectedKind:   shared.ErrOther,
			expectRetriable: false,
		},
		{
			name:           "generic retriable error",
			err:            errors.New("temporary network failure"),
			isClaudeMax:    false,
			expectedKind:   shared.ErrOther,
			expectRetriable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyBasicError(tt.err, tt.isClaudeMax)
			if result.Kind != tt.expectedKind {
				t.Errorf("Kind = %v, want %v", result.Kind, tt.expectedKind)
			}
			if result.Retriable != tt.expectRetriable {
				t.Errorf("Retriable = %v, want %v", result.Retriable, tt.expectRetriable)
			}
		})
	}
}

func TestRegexPatterns(t *testing.T) {
	t.Run("reJSON matches retry_after_ms", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{`"retry_after_ms": 1234`, "1234"},
			{`"retry_after_ms":5000`, "5000"},
			{`{"error": "limit", "retry_after_ms": 3000}`, "3000"},
		}

		for _, tt := range tests {
			matches := reJSON.FindStringSubmatch(tt.input)
			if len(matches) < 2 {
				t.Errorf("reJSON failed to match %q", tt.input)
				continue
			}
			if matches[1] != tt.expected {
				t.Errorf("reJSON match = %q, want %q", matches[1], tt.expected)
			}
		}
	})

	t.Run("reRetryAfter matches various formats", func(t *testing.T) {
		tests := []string{
			"retry-after: 30",
			"retry_after: 30s",
			"retry after 30",
		}

		for _, input := range tests {
			if !reRetryAfter.MatchString(input) {
				t.Errorf("reRetryAfter failed to match %q", input)
			}
		}
	})

	t.Run("reTryAgain matches various formats", func(t *testing.T) {
		tests := []string{
			"try again in 30 seconds",
			"try again in 30",
			"retry in 10 seconds",
			"retry in 10s",
		}

		for _, input := range tests {
			if !reTryAgain.MatchString(input) {
				t.Errorf("reTryAgain failed to match %q", input)
			}
		}
	})
}
