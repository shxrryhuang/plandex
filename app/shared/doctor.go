package shared

import "time"

// DoctorRequest is the request for the doctor endpoint
type DoctorRequest struct {
	PlanId string `json:"planId,omitempty"` // Optional: check specific plan
	Fix    bool   `json:"fix,omitempty"`    // Whether to fix issues
}

// DoctorResponse contains the health check results
type DoctorResponse struct {
	Healthy       bool                `json:"healthy"`
	Issues        []DoctorIssue       `json:"issues"`
	Checks        []DoctorCheck       `json:"checks"`
	StaleLocks    []StaleLockInfo     `json:"staleLocks,omitempty"`
	ActiveLocks   []ActiveLockInfo    `json:"activeLocks,omitempty"`
	FixedIssues   []string            `json:"fixedIssues,omitempty"`
	ServerMetrics *ServerMetrics      `json:"serverMetrics,omitempty"`
}

// DoctorIssue represents a detected problem
type DoctorIssue struct {
	Type        DoctorIssueType `json:"type"`
	Severity    IssueSeverity   `json:"severity"`
	Description string          `json:"description"`
	Suggestion  string          `json:"suggestion"`
	PlanId      string          `json:"planId,omitempty"`
	LockId      string          `json:"lockId,omitempty"`
}

// DoctorCheck represents a health check result
type DoctorCheck struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message,omitempty"`
	Latency int64       `json:"latency,omitempty"` // milliseconds
}

// StaleLockInfo contains information about a stale lock
type StaleLockInfo struct {
	LockId          string    `json:"lockId"`
	PlanId          string    `json:"planId"`
	PlanName        string    `json:"planName,omitempty"`
	Branch          string    `json:"branch,omitempty"`
	Scope           string    `json:"scope"`
	LastHeartbeat   time.Time `json:"lastHeartbeat"`
	Age             string    `json:"age"` // human-readable duration
	AgeSeconds      int64     `json:"ageSeconds"`
	CreatedAt       time.Time `json:"createdAt"`
}

// ActiveLockInfo contains information about an active lock
type ActiveLockInfo struct {
	LockId        string    `json:"lockId"`
	PlanId        string    `json:"planId"`
	PlanName      string    `json:"planName,omitempty"`
	Branch        string    `json:"branch,omitempty"`
	Scope         string    `json:"scope"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
	CreatedAt     time.Time `json:"createdAt"`
	Reason        string    `json:"reason,omitempty"`
}

// ServerMetrics contains server performance metrics
type ServerMetrics struct {
	Uptime          string `json:"uptime"`
	GoroutineCount  int    `json:"goroutineCount"`
	MemoryUsageMB   int64  `json:"memoryUsageMB"`
	ActiveStreams   int    `json:"activeStreams"`
	QueuedOperations int   `json:"queuedOperations"`
}

// DoctorIssueType categorizes the type of issue
type DoctorIssueType string

const (
	IssueTypeStaleLock       DoctorIssueType = "stale_lock"
	IssueTypeOrphanedLock    DoctorIssueType = "orphaned_lock"
	IssueTypeLongRunningOp   DoctorIssueType = "long_running_operation"
	IssueTypeQueueBacklog    DoctorIssueType = "queue_backlog"
	IssueTypeHighMemory      DoctorIssueType = "high_memory"
	IssueTypeDBConnection    DoctorIssueType = "db_connection"
	IssueTypeHeartbeatMissed DoctorIssueType = "heartbeat_missed"
)

// IssueSeverity indicates how critical an issue is
type IssueSeverity string

const (
	SeverityInfo     IssueSeverity = "info"
	SeverityWarning  IssueSeverity = "warning"
	SeverityError    IssueSeverity = "error"
	SeverityCritical IssueSeverity = "critical"
)

// CheckStatus indicates the result of a health check
type CheckStatus string

const (
	CheckStatusOK      CheckStatus = "ok"
	CheckStatusWarning CheckStatus = "warning"
	CheckStatusError   CheckStatus = "error"
)

// ConcurrencyError represents an error related to concurrent operations
type ConcurrencyError struct {
	Type           ConcurrencyErrorType `json:"type"`
	Message        string               `json:"message"`
	PlanId         string               `json:"planId,omitempty"`
	Branch         string               `json:"branch,omitempty"`
	QueuePosition  int                  `json:"queuePosition,omitempty"`
	EstimatedWait  int                  `json:"estimatedWaitSeconds,omitempty"`
	RetryAfter     int                  `json:"retryAfterSeconds,omitempty"`
	ActiveLockInfo *ActiveLockInfo      `json:"activeLockInfo,omitempty"`
}

// ConcurrencyErrorType categorizes concurrency-related errors
type ConcurrencyErrorType string

const (
	ConcurrencyErrorLockTimeout   ConcurrencyErrorType = "lock_timeout"
	ConcurrencyErrorLockConflict  ConcurrencyErrorType = "lock_conflict"
	ConcurrencyErrorQueueFull     ConcurrencyErrorType = "queue_full"
	ConcurrencyErrorDeadlock      ConcurrencyErrorType = "deadlock"
	ConcurrencyErrorHeartbeatLost ConcurrencyErrorType = "heartbeat_lost"
)
