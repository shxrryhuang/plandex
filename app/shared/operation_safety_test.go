package shared

import "testing"

func TestOperationSafety_String(t *testing.T) {
	tests := []struct {
		safety   OperationSafety
		expected string
	}{
		{OperationSafe, "safe"},
		{OperationConditional, "conditional"},
		{OperationIrreversible, "irreversible"},
		{OperationSafety(99), "unknown"},
	}

	for _, tc := range tests {
		if got := tc.safety.String(); got != tc.expected {
			t.Errorf("OperationSafety(%d).String() = %q, want %q", tc.safety, got, tc.expected)
		}
	}
}

func TestIsOperationSafe(t *testing.T) {
	permissiveConfig := &RetryConfig{RetryIrreversible: true}
	restrictiveConfig := &RetryConfig{RetryIrreversible: false}
	nilConfig := (*RetryConfig)(nil)

	tests := []struct {
		name     string
		safety   OperationSafety
		config   *RetryConfig
		expected bool
	}{
		{"safe + restrictive", OperationSafe, restrictiveConfig, true},
		{"safe + permissive", OperationSafe, permissiveConfig, true},
		{"safe + nil config", OperationSafe, nilConfig, true},
		{"conditional + restrictive", OperationConditional, restrictiveConfig, true},
		{"conditional + permissive", OperationConditional, permissiveConfig, true},
		{"irreversible + restrictive", OperationIrreversible, restrictiveConfig, false},
		{"irreversible + permissive", OperationIrreversible, permissiveConfig, true},
		{"irreversible + nil config", OperationIrreversible, nilConfig, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsOperationSafe(tc.safety, tc.config)
			if got != tc.expected {
				t.Errorf("IsOperationSafe(%s, config) = %v, want %v", tc.safety, got, tc.expected)
			}
		})
	}
}

func TestClassifyOperation(t *testing.T) {
	safeOps := []string{"model_request", "context_load", "file_read", "checkpoint_create", "validation", "health_check"}
	conditionalOps := []string{"file_write", "file_edit", "file_delete", "file_move", "file_build", "context_update", "plan_update"}
	irreversibleOps := []string{"shell_exec", "shell_destructive", "external_api_write", "deploy", "send_notification", "external_webhook"}

	for _, op := range safeOps {
		if got := ClassifyOperation(op); got != OperationSafe {
			t.Errorf("ClassifyOperation(%q) = %s, want safe", op, got)
		}
	}
	for _, op := range conditionalOps {
		if got := ClassifyOperation(op); got != OperationConditional {
			t.Errorf("ClassifyOperation(%q) = %s, want conditional", op, got)
		}
	}
	for _, op := range irreversibleOps {
		if got := ClassifyOperation(op); got != OperationIrreversible {
			t.Errorf("ClassifyOperation(%q) = %s, want irreversible", op, got)
		}
	}

	// Unknown defaults to safe
	if got := ClassifyOperation("totally_unknown_op"); got != OperationSafe {
		t.Errorf("ClassifyOperation(unknown) = %s, want safe (default)", got)
	}
}
