package shared

// =============================================================================
// OPERATION SAFETY CLASSIFICATION
// =============================================================================
//
// Before the retry loop re-executes an operation, it must determine whether
// doing so is safe.  An AI model request is idempotent — repeating it produces
// an equivalent result.  A file write backed by a checkpoint can be rolled
// back.  A destructive shell command (rm, deploy, external API POST) cannot.
//
// This classification prevents the retry engine from accidentally repeating
// side effects that the user did not intend.
//
// =============================================================================

// OperationSafety classifies how safe it is to re-execute an operation.
type OperationSafety int

const (
	// OperationSafe — the operation is idempotent or read-only.
	// Retrying produces an equivalent outcome with no side effects.
	// Examples: LLM inference requests, context loading, file reads.
	OperationSafe OperationSafety = iota

	// OperationConditional — the operation has side effects but can be
	// rolled back to a known good state (e.g. a checkpoint or git commit)
	// before retry.  The retry loop will roll back before re-executing.
	// Examples: file writes/edits when a pre-operation checkpoint exists.
	OperationConditional

	// OperationIrreversible — the operation cannot be undone.  Retrying
	// risks duplicating the side effect.  The retry loop will refuse to
	// re-execute unless RetryConfig.RetryIrreversible is explicitly true.
	// Examples: destructive shell commands, external API writes (deploy,
	// send notification), database mutations outside a transaction.
	OperationIrreversible
)

// String returns a human-readable label for logging and error messages.
func (s OperationSafety) String() string {
	switch s {
	case OperationSafe:
		return "safe"
	case OperationConditional:
		return "conditional"
	case OperationIrreversible:
		return "irreversible"
	default:
		return "unknown"
	}
}

// IsOperationSafe returns true if the retry loop is permitted to re-execute
// an operation with the given safety level under the provided config.
//
//   - Safe operations are always retryable.
//   - Conditional operations are retryable (the caller is responsible for
//     rolling back to a checkpoint before retry).
//   - Irreversible operations are blocked unless RetryConfig.RetryIrreversible
//     is explicitly set.
func IsOperationSafe(safety OperationSafety, config *RetryConfig) bool {
	switch safety {
	case OperationSafe, OperationConditional:
		return true
	case OperationIrreversible:
		return config != nil && config.RetryIrreversible
	default:
		return false
	}
}

// ClassifyOperation returns the safety level for well-known operation types.
// Unknown operation strings default to OperationSafe so that model-request
// retries (the most common case) work without explicit classification.
func ClassifyOperation(operationType string) OperationSafety {
	switch operationType {
	// Idempotent / read-only
	case "model_request", "context_load", "file_read", "checkpoint_create",
		"validation", "health_check":
		return OperationSafe

	// Side-effectful but rollback-able
	case "file_write", "file_edit", "file_delete", "file_move", "file_build",
		"context_update", "plan_update":
		return OperationConditional

	// Destructive / irreversible
	case "shell_exec", "shell_destructive", "external_api_write",
		"deploy", "send_notification", "external_webhook":
		return OperationIrreversible

	default:
		// Conservative default: treat unknown operations as safe so that
		// the retry engine doesn't silently block model requests that use
		// unrecognised operation labels.
		return OperationSafe
	}
}
