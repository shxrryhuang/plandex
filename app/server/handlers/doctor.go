package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"plandex-server/db"
	"runtime"
	"time"

	shared "plandex-shared"
)

// DoctorHandler handles health check and diagnostics requests
func DoctorHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request for DoctorHandler")

	auth := Authenticate(w, r, true)
	if auth == nil {
		return
	}

	// Parse request body
	var req shared.DoctorRequest
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) > 0 {
			json.Unmarshal(body, &req)
		}
		defer r.Body.Close()
	}

	response := shared.DoctorResponse{
		Healthy: true,
		Issues:  []shared.DoctorIssue{},
		Checks:  []shared.DoctorCheck{},
	}

	// Run health checks
	runHealthChecks(&response, auth.OrgId)

	// Check for stale locks
	checkStaleLocks(&response, auth.OrgId, req.PlanId)

	// Check for active locks
	checkActiveLocks(&response, auth.OrgId, req.PlanId)

	// Get server metrics
	response.ServerMetrics = getServerMetrics()

	// Fix issues if requested
	if req.Fix && len(response.StaleLocks) > 0 {
		fixStaleLocks(&response, auth.OrgId)
	}

	// Determine overall health
	for _, issue := range response.Issues {
		if issue.Severity == shared.SeverityError || issue.Severity == shared.SeverityCritical {
			response.Healthy = false
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func runHealthChecks(response *shared.DoctorResponse, orgId string) {
	// Database connection check
	dbCheck := shared.DoctorCheck{Name: "Database Connection"}
	start := time.Now()
	err := db.Conn.Ping()
	dbCheck.Latency = time.Since(start).Milliseconds()

	if err != nil {
		dbCheck.Status = shared.CheckStatusError
		dbCheck.Message = fmt.Sprintf("Database connection failed: %v", err)
		response.Issues = append(response.Issues, shared.DoctorIssue{
			Type:        shared.IssueTypeDBConnection,
			Severity:    shared.SeverityCritical,
			Description: "Cannot connect to database",
			Suggestion:  "Check database server status and connection settings",
		})
	} else {
		dbCheck.Status = shared.CheckStatusOK
		dbCheck.Message = fmt.Sprintf("Connected (latency: %dms)", dbCheck.Latency)
	}
	response.Checks = append(response.Checks, dbCheck)

	// Memory check
	memCheck := shared.DoctorCheck{Name: "Memory Usage"}
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memUsageMB := int64(memStats.Alloc / 1024 / 1024)

	if memUsageMB > 1500 {
		memCheck.Status = shared.CheckStatusWarning
		memCheck.Message = fmt.Sprintf("%dMB (high)", memUsageMB)
		response.Issues = append(response.Issues, shared.DoctorIssue{
			Type:        shared.IssueTypeHighMemory,
			Severity:    shared.SeverityWarning,
			Description: fmt.Sprintf("High memory usage: %dMB", memUsageMB),
			Suggestion:  "Consider restarting the server or increasing memory limits",
		})
	} else {
		memCheck.Status = shared.CheckStatusOK
		memCheck.Message = fmt.Sprintf("%dMB", memUsageMB)
	}
	response.Checks = append(response.Checks, memCheck)

	// Goroutine check
	goroutineCheck := shared.DoctorCheck{Name: "Goroutines"}
	goroutineCount := runtime.NumGoroutine()

	if goroutineCount > 1000 {
		goroutineCheck.Status = shared.CheckStatusWarning
		goroutineCheck.Message = fmt.Sprintf("%d (high)", goroutineCount)
		response.Issues = append(response.Issues, shared.DoctorIssue{
			Type:        shared.IssueTypeLongRunningOp,
			Severity:    shared.SeverityWarning,
			Description: fmt.Sprintf("High goroutine count: %d", goroutineCount),
			Suggestion:  "May indicate stuck operations or resource leaks",
		})
	} else {
		goroutineCheck.Status = shared.CheckStatusOK
		goroutineCheck.Message = fmt.Sprintf("%d", goroutineCount)
	}
	response.Checks = append(response.Checks, goroutineCheck)
}

func checkStaleLocks(response *shared.DoctorResponse, orgId string, planId string) {
	// Query for stale locks (heartbeat > 60 seconds ago)
	query := `
		SELECT
			rl.id, rl.plan_id, rl.scope, rl.branch, rl.last_heartbeat_at, rl.created_at,
			COALESCE(p.name, 'Unknown') as plan_name
		FROM repo_locks rl
		LEFT JOIN plans p ON p.id = rl.plan_id
		WHERE rl.org_id = $1
		AND rl.last_heartbeat_at < NOW() - INTERVAL '60 seconds'
	`
	args := []interface{}{orgId}

	if planId != "" {
		query += " AND rl.plan_id = $2"
		args = append(args, planId)
	}

	rows, err := db.Conn.Query(query, args...)
	if err != nil {
		log.Printf("Error querying stale locks: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var lockInfo staleLockRow
		err := rows.Scan(
			&lockInfo.Id, &lockInfo.PlanId, &lockInfo.Scope, &lockInfo.Branch,
			&lockInfo.LastHeartbeat, &lockInfo.CreatedAt, &lockInfo.PlanName,
		)
		if err != nil {
			log.Printf("Error scanning stale lock row: %v", err)
			continue
		}

		age := time.Since(lockInfo.LastHeartbeat)
		branch := ""
		if lockInfo.Branch != nil {
			branch = *lockInfo.Branch
		}

		staleLock := shared.StaleLockInfo{
			LockId:        lockInfo.Id,
			PlanId:        lockInfo.PlanId,
			PlanName:      lockInfo.PlanName,
			Branch:        branch,
			Scope:         lockInfo.Scope,
			LastHeartbeat: lockInfo.LastHeartbeat,
			Age:           formatDuration(age),
			AgeSeconds:    int64(age.Seconds()),
			CreatedAt:     lockInfo.CreatedAt,
		}

		response.StaleLocks = append(response.StaleLocks, staleLock)

		// Add as issue
		response.Issues = append(response.Issues, shared.DoctorIssue{
			Type:        shared.IssueTypeStaleLock,
			Severity:    shared.SeverityWarning,
			Description: fmt.Sprintf("Stale lock on plan '%s' (age: %s)", lockInfo.PlanName, staleLock.Age),
			Suggestion:  "Run 'plandex doctor --fix' to clean up stale locks",
			PlanId:      lockInfo.PlanId,
			LockId:      lockInfo.Id,
		})
	}
}

func checkActiveLocks(response *shared.DoctorResponse, orgId string, planId string) {
	// Query for active locks
	query := `
		SELECT
			rl.id, rl.plan_id, rl.scope, rl.branch, rl.last_heartbeat_at, rl.created_at,
			COALESCE(p.name, 'Unknown') as plan_name
		FROM repo_locks rl
		LEFT JOIN plans p ON p.id = rl.plan_id
		WHERE rl.org_id = $1
		AND rl.last_heartbeat_at >= NOW() - INTERVAL '60 seconds'
	`
	args := []interface{}{orgId}

	if planId != "" {
		query += " AND rl.plan_id = $2"
		args = append(args, planId)
	}

	rows, err := db.Conn.Query(query, args...)
	if err != nil {
		log.Printf("Error querying active locks: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var lockInfo staleLockRow
		err := rows.Scan(
			&lockInfo.Id, &lockInfo.PlanId, &lockInfo.Scope, &lockInfo.Branch,
			&lockInfo.LastHeartbeat, &lockInfo.CreatedAt, &lockInfo.PlanName,
		)
		if err != nil {
			log.Printf("Error scanning active lock row: %v", err)
			continue
		}

		branch := ""
		if lockInfo.Branch != nil {
			branch = *lockInfo.Branch
		}

		activeLock := shared.ActiveLockInfo{
			LockId:        lockInfo.Id,
			PlanId:        lockInfo.PlanId,
			PlanName:      lockInfo.PlanName,
			Branch:        branch,
			Scope:         lockInfo.Scope,
			LastHeartbeat: lockInfo.LastHeartbeat,
			CreatedAt:     lockInfo.CreatedAt,
		}

		response.ActiveLocks = append(response.ActiveLocks, activeLock)
	}
}

func fixStaleLocks(response *shared.DoctorResponse, orgId string) {
	for _, staleLock := range response.StaleLocks {
		_, err := db.Conn.Exec("DELETE FROM repo_locks WHERE id = $1 AND org_id = $2", staleLock.LockId, orgId)
		if err != nil {
			log.Printf("Error deleting stale lock %s: %v", staleLock.LockId, err)
			continue
		}
		response.FixedIssues = append(response.FixedIssues,
			fmt.Sprintf("Removed stale lock on plan '%s' (lock_id: %s)", staleLock.PlanName, staleLock.LockId[:8]))
	}

	// Clear the stale locks list since they're fixed
	if len(response.FixedIssues) > 0 {
		response.StaleLocks = []shared.StaleLockInfo{}
		// Remove stale lock issues
		filteredIssues := []shared.DoctorIssue{}
		for _, issue := range response.Issues {
			if issue.Type != shared.IssueTypeStaleLock {
				filteredIssues = append(filteredIssues, issue)
			}
		}
		response.Issues = filteredIssues
	}
}

func getServerMetrics() *shared.ServerMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &shared.ServerMetrics{
		GoroutineCount: runtime.NumGoroutine(),
		MemoryUsageMB:  int64(memStats.Alloc / 1024 / 1024),
		// Note: ActiveStreams and QueuedOperations would need to be tracked separately
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d hours %d minutes", hours, minutes)
}

type staleLockRow struct {
	Id            string
	PlanId        string
	PlanName      string
	Scope         string
	Branch        *string
	LastHeartbeat time.Time
	CreatedAt     time.Time
}
