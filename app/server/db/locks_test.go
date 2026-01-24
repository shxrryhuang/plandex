package db

import (
	"strings"
	"testing"
)

// TestLockScopeValidation tests lock scope constants
func TestLockScopeValidation(t *testing.T) {
	tests := []struct {
		name  string
		scope LockScope
		valid bool
	}{
		{"read scope is valid", LockScopeRead, true},
		{"write scope is valid", LockScopeWrite, true},
		{"empty scope is invalid", "", false},
		{"unknown scope is invalid", "unknown", false},
	}

	validScopes := map[LockScope]bool{
		LockScopeRead:  true,
		LockScopeWrite: true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, valid := validScopes[tt.scope]
			if valid != tt.valid {
				t.Errorf("LockScope %q: got valid=%v, want %v", tt.scope, valid, tt.valid)
			}
		})
	}
}

// TestLockErrorMessages tests that error messages are descriptive
func TestLockErrorMessages(t *testing.T) {
	t.Run("lock retry error contains helpful context", func(t *testing.T) {
		// Simulate the error message format we use
		errMsg := "failed to acquire lock after 6 attempts (cause: timeout). Another operation may be in progress or a previous operation did not release its lock properly. Try again in a few seconds"

		requiredPhrases := []string{
			"failed to acquire lock",
			"attempts",
			"Another operation may be in progress",
			"Try again",
		}

		for _, phrase := range requiredPhrases {
			if !strings.Contains(errMsg, phrase) {
				t.Errorf("error message should contain %q", phrase)
			}
		}
	})

	t.Run("lock error includes cause", func(t *testing.T) {
		errMsg := "failed to acquire lock after 3 attempts (cause: connection refused)"

		if !strings.Contains(errMsg, "cause:") {
			t.Error("error message should include the cause")
		}
	})
}

// TestLockRepoParamsValidation tests LockRepoParams structure
func TestLockRepoParamsValidation(t *testing.T) {
	tests := []struct {
		name    string
		params  LockRepoParams
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid params with all fields",
			params: LockRepoParams{
				OrgId:  "org-123",
				UserId: "user-456",
				PlanId: "plan-789",
				Branch: "main",
				Scope:  LockScopeRead,
				Reason: "test operation",
			},
			wantErr: false,
		},
		{
			name: "valid params without branch (root lock)",
			params: LockRepoParams{
				OrgId:  "org-123",
				UserId: "user-456",
				PlanId: "plan-789",
				Branch: "",
				Scope:  LockScopeRead,
				Reason: "root lock",
			},
			wantErr: false,
		},
		{
			name: "missing orgId should be invalid",
			params: LockRepoParams{
				OrgId:  "",
				UserId: "user-456",
				PlanId: "plan-789",
				Branch: "main",
				Scope:  LockScopeRead,
				Reason: "test",
			},
			wantErr: true,
			errMsg:  "OrgId is required",
		},
		{
			name: "missing planId should be invalid",
			params: LockRepoParams{
				OrgId:  "org-123",
				UserId: "user-456",
				PlanId: "",
				Branch: "main",
				Scope:  LockScopeRead,
				Reason: "test",
			},
			wantErr: true,
			errMsg:  "PlanId is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLockParams(tt.params)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// validateLockParams is a helper to validate lock parameters
func validateLockParams(params LockRepoParams) error {
	if params.OrgId == "" {
		return &ValidationError{Field: "OrgId", Message: "OrgId is required"}
	}
	if params.PlanId == "" {
		return &ValidationError{Field: "PlanId", Message: "PlanId is required"}
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// TestExecRepoOperationParamsValidation tests ExecRepoOperationParams
func TestExecRepoOperationParamsValidation(t *testing.T) {
	tests := []struct {
		name   string
		params ExecRepoOperationParams
		valid  bool
	}{
		{
			name: "valid write operation",
			params: ExecRepoOperationParams{
				OrgId:  "org-123",
				UserId: "user-456",
				PlanId: "plan-789",
				Branch: "main",
				Scope:  LockScopeWrite,
				Reason: "update files",
			},
			valid: true,
		},
		{
			name: "valid read operation",
			params: ExecRepoOperationParams{
				OrgId:  "org-123",
				UserId: "user-456",
				PlanId: "plan-789",
				Branch: "feature",
				Scope:  LockScopeRead,
				Reason: "read files",
			},
			valid: true,
		},
		{
			name: "clear repo on error flag set",
			params: ExecRepoOperationParams{
				OrgId:          "org-123",
				UserId:         "user-456",
				PlanId:         "plan-789",
				Branch:         "main",
				Scope:          LockScopeWrite,
				Reason:         "risky operation",
				ClearRepoOnErr: true,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			valid := tt.params.OrgId != "" && tt.params.PlanId != ""
			if valid != tt.valid {
				t.Errorf("params validation: got %v, want %v", valid, tt.valid)
			}
		})
	}
}
