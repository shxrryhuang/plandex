package shared

import (
	"strings"
	"testing"
)

func TestSanitizeString_OpenAIKey(t *testing.T) {
	tests := []struct {
		input    string
		level    SanitizeLevel
		expected string
	}{
		{
			input:    "Error with key sk-1234567890abcdefghijklmnopqrstuv",
			level:    SanitizeLevelStandard,
			expected: "Error with key [REDACTED_OPENAI_KEY]",
		},
		{
			input:    "sk-proj-abc123def456ghi789jkl012mno345_pqr678",
			level:    SanitizeLevelStandard,
			expected: "[REDACTED_OPENAI_PROJECT_KEY]",
		},
		{
			input:    "Error with key sk-1234567890abcdefghijklmnopqrstuv",
			level:    SanitizeLevelNone,
			expected: "Error with key sk-1234567890abcdefghijklmnopqrstuv", // No change
		},
	}

	for _, tc := range tests {
		result := SanitizeString(tc.input, tc.level)
		if result != tc.expected {
			t.Errorf("SanitizeString(%q, %v) = %q, want %q", tc.input, tc.level, result, tc.expected)
		}
	}
}

func TestSanitizeString_AnthropicKey(t *testing.T) {
	input := "Using key sk-ant-api03-abcdefghijklmnopqrstuvwxyz"
	result := SanitizeString(input, SanitizeLevelStandard)

	if !strings.Contains(result, "[REDACTED_ANTHROPIC_KEY]") {
		t.Errorf("Should redact Anthropic key, got: %s", result)
	}
	if strings.Contains(result, "sk-ant-") {
		t.Errorf("Should not contain original key, got: %s", result)
	}
}

func TestSanitizeString_GoogleKey(t *testing.T) {
	// Google API keys are 39 characters: AIza + 35 chars
	// Example: AIzaSyAabcdefghijklmnopqrstuvwxyz123456 (39 total)
	input := "Google key: AIzaSyAabcdefghijklmnopqrstuvwxyz123456"
	result := SanitizeString(input, SanitizeLevelStandard)

	if !strings.Contains(result, "[REDACTED_GOOGLE_KEY]") {
		t.Errorf("Should redact Google key, got: %s", result)
	}
}

func TestSanitizeString_BearerToken(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	result := SanitizeString(input, SanitizeLevelStandard)

	if !strings.Contains(result, "Bearer [REDACTED]") {
		t.Errorf("Should redact Bearer token, got: %s", result)
	}
}

func TestSanitizeString_JWT(t *testing.T) {
	input := "Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	result := SanitizeString(input, SanitizeLevelStandard)

	if !strings.Contains(result, "[REDACTED_JWT]") {
		t.Errorf("Should redact JWT, got: %s", result)
	}
}

func TestSanitizeString_Password(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"password=mysecretpassword"},
		{"PASSWORD:mysecretpassword"},
		{"pwd=hunter2andmore"},
	}

	for _, tc := range tests {
		result := SanitizeString(tc.input, SanitizeLevelStandard)
		if !strings.Contains(result, "[REDACTED]") {
			t.Errorf("Should redact password in %q, got: %s", tc.input, result)
		}
	}
}

func TestSanitizeString_ConnectionString(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"mongodb://user:password@localhost:27017/db"},
		{"postgres://admin:secret@host:5432/mydb"},
		{"mysql://root:pass123@127.0.0.1:3306/test"},
	}

	for _, tc := range tests {
		result := SanitizeString(tc.input, SanitizeLevelStandard)
		if !strings.Contains(result, "[REDACTED]@") {
			t.Errorf("Should redact connection string in %q, got: %s", tc.input, result)
		}
	}
}

func TestSanitizeString_AWSKeys(t *testing.T) {
	// AWS access key
	input := "Access key: AKIAIOSFODNN7EXAMPLE"
	result := SanitizeString(input, SanitizeLevelStandard)

	if !strings.Contains(result, "[REDACTED_AWS_KEY]") {
		t.Errorf("Should redact AWS access key, got: %s", result)
	}
}

func TestSanitizeString_GitHubToken(t *testing.T) {
	tests := []struct {
		input       string
		shouldMatch string
	}{
		{"Token: ghp_abcdefghijklmnopqrstuvwxyz1234567890", "[REDACTED_GITHUB_TOKEN]"},
		{"Token: ghs_abcdefghijklmnopqrstuvwxyz1234567890", "[REDACTED_GITHUB_TOKEN]"},
		{"Token: gho_abcdefghijklmnopqrstuvwxyz1234567890", "[REDACTED_GITHUB_OAUTH]"},
	}

	for _, tc := range tests {
		result := SanitizeString(tc.input, SanitizeLevelStandard)
		if !strings.Contains(result, tc.shouldMatch) {
			t.Errorf("Should contain %s for input %q, got: %s", tc.shouldMatch, tc.input, result)
		}
	}
}

func TestSanitizeString_Paths_Standard(t *testing.T) {
	// Standard level should NOT redact paths
	input := "/Users/john/projects/myapp/file.txt"
	result := SanitizeString(input, SanitizeLevelStandard)

	if result != input {
		t.Errorf("Standard level should not redact paths, got: %s", result)
	}
}

func TestSanitizeString_Paths_Strict(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/john/projects/file.txt", "~/projects/file.txt"},
		{"/home/jane/code/app.go", "~/code/app.go"},
		{`C:\Users\admin\Documents\file.txt`, `C:\Users\[USER]\Documents\file.txt`},
	}

	for _, tc := range tests {
		result := SanitizeString(tc.input, SanitizeLevelStrict)
		if result != tc.expected {
			t.Errorf("SanitizeString(%q, Strict) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestSanitizeString_IP_Strict(t *testing.T) {
	// Should redact non-localhost IPs
	input := "Connecting to 192.168.1.100:8080"
	result := SanitizeString(input, SanitizeLevelStrict)

	if !strings.Contains(result, "[REDACTED_IP]") {
		t.Errorf("Should redact IP address, got: %s", result)
	}

	// Should NOT redact localhost
	input = "Connecting to 127.0.0.1:8080"
	result = SanitizeString(input, SanitizeLevelStrict)

	if strings.Contains(result, "[REDACTED_IP]") {
		t.Errorf("Should NOT redact localhost, got: %s", result)
	}
}

func TestSanitizeString_Email_Strict(t *testing.T) {
	input := "Contact user@example.com for support"
	result := SanitizeString(input, SanitizeLevelStrict)

	if !strings.Contains(result, "[REDACTED_EMAIL]") {
		t.Errorf("Should redact email at strict level, got: %s", result)
	}

	// Standard should NOT redact email
	result = SanitizeString(input, SanitizeLevelStandard)
	if strings.Contains(result, "[REDACTED_EMAIL]") {
		t.Errorf("Standard should NOT redact email, got: %s", result)
	}
}

func TestSanitizeError(t *testing.T) {
	report := &ErrorReport{
		Id: "err_123",
		RootCause: &RootCause{
			Category:      ErrorCategoryProvider,
			Type:          "auth_invalid",
			Code:          "AUTH_ERROR",
			Message:       "Invalid API key: sk-1234567890abcdefghijklmnopqrstuv",
			OriginalError: "Error with key sk-1234567890abcdefghijklmnopqrstuv",
		},
		StepContext: &StepContext{
			PlanId:   "plan-123",
			FilePath: "/Users/john/project/file.txt",
		},
		Recovery: &RecoveryAction{
			ManualActions: []ManualAction{
				{
					Description: "Update API key sk-1234567890abcdefghijklmnopqrstuv",
					Command:     "export OPENAI_API_KEY=sk-newkey...",
				},
			},
		},
	}

	sanitized := SanitizeError(report, SanitizeLevelStandard)

	// Check message is sanitized
	if strings.Contains(sanitized.RootCause.Message, "sk-1234") {
		t.Errorf("Message should be sanitized, got: %s", sanitized.RootCause.Message)
	}
	if !strings.Contains(sanitized.RootCause.Message, "[REDACTED_OPENAI_KEY]") {
		t.Errorf("Message should contain redaction marker, got: %s", sanitized.RootCause.Message)
	}

	// Check original is not modified
	if !strings.Contains(report.RootCause.Message, "sk-1234") {
		t.Error("Original report should not be modified")
	}

	// Check manual actions are sanitized
	if strings.Contains(sanitized.Recovery.ManualActions[0].Description, "sk-1234") {
		t.Errorf("Manual action description should be sanitized")
	}
}

func TestSanitizeError_Nil(t *testing.T) {
	result := SanitizeError(nil, SanitizeLevelStandard)
	if result != nil {
		t.Error("SanitizeError(nil) should return nil")
	}
}

func TestSanitizeError_NoneLevel(t *testing.T) {
	report := &ErrorReport{
		Id: "err_123",
		RootCause: &RootCause{
			Message: "Key: sk-1234567890abcdefghijklmnopqrstuv",
		},
	}

	result := SanitizeError(report, SanitizeLevelNone)

	// Should return same report
	if result != report {
		t.Error("SanitizeLevelNone should return original report")
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sk-1234567890abcdefghijklmnopqrstuv", "sk-1234...stuv"},
		{"short", "[HIDDEN]"},
		{"1234567890ab", "[HIDDEN]"}, // 12 chars exactly
		{"1234567890abc", "1234567...0abc"}, // 13 chars
	}

	for _, tc := range tests {
		result := MaskAPIKey(tc.input)
		if result != tc.expected {
			t.Errorf("MaskAPIKey(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestContainsSensitiveData(t *testing.T) {
	tests := []struct {
		input    string
		contains bool
	}{
		{"sk-1234567890abcdefghijklmnopqrstuv", true},
		{"Bearer token123456789012345678901", true},
		{"Just a normal string", false},
		{"password=secret123456", true},
		{"AKIAIOSFODNN7EXAMPLE", true},
	}

	for _, tc := range tests {
		result := ContainsSensitiveData(tc.input)
		if result != tc.contains {
			t.Errorf("ContainsSensitiveData(%q) = %v, want %v", tc.input, result, tc.contains)
		}
	}
}

func TestGetSensitivePatternNames(t *testing.T) {
	input := "Key: sk-1234567890abcdefghijklmnopqrstuv, Bearer token123456789012345678901"
	names := GetSensitivePatternNames(input)

	if len(names) == 0 {
		t.Error("Should find sensitive patterns")
	}

	foundOpenAI := false
	foundBearer := false
	for _, name := range names {
		if name == "openai_key" {
			foundOpenAI = true
		}
		if name == "bearer_token" {
			foundBearer = true
		}
	}

	if !foundOpenAI {
		t.Error("Should find openai_key pattern")
	}
	if !foundBearer {
		t.Error("Should find bearer_token pattern")
	}
}

func TestSanitizeMap(t *testing.T) {
	input := map[string]interface{}{
		"api_key": "sk-1234567890abcdefghijklmnopqrstuv",
		"count":   42,
		"nested": map[string]interface{}{
			"password": "password=secret123456",
		},
		"list": []interface{}{
			"sk-abc1234567890defghijklmnopqrstuv",
			"normal string",
		},
	}

	result := SanitizeMap(input, SanitizeLevelStandard)

	// Check top-level string
	if apiKey, ok := result["api_key"].(string); ok {
		if strings.Contains(apiKey, "sk-1234") {
			t.Errorf("api_key should be sanitized, got: %s", apiKey)
		}
	}

	// Check nested
	if nested, ok := result["nested"].(map[string]interface{}); ok {
		if pwd, ok := nested["password"].(string); ok {
			if strings.Contains(pwd, "secret") {
				t.Errorf("nested password should be sanitized, got: %s", pwd)
			}
		}
	}

	// Check count unchanged
	if count, ok := result["count"].(int); ok {
		if count != 42 {
			t.Errorf("count should be unchanged, got: %d", count)
		}
	}
}

func TestParseSanitizeLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected SanitizeLevel
	}{
		{"none", SanitizeLevelNone},
		{"None", SanitizeLevelNone},
		{"NONE", SanitizeLevelNone},
		{"0", SanitizeLevelNone},
		{"standard", SanitizeLevelStandard},
		{"Standard", SanitizeLevelStandard},
		{"1", SanitizeLevelStandard},
		{"strict", SanitizeLevelStrict},
		{"STRICT", SanitizeLevelStrict},
		{"2", SanitizeLevelStrict},
		{"unknown", SanitizeLevelStandard}, // Default
	}

	for _, tc := range tests {
		result := ParseSanitizeLevel(tc.input)
		if result != tc.expected {
			t.Errorf("ParseSanitizeLevel(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

func TestSanitizeLevel_String(t *testing.T) {
	tests := []struct {
		level    SanitizeLevel
		expected string
	}{
		{SanitizeLevelNone, "none"},
		{SanitizeLevelStandard, "standard"},
		{SanitizeLevelStrict, "strict"},
		{SanitizeLevel(99), "unknown"},
	}

	for _, tc := range tests {
		result := tc.level.String()
		if result != tc.expected {
			t.Errorf("SanitizeLevel(%d).String() = %q, want %q", tc.level, result, tc.expected)
		}
	}
}

func TestSanitizeString_MultiplePatterns(t *testing.T) {
	// String with multiple sensitive items
	input := "Config: api_key=sk-1234567890abcdefghijklmnopqrstuv, token=Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	result := SanitizeString(input, SanitizeLevelStandard)

	if strings.Contains(result, "sk-1234") {
		t.Errorf("Should redact OpenAI key, got: %s", result)
	}
	if strings.Contains(result, "eyJ") {
		t.Errorf("Should redact JWT, got: %s", result)
	}
}

func TestSanitizeError_WithModelContext(t *testing.T) {
	report := &ErrorReport{
		Id: "err_123",
		RootCause: &RootCause{
			Message: "Error",
		},
		StepContext: &StepContext{
			PlanId: "plan-123",
			ModelContext: &ModelContext{
				Model:           "gpt-4",
				Provider:        "openai",
				PartialResponse: "Response with key sk-1234567890abcdefghijklmnopqrstuv",
			},
		},
	}

	sanitized := SanitizeError(report, SanitizeLevelStandard)

	if strings.Contains(sanitized.StepContext.ModelContext.PartialResponse, "sk-1234") {
		t.Errorf("PartialResponse should be sanitized")
	}
}

func TestSanitizeError_WithRelatedErrors(t *testing.T) {
	related := &ErrorReport{
		Id: "err_456",
		RootCause: &RootCause{
			Message: "Related error with sk-1234567890abcdefghijklmnopqrstuv",
		},
	}

	report := &ErrorReport{
		Id: "err_123",
		RootCause: &RootCause{
			Message: "Main error",
		},
		RelatedErrors: []*ErrorReport{related},
	}

	sanitized := SanitizeError(report, SanitizeLevelStandard)

	if len(sanitized.RelatedErrors) != 1 {
		t.Fatal("Should have 1 related error")
	}

	if strings.Contains(sanitized.RelatedErrors[0].RootCause.Message, "sk-1234") {
		t.Errorf("Related error should be sanitized")
	}
}
