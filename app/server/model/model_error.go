package model

import (
	"fmt"
	"log"
	"net/http"
	shared "plandex-shared"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type HTTPError struct {
	StatusCode int
	Body       string
	Header     http.Header
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("status code: %d, body: %s", e.StatusCode, e.Body)
}

// JSON-style  `"retry_after_ms":1234`
var reJSON = regexp.MustCompile(`"retry_after_ms"\s*:\s*(\d+)`)

// Header- or text-style  "Retry-After: 12" / "retry_after: 12s"
var reRetryAfter = regexp.MustCompile(
	`retry[_\-\s]?after[_\-\s]?(?:[:\s]+)?(\d+)(ms|seconds?|secs?|s)?`,
)

// Free-form Azure style  "Try again in 59 seconds."
// Also matches "Retry in 10 seconds."
var reTryAgain = regexp.MustCompile(
	`(?:re)?try[_\-\s]+(?:again[_\-\s]+)?in[_\-\s]+(\d+)(ms|seconds?|secs?|s)?`,
)

func ClassifyErrMsg(msg string) *shared.ModelError {
	log.Printf("Classifying error message: %s", msg)

	msg = strings.ToLower(msg)

	if strings.Contains(msg, "maximum context length") ||
		strings.Contains(msg, "context length exceeded") ||
		strings.Contains(msg, "exceed context limit") ||
		strings.Contains(msg, "decrease input length") ||
		strings.Contains(msg, "too many tokens") ||
		strings.Contains(msg, "payload too large") ||
		strings.Contains(msg, "payload is too large") ||
		strings.Contains(msg, "input is too large") ||
		strings.Contains(msg, "input too large") ||
		strings.Contains(msg, "input is too long") ||
		strings.Contains(msg, "input too long") {
		log.Printf("Context too long error: %s", msg)
		return &shared.ModelError{
			Kind:              shared.ErrContextTooLong,
			Retriable:         false,
			RetryAfterSeconds: 0,
		}
	}

	if strings.Contains(msg, "model_overloaded") ||
		strings.Contains(msg, "model overloaded") ||
		strings.Contains(msg, "server is overloaded") ||
		strings.Contains(msg, "model is currently overloaded") ||
		strings.Contains(msg, "overloaded_error") ||
		strings.Contains(msg, "resource has been exhausted") {
		log.Printf("Overloaded error: %s", msg)
		return &shared.ModelError{
			Kind:              shared.ErrOverloaded,
			Retriable:         true,
			RetryAfterSeconds: 0,
		}
	}

	if strings.Contains(msg, "cache control") {
		log.Printf("Cache control error: %s", msg)
		return &shared.ModelError{
			Kind:              shared.ErrCacheSupport,
			Retriable:         true,
			RetryAfterSeconds: 0,
		}
	}

	log.Println("No error classification based on message")

	return nil
}

func ClassifyModelError(code int, message string, headers http.Header, isClaudeMax bool) shared.ModelError {
	msg := strings.ToLower(message)

	// first of all, if it's claude max and a 429, it means the subscription limit was reached, so handle it accordingly
	if isClaudeMax && code == 429 {
		retryAfter := extractRetryAfter(headers, msg)
		if retryAfter > 0 {
			return shared.ModelError{
				Kind:              shared.ErrSubscriptionQuotaExhausted,
				Retriable:         true,
				RetryAfterSeconds: retryAfter,
			}
		}
		return shared.ModelError{
			Kind:              shared.ErrSubscriptionQuotaExhausted,
			Retriable:         false,
			RetryAfterSeconds: 0,
		}
	}

	// next try to classify the error based on the message only
	msgRes := ClassifyErrMsg(msg)
	if msgRes != nil {
		log.Printf("Classified error message: %+v", msgRes)
		return *msgRes
	}

	var res shared.ModelError

	switch code {
	case 429, 529:
		res = shared.ModelError{
			Kind:              shared.ErrRateLimited,
			Retriable:         true,
			RetryAfterSeconds: 0,
		}
	case 413:
		res = shared.ModelError{
			Kind:              shared.ErrContextTooLong,
			Retriable:         false,
			RetryAfterSeconds: 0,
		}

	// rare codes but they never succeed on retry if they do show up
	case 501, 505:
		res = shared.ModelError{
			Kind:              shared.ErrOther,
			Retriable:         false,
			RetryAfterSeconds: 0,
		}
	default:
		res = shared.ModelError{
			Kind:              shared.ErrOther,
			Retriable:         code >= 500 || strings.Contains(msg, "provider returned error"), // 'provider returned error' is from OpenRouter, and unless it's a non-retriable status code, it should still be retried since OpenRouter may switch to a different provider
			RetryAfterSeconds: 0,
		}
	}

	log.Printf("Model error: %+v", res)

	// best‑effort parse of "Retry‑After" style hints in the message
	if res.Retriable {
		retryAfter := extractRetryAfter(headers, msg)

		// if the retry after is greater than the max delay, then the error is not retriable
		if retryAfter > MAX_RETRY_DELAY_SECONDS {
			log.Printf("Retry after %d seconds is greater than the max delay of %d seconds - not retriable", retryAfter, MAX_RETRY_DELAY_SECONDS)
			res.Retriable = false
		} else {
			res.RetryAfterSeconds = retryAfter
		}

	}

	return res
}

func extractRetryAfter(h http.Header, body string) (sec int) {
	now := time.Now()

	// Retry-After header: seconds or HTTP-date
	if v := h.Get("Retry-After"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
		if t, err := time.Parse(http.TimeFormat, v); err == nil {
			d := int(t.Sub(now).Seconds())
			if d > 0 {
				return d
			}
		}
	}

	// X-RateLimit-Reset epoch
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if reset, _ := strconv.ParseInt(v, 10, 64); reset > now.Unix() {
			return int(reset - now.Unix())
		}
	}

	lower := strings.ToLower(strings.TrimSpace(body))

	// "retry_after_ms": 1234
	if m := reJSON.FindStringSubmatch(lower); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return n / 1000
	}
	// "retry after 12"
	if m := reRetryAfter.FindStringSubmatch(lower); len(m) >= 2 {
		unit := ""
		if len(m) == 3 {
			unit = m[2]
		}
		return normalizeUnit(m[1], unit)
	}

	// "try again in 8"
	if m := reTryAgain.FindStringSubmatch(lower); len(m) >= 2 {
		unit := ""
		if len(m) == 3 {
			unit = m[2]
		}
		return normalizeUnit(m[1], unit)
	}
	return 0
}

func normalizeUnit(numStr, unit string) int {
	n, _ := strconv.Atoi(numStr) // safe because the regex matched \d+

	switch unit {
	case "ms": // milliseconds
		return n / 1000
	case "sec", "secs", "second", "seconds", "s":
		return n // already in seconds
	default: // unit omitted ⇒ assume seconds
		return n
	}
}

// classifyBasicError uses the unified ProviderFailure classification system
// and converts the result to a ModelError for backwards compatibility
func classifyBasicError(err error, isClaudeMax bool) shared.ModelError {
	return classifyBasicErrorWithProvider(err, isClaudeMax, "")
}

// classifyBasicErrorWithProvider is like classifyBasicError but also takes a provider hint
// for more accurate classification of provider-specific errors
func classifyBasicErrorWithProvider(err error, isClaudeMax bool, provider string) shared.ModelError {
	var httpCode int
	var body string
	var headers http.Header

	// Extract HTTP details if available
	if httpErr, ok := err.(*HTTPError); ok {
		httpCode = httpErr.StatusCode
		body = httpErr.Body
		headers = httpErr.Header
	} else {
		// Use error message as body for classification
		body = err.Error()
	}

	// For HTTP errors, use the full classification flow
	if httpCode > 0 {
		return classifyHTTPError(httpCode, body, headers, isClaudeMax, provider)
	}

	// For non-HTTP errors (plain error messages), use message-based classification first
	// This preserves the original behavior of ClassifyErrMsg

	// First, check for non-retryable context errors
	if isNonRetriableBasicErr(err) {
		return shared.ModelError{Kind: shared.ErrOther, Retriable: false}
	}

	// Try message-based classification
	msgRes := ClassifyErrMsg(body)
	if msgRes != nil {
		log.Printf("classifyBasicError: classified via message: kind=%s, retriable=%v",
			msgRes.Kind, msgRes.Retriable)
		return *msgRes
	}

	// For unknown errors without HTTP code, assume retryable (generic transient errors)
	// This preserves the original behavior for things like "temporary network failure"
	log.Printf("classifyBasicError: unknown error without HTTP code - assuming retriable")
	return shared.ModelError{Kind: shared.ErrOther, Retriable: true}
}

// classifyHTTPError handles classification for HTTP errors with status codes
func classifyHTTPError(httpCode int, body string, headers http.Header, isClaudeMax bool, provider string) shared.ModelError {
	// Use unified classification from ProviderFailure
	pf := shared.ClassifyProviderFailure(httpCode, "", body, provider)

	// Handle Claude Max special case: 429 = subscription quota exhausted
	if isClaudeMax && httpCode == 429 {
		pf.Type = shared.FailureTypeQuotaExhausted
		// Check for retry-after to determine if it's retriable
		retryAfter := extractRetryAfter(headers, body)
		if retryAfter > 0 && retryAfter <= MAX_RETRY_DELAY_SECONDS {
			pf.Retryable = true
			pf.RetryAfterSeconds = retryAfter
		} else {
			pf.Retryable = false
		}
	}

	// Extract retry-after for retryable errors
	if pf.Retryable && pf.RetryAfterSeconds == 0 {
		retryAfter := extractRetryAfter(headers, body)
		if retryAfter > MAX_RETRY_DELAY_SECONDS {
			// If retry-after is too long, mark as non-retryable
			log.Printf("Retry after %d seconds exceeds max delay of %d seconds - marking as non-retryable",
				retryAfter, MAX_RETRY_DELAY_SECONDS)
			pf.Retryable = false
		} else if retryAfter > 0 {
			pf.RetryAfterSeconds = retryAfter
		}
	}

	// Convert to ModelError for backwards compatibility
	result := shared.FromProviderFailure(pf)
	if result == nil {
		// Fallback if conversion fails
		return shared.ModelError{
			Kind:      shared.ErrOther,
			Retriable: false,
		}
	}

	log.Printf("classifyBasicError: type=%s, kind=%s, retriable=%v, retryAfter=%d",
		pf.Type, result.Kind, result.Retriable, result.RetryAfterSeconds)

	return *result
}

// classifyErrorToProviderFailure directly returns a ProviderFailure for cases
// where the full classification is needed (e.g., circuit breaker, journal logging)
func classifyErrorToProviderFailure(err error, provider string) *shared.ProviderFailure {
	var httpCode int
	var body string

	if httpErr, ok := err.(*HTTPError); ok {
		httpCode = httpErr.StatusCode
		body = httpErr.Body
	} else {
		body = err.Error()
	}

	return shared.ClassifyProviderFailure(httpCode, "", body, provider)
}

func isNonRetriableBasicErr(err error) bool {
	errStr := err.Error()

	// we don't want to retry on the errors below
	if strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "context canceled") {
		log.Println("Context deadline exceeded or canceled - no retry")
		return true
	}

	if strings.Contains(errStr, "status code: 400") &&
		strings.Contains(errStr, "reduce the length of the messages") {
		log.Println("Token limit exceeded - no retry")
		return true
	}

	if strings.Contains(errStr, "status code: 401") {
		log.Println("Invalid auth or api key - no retry")
		return true
	}

	if strings.Contains(errStr, "status code: 429") && strings.Contains(errStr, "exceeded your current quota") {
		log.Println("Current quota exceeded - no retry")
		return true
	}

	return false
}
