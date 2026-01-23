package handlers

import (
	"testing"
)

// TestEmailValidation tests email format validation
func TestEmailValidation(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		isValid bool
	}{
		{"valid email", "user@example.com", true},
		{"valid email with subdomain", "user@mail.example.com", true},
		{"valid email with plus", "user+tag@example.com", true},
		{"valid email with dots", "first.last@example.com", true},
		{"missing @", "userexample.com", false},
		{"missing domain", "user@", false},
		{"missing local part", "@example.com", false},
		{"empty string", "", false},
		{"spaces in email", "user @example.com", false},
		{"multiple @", "user@@example.com", false},
		{"valid with numbers", "user123@example123.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic email validation check
			valid := len(tt.email) > 0 &&
				containsExactlyOne(tt.email, '@') &&
				!containsSpace(tt.email)

			if tt.email != "" {
				parts := splitAt(tt.email, '@')
				if len(parts) == 2 {
					valid = valid && len(parts[0]) > 0 && len(parts[1]) > 0
				}
			}

			if valid != tt.isValid {
				t.Errorf("email %q: got valid=%v, want %v", tt.email, valid, tt.isValid)
			}
		})
	}
}

// Helper functions for validation tests
func containsExactlyOne(s string, char rune) bool {
	count := 0
	for _, c := range s {
		if c == char {
			count++
		}
	}
	return count == 1
}

func containsSpace(s string) bool {
	for _, c := range s {
		if c == ' ' {
			return true
		}
	}
	return false
}

func splitAt(s string, char rune) []string {
	result := make([]string, 0, 2)
	current := ""
	for _, c := range s {
		if c == char {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	result = append(result, current)
	return result
}

// TestPlanNameValidation tests plan name validation
func TestPlanNameValidation(t *testing.T) {
	tests := []struct {
		name      string
		planName  string
		isValid   bool
		errReason string
	}{
		{"valid simple name", "my-plan", true, ""},
		{"valid with numbers", "plan123", true, ""},
		{"valid with underscores", "my_plan_name", true, ""},
		{"valid with dots", "plan.v1", true, ""},
		{"empty name", "", false, "name cannot be empty"},
		{"too long", createStringOfLength(300, 'x'), false, "name too long"},
		{"has newline", "plan\nname", false, "invalid characters"},
		{"has tab", "plan\tname", false, "invalid characters"},
		{"valid max length", createStringOfLength(255, 'a'), true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var valid bool
			var errReason string

			if len(tt.planName) == 0 {
				valid = false
				errReason = "name cannot be empty"
			} else if len(tt.planName) > 255 {
				valid = false
				errReason = "name too long"
			} else if containsControlChars(tt.planName) {
				valid = false
				errReason = "invalid characters"
			} else {
				valid = true
			}

			if valid != tt.isValid {
				t.Errorf("planName %q: got valid=%v, want %v", tt.planName, valid, tt.isValid)
			}
			if !valid && errReason != tt.errReason {
				t.Errorf("planName %q: got errReason=%q, want %q", tt.planName, errReason, tt.errReason)
			}
		})
	}
}

func createStringOfLength(length int, char rune) string {
	result := make([]rune, length)
	for i := 0; i < length; i++ {
		result[i] = char
	}
	return string(result)
}

func containsControlChars(s string) bool {
	for _, c := range s {
		if c < 32 || c == 127 {
			return true
		}
	}
	return false
}

// TestBranchNameValidation tests branch name validation
func TestBranchNameValidation(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		isValid    bool
	}{
		{"valid main", "main", true},
		{"valid feature branch", "feature/new-feature", true},
		{"valid with numbers", "v1.2.3", true},
		{"valid with underscores", "feature_branch", true},
		{"empty name", "", false},
		{"starts with slash", "/branch", false},
		{"ends with slash", "branch/", false},
		{"double slash", "feature//name", false},
		{"has spaces", "feature branch", false},
		{"has special chars", "branch@name", false},
		{"dot dot sequence", "branch..name", false},
		{"ends with dot", "branch.", false},
		{"ends with .lock", "branch.lock", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidBranchName(tt.branchName)
			if valid != tt.isValid {
				t.Errorf("branchName %q: got valid=%v, want %v", tt.branchName, valid, tt.isValid)
			}
		})
	}
}

func isValidBranchName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if name[0] == '/' || name[len(name)-1] == '/' {
		return false
	}
	if name[len(name)-1] == '.' {
		return false
	}
	if len(name) >= 5 && name[len(name)-5:] == ".lock" {
		return false
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		// Check for invalid characters
		if c == ' ' || c == '~' || c == '^' || c == ':' || c == '?' || c == '*' || c == '[' || c == '\\' || c == '@' {
			return false
		}
		// Check for ..
		if c == '.' && i > 0 && name[i-1] == '.' {
			return false
		}
		// Check for //
		if c == '/' && i > 0 && name[i-1] == '/' {
			return false
		}
	}
	return true
}

// TestPathValidation tests file path validation
func TestPathValidation(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		isValid bool
	}{
		{"valid relative path", "src/main.go", true},
		{"valid nested path", "a/b/c/d/file.txt", true},
		{"valid hidden file", ".gitignore", true},
		{"valid hidden dir", ".config/settings.json", true},
		{"empty path", "", false},
		{"absolute path unix", "/etc/passwd", false},
		{"absolute path windows", "C:\\Users\\file", false},
		{"path traversal", "../outside/file", false},
		{"path traversal hidden", "a/../../../etc/passwd", false},
		{"null byte", "file\x00name", false},
		{"valid with spaces", "path with spaces/file.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidRelativePath(tt.path)
			if valid != tt.isValid {
				t.Errorf("path %q: got valid=%v, want %v", tt.path, valid, tt.isValid)
			}
		})
	}
}

func isValidRelativePath(path string) bool {
	if len(path) == 0 {
		return false
	}

	// Check for absolute paths
	if path[0] == '/' || path[0] == '\\' {
		return false
	}
	if len(path) >= 2 && path[1] == ':' {
		return false
	}

	// Check for null bytes
	for _, c := range path {
		if c == 0 {
			return false
		}
	}

	// Check for path traversal
	if containsPathTraversal(path) {
		return false
	}

	return true
}

func containsPathTraversal(path string) bool {
	// Simple check for .. at start or after /
	if len(path) >= 2 && path[:2] == ".." {
		return true
	}
	for i := 0; i < len(path)-2; i++ {
		if path[i] == '/' && path[i+1:i+3] == ".." {
			return true
		}
	}
	return false
}

// TestTokenValidation tests API token format validation
func TestTokenValidation(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		isValid bool
	}{
		{"valid token", "sk-abc123def456", true},
		{"valid long token", "sk-" + createStringOfLength(64, 'a'), true},
		{"empty token", "", false},
		{"too short", "sk-ab", false},
		{"missing prefix", "abc123def456", false},
		{"wrong prefix", "ak-abc123def456", false},
		{"has spaces", "sk-abc 123def456", false},
		{"has newlines", "sk-abc123\ndef456", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidAPIToken(tt.token)
			if valid != tt.isValid {
				t.Errorf("token: got valid=%v, want %v", valid, tt.isValid)
			}
		})
	}
}

func isValidAPIToken(token string) bool {
	if len(token) < 10 {
		return false
	}
	if len(token) < 3 || token[:3] != "sk-" {
		return false
	}
	for _, c := range token {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			return false
		}
	}
	return true
}

// TestHTTPMethodValidation tests HTTP method validation
func TestHTTPMethodValidation(t *testing.T) {
	validMethods := map[string]bool{
		"GET":     true,
		"POST":    true,
		"PUT":     true,
		"DELETE":  true,
		"PATCH":   true,
		"OPTIONS": true,
		"HEAD":    true,
	}

	tests := []struct {
		name    string
		method  string
		isValid bool
	}{
		{"GET is valid", "GET", true},
		{"POST is valid", "POST", true},
		{"PUT is valid", "PUT", true},
		{"DELETE is valid", "DELETE", true},
		{"PATCH is valid", "PATCH", true},
		{"lowercase get is invalid", "get", false},
		{"CONNECT is invalid", "CONNECT", false},
		{"TRACE is invalid", "TRACE", false},
		{"empty is invalid", "", false},
		{"custom method", "CUSTOM", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, valid := validMethods[tt.method]
			if valid != tt.isValid {
				t.Errorf("method %q: got valid=%v, want %v", tt.method, valid, tt.isValid)
			}
		})
	}
}

// TestContentTypeValidation tests content type validation
func TestContentTypeValidation(t *testing.T) {
	validContentTypes := map[string]bool{
		"application/json":                  true,
		"application/json; charset=utf-8":   true,
		"text/plain":                        true,
		"text/html":                         true,
		"multipart/form-data":               true,
		"application/x-www-form-urlencoded": true,
	}

	tests := []struct {
		name        string
		contentType string
		isJSON      bool
	}{
		{"json type", "application/json", true},
		{"json with charset", "application/json; charset=utf-8", true},
		{"text plain", "text/plain", false},
		{"form data", "multipart/form-data", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isJSON := len(tt.contentType) >= 16 && tt.contentType[:16] == "application/json"
			if isJSON != tt.isJSON {
				t.Errorf("contentType %q: got isJSON=%v, want %v", tt.contentType, isJSON, tt.isJSON)
			}
			_, _ = validContentTypes[tt.contentType]
		})
	}
}
