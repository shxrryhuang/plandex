package shared

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDoctorResponseSerialization(t *testing.T) {
	t.Run("serialize empty response", func(t *testing.T) {
		resp := DoctorResponse{
			Healthy: true,
			Issues:  []DoctorIssue{},
			Checks:  []DoctorCheck{},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var parsed DoctorResponse
		err = json.Unmarshal(data, &parsed)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if !parsed.Healthy {
			t.Error("Expected healthy to be true")
		}
	})

	t.Run("serialize response with stale locks", func(t *testing.T) {
		now := time.Now()
		resp := DoctorResponse{
			Healthy: false,
			Issues: []DoctorIssue{
				{
					Type:        IssueTypeStaleLock,
					Severity:    SeverityWarning,
					Description: "Stale lock on plan 'test'",
					Suggestion:  "Run doctor --fix",
					PlanId:      "plan-123",
					LockId:      "lock-456",
				},
			},
			Checks: []DoctorCheck{
				{
					Name:    "Database Connection",
					Status:  CheckStatusOK,
					Message: "Connected",
					Latency: 45,
				},
			},
			StaleLocks: []StaleLockInfo{
				{
					LockId:        "lock-456",
					PlanId:        "plan-123",
					PlanName:      "test",
					Branch:        "main",
					Scope:         "write",
					LastHeartbeat: now.Add(-2 * time.Hour),
					Age:           "2 hours",
					AgeSeconds:    7200,
					CreatedAt:     now.Add(-3 * time.Hour),
				},
			},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var parsed DoctorResponse
		err = json.Unmarshal(data, &parsed)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if parsed.Healthy {
			t.Error("Expected healthy to be false")
		}
		if len(parsed.StaleLocks) != 1 {
			t.Errorf("Expected 1 stale lock, got %d", len(parsed.StaleLocks))
		}
		if parsed.StaleLocks[0].Age != "2 hours" {
			t.Errorf("Expected age '2 hours', got '%s'", parsed.StaleLocks[0].Age)
		}
	})
}

func TestDoctorIssueTypes(t *testing.T) {
	issueTypes := []DoctorIssueType{
		IssueTypeStaleLock,
		IssueTypeOrphanedLock,
		IssueTypeLongRunningOp,
		IssueTypeQueueBacklog,
		IssueTypeHighMemory,
		IssueTypeDBConnection,
		IssueTypeHeartbeatMissed,
	}

	for _, issueType := range issueTypes {
		t.Run(string(issueType), func(t *testing.T) {
			issue := DoctorIssue{
				Type:        issueType,
				Severity:    SeverityWarning,
				Description: "Test issue",
			}

			data, err := json.Marshal(issue)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var parsed DoctorIssue
			err = json.Unmarshal(data, &parsed)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if parsed.Type != issueType {
				t.Errorf("Expected type %s, got %s", issueType, parsed.Type)
			}
		})
	}
}

func TestIssueSeverityLevels(t *testing.T) {
	severities := []IssueSeverity{
		SeverityInfo,
		SeverityWarning,
		SeverityError,
		SeverityCritical,
	}

	for _, severity := range severities {
		t.Run(string(severity), func(t *testing.T) {
			issue := DoctorIssue{
				Type:     IssueTypeStaleLock,
				Severity: severity,
			}

			data, err := json.Marshal(issue)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var parsed DoctorIssue
			err = json.Unmarshal(data, &parsed)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if parsed.Severity != severity {
				t.Errorf("Expected severity %s, got %s", severity, parsed.Severity)
			}
		})
	}
}

func TestCheckStatusValues(t *testing.T) {
	statuses := []CheckStatus{
		CheckStatusOK,
		CheckStatusWarning,
		CheckStatusError,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			check := DoctorCheck{
				Name:   "Test Check",
				Status: status,
			}

			data, err := json.Marshal(check)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var parsed DoctorCheck
			err = json.Unmarshal(data, &parsed)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if parsed.Status != status {
				t.Errorf("Expected status %s, got %s", status, parsed.Status)
			}
		})
	}
}

func TestConcurrencyErrorTypes(t *testing.T) {
	errorTypes := []ConcurrencyErrorType{
		ConcurrencyErrorLockTimeout,
		ConcurrencyErrorLockConflict,
		ConcurrencyErrorQueueFull,
		ConcurrencyErrorDeadlock,
		ConcurrencyErrorHeartbeatLost,
	}

	for _, errType := range errorTypes {
		t.Run(string(errType), func(t *testing.T) {
			concErr := ConcurrencyError{
				Type:    errType,
				Message: "Test error",
				PlanId:  "plan-123",
			}

			data, err := json.Marshal(concErr)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var parsed ConcurrencyError
			err = json.Unmarshal(data, &parsed)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if parsed.Type != errType {
				t.Errorf("Expected type %s, got %s", errType, parsed.Type)
			}
		})
	}
}

func TestServerMetrics(t *testing.T) {
	metrics := ServerMetrics{
		Uptime:           "2h 30m",
		GoroutineCount:   150,
		MemoryUsageMB:    256,
		ActiveStreams:    5,
		QueuedOperations: 3,
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed ServerMetrics
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.GoroutineCount != 150 {
		t.Errorf("Expected goroutine count 150, got %d", parsed.GoroutineCount)
	}
	if parsed.MemoryUsageMB != 256 {
		t.Errorf("Expected memory 256MB, got %d", parsed.MemoryUsageMB)
	}
}

func TestDoctorRequest(t *testing.T) {
	t.Run("serialize request with fix flag", func(t *testing.T) {
		req := DoctorRequest{
			PlanId: "plan-123",
			Fix:    true,
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var parsed DoctorRequest
		err = json.Unmarshal(data, &parsed)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if !parsed.Fix {
			t.Error("Expected fix to be true")
		}
		if parsed.PlanId != "plan-123" {
			t.Errorf("Expected plan ID 'plan-123', got '%s'", parsed.PlanId)
		}
	})

	t.Run("serialize empty request", func(t *testing.T) {
		req := DoctorRequest{}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var parsed DoctorRequest
		err = json.Unmarshal(data, &parsed)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if parsed.Fix {
			t.Error("Expected fix to be false by default")
		}
	})
}
