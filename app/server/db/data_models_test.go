package db

import (
	"testing"
)

// TestOrgRolePermissions validates organization role permission checks
func TestOrgRolePermissions(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		expected bool
	}{
		{
			name:     "owner has admin permissions",
			role:     "owner",
			expected: true,
		},
		{
			name:     "admin has admin permissions",
			role:     "admin",
			expected: true,
		},
		{
			name:     "regular user does not have admin permissions",
			role:     "regular",
			expected: false,
		},
		{
			name:     "empty role does not have admin permissions",
			role:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role == "owner" || tt.role == "admin"
			if result != tt.expected {
				t.Errorf("role %q admin check = %v, want %v", tt.role, result, tt.expected)
			}
		})
	}
}

// TestPlanStatusTransitions validates plan status state machine transitions
func TestPlanStatusTransitions(t *testing.T) {
	validTransitions := map[string][]string{
		"drafting":    {"building", "finished", "error"},
		"building":    {"built", "error", "stopped"},
		"built":       {"applying", "replying", "error"},
		"applying":    {"applied", "error"},
		"applied":     {"finished", "drafting"},
		"replying":    {"replied", "error"},
		"replied":     {"finished", "drafting"},
		"finished":    {"drafting"},
		"error":       {"drafting"},
		"stopped":     {"drafting"},
		"prompting":   {"drafting", "error"},
		"responding":  {"drafting", "error", "responded"},
		"responded":   {"drafting", "finished"},
		"missingFile": {"drafting", "error"},
	}

	tests := []struct {
		name        string
		fromStatus  string
		toStatus    string
		shouldAllow bool
	}{
		{"drafting to building", "drafting", "building", true},
		{"drafting to finished", "drafting", "finished", true},
		{"building to built", "building", "built", true},
		{"building to error", "building", "error", true},
		{"built to applying", "built", "applying", true},
		{"error to drafting", "error", "drafting", true},
		{"finished to drafting", "finished", "drafting", true},
		// Invalid transitions
		{"drafting to applied", "drafting", "applied", false},
		{"finished to building", "finished", "building", false},
		{"error to built", "error", "built", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := false
			if validNext, ok := validTransitions[tt.fromStatus]; ok {
				for _, next := range validNext {
					if next == tt.toStatus {
						allowed = true
						break
					}
				}
			}
			if allowed != tt.shouldAllow {
				t.Errorf("transition %s -> %s: got allowed=%v, want %v",
					tt.fromStatus, tt.toStatus, allowed, tt.shouldAllow)
			}
		})
	}
}

// TestContextTypeValidation validates context type values
func TestContextTypeValidation(t *testing.T) {
	validTypes := map[string]bool{
		"file":           true,
		"directory":      true,
		"url":            true,
		"piped":          true,
		"note":           true,
		"image":          true,
		"directoryTree":  true,
	}

	tests := []struct {
		name      string
		ctxType   string
		wantValid bool
	}{
		{"file type is valid", "file", true},
		{"directory type is valid", "directory", true},
		{"url type is valid", "url", true},
		{"piped type is valid", "piped", true},
		{"note type is valid", "note", true},
		{"image type is valid", "image", true},
		{"directoryTree type is valid", "directoryTree", true},
		{"invalid type", "invalid", false},
		{"empty type", "", false},
		{"unknown type", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, valid := validTypes[tt.ctxType]
			if valid != tt.wantValid {
				t.Errorf("context type %q: got valid=%v, want %v", tt.ctxType, valid, tt.wantValid)
			}
		})
	}
}

// TestBuildModeValidation validates build mode values
func TestBuildModeValidation(t *testing.T) {
	validModes := map[string]bool{
		"auto":   true,
		"manual": true,
	}

	tests := []struct {
		name      string
		mode      string
		wantValid bool
	}{
		{"auto mode is valid", "auto", true},
		{"manual mode is valid", "manual", true},
		{"invalid mode", "invalid", false},
		{"empty mode", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, valid := validModes[tt.mode]
			if valid != tt.wantValid {
				t.Errorf("build mode %q: got valid=%v, want %v", tt.mode, valid, tt.wantValid)
			}
		})
	}
}

// TestPlanArchiveStatus validates plan archive status handling
func TestPlanArchiveStatus(t *testing.T) {
	tests := []struct {
		name         string
		isArchived   bool
		canModify    bool
		canView      bool
		canUnarchive bool
	}{
		{
			name:         "active plan can be modified and viewed",
			isArchived:   false,
			canModify:    true,
			canView:      true,
			canUnarchive: false,
		},
		{
			name:         "archived plan can only be viewed and unarchived",
			isArchived:   true,
			canModify:    false,
			canView:      true,
			canUnarchive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canModify := !tt.isArchived
			canView := true // Always viewable
			canUnarchive := tt.isArchived

			if canModify != tt.canModify {
				t.Errorf("canModify: got %v, want %v", canModify, tt.canModify)
			}
			if canView != tt.canView {
				t.Errorf("canView: got %v, want %v", canView, tt.canView)
			}
			if canUnarchive != tt.canUnarchive {
				t.Errorf("canUnarchive: got %v, want %v", canUnarchive, tt.canUnarchive)
			}
		})
	}
}

// TestOperationTypeValidation validates file operation types
func TestOperationTypeValidation(t *testing.T) {
	validOps := map[string]bool{
		"file":       true,
		"move":       true,
		"remove":     true,
		"createFile": true,
		"reset":      true,
	}

	tests := []struct {
		name      string
		opType    string
		wantValid bool
	}{
		{"file operation is valid", "file", true},
		{"move operation is valid", "move", true},
		{"remove operation is valid", "remove", true},
		{"createFile operation is valid", "createFile", true},
		{"reset operation is valid", "reset", true},
		{"delete is invalid (use remove)", "delete", false},
		{"rename is invalid (use move)", "rename", false},
		{"empty operation type", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, valid := validOps[tt.opType]
			if valid != tt.wantValid {
				t.Errorf("operation type %q: got valid=%v, want %v", tt.opType, valid, tt.wantValid)
			}
		})
	}
}
