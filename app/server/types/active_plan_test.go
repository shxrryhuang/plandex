package types

import (
	"sync"
	"testing"
)

// TestActiveBuildState tests ActiveBuild state tracking
func TestActiveBuildState(t *testing.T) {
	t.Run("new build is not finished", func(t *testing.T) {
		build := &ActiveBuild{
			ReplyId: "reply-123",
			Path:    "src/main.go",
			Success: false,
			Error:   nil,
		}

		if build.BuildFinished() {
			t.Error("expected new build to not be finished")
		}
	})

	t.Run("successful build is finished", func(t *testing.T) {
		build := &ActiveBuild{
			ReplyId: "reply-456",
			Path:    "src/main.go",
			Success: true,
			Error:   nil,
		}

		if !build.BuildFinished() {
			t.Error("expected successful build to be finished")
		}
	})

	t.Run("build with error is finished", func(t *testing.T) {
		build := &ActiveBuild{
			ReplyId: "reply-789",
			Path:    "src/main.go",
			Success: false,
			Error:   &testError{msg: "build failed"},
		}

		if !build.BuildFinished() {
			t.Error("expected build with error to be finished")
		}
	})

	t.Run("file operation detection", func(t *testing.T) {
		tests := []struct {
			name     string
			build    *ActiveBuild
			isFileOp bool
		}{
			{
				name:     "regular file build",
				build:    &ActiveBuild{IsMoveOp: false, IsRemoveOp: false, IsResetOp: false},
				isFileOp: false,
			},
			{
				name:     "move operation",
				build:    &ActiveBuild{IsMoveOp: true, MoveDestination: "new/path.go"},
				isFileOp: true,
			},
			{
				name:     "remove operation",
				build:    &ActiveBuild{IsRemoveOp: true},
				isFileOp: true,
			},
			{
				name:     "reset operation",
				build:    &ActiveBuild{IsResetOp: true},
				isFileOp: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.build.IsFileOperation() != tt.isFileOp {
					t.Errorf("IsFileOperation() = %v, want %v", tt.build.IsFileOperation(), tt.isFileOp)
				}
			})
		}
	})
}

// testError implements error interface for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestActiveBuildTokenTracking tests token counting in builds
func TestActiveBuildTokenTracking(t *testing.T) {
	t.Run("track file content tokens", func(t *testing.T) {
		build := &ActiveBuild{
			Path:              "src/main.go",
			FileContent:       "package main\n\nfunc main() {}\n",
			FileContentTokens: 15,
			CurrentFileTokens: 10,
		}

		if build.FileContentTokens != 15 {
			t.Errorf("expected FileContentTokens 15, got %d", build.FileContentTokens)
		}
		if build.CurrentFileTokens != 10 {
			t.Errorf("expected CurrentFileTokens 10, got %d", build.CurrentFileTokens)
		}
	})

	t.Run("token tracking for multiple builds", func(t *testing.T) {
		builds := []*ActiveBuild{
			{Path: "file1.go", FileContentTokens: 100},
			{Path: "file2.go", FileContentTokens: 200},
			{Path: "file3.go", FileContentTokens: 150},
		}

		totalTokens := 0
		for _, b := range builds {
			totalTokens += b.FileContentTokens
		}

		if totalTokens != 450 {
			t.Errorf("expected total tokens 450, got %d", totalTokens)
		}
	})
}

// TestBuildQueueManagement tests build queue operations
func TestBuildQueueManagement(t *testing.T) {
	t.Run("empty queue detection", func(t *testing.T) {
		queues := map[string][]*ActiveBuild{
			"file1.go": {},
			"file2.go": {{Success: true}},
		}

		// Check empty queue
		if len(queues["file1.go"]) != 0 {
			t.Error("expected file1.go queue to be empty")
		}

		// Check queue with finished build
		allFinished := true
		for _, build := range queues["file2.go"] {
			if !build.BuildFinished() {
				allFinished = false
				break
			}
		}
		if !allFinished {
			t.Error("expected all builds in file2.go queue to be finished")
		}
	})

	t.Run("queue with pending builds", func(t *testing.T) {
		queue := []*ActiveBuild{
			{Path: "file.go", Success: false, Error: nil},
			{Path: "file.go", Success: true, Error: nil},
		}

		pendingCount := 0
		for _, build := range queue {
			if !build.BuildFinished() {
				pendingCount++
			}
		}

		if pendingCount != 1 {
			t.Errorf("expected 1 pending build, got %d", pendingCount)
		}
	})
}

// TestConcurrentBuildAccess tests concurrent access to builds
func TestConcurrentBuildAccess(t *testing.T) {
	t.Run("concurrent build status updates", func(t *testing.T) {
		builds := make([]*ActiveBuild, 100)
		for i := range builds {
			builds[i] = &ActiveBuild{
				Path:    "file.go",
				Success: false,
			}
		}

		var wg sync.WaitGroup
		var mu sync.Mutex

		for i := range builds {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				mu.Lock()
				builds[idx].Success = true
				mu.Unlock()
			}(i)
		}

		wg.Wait()

		successCount := 0
		for _, b := range builds {
			if b.Success {
				successCount++
			}
		}

		if successCount != 100 {
			t.Errorf("expected 100 successful builds, got %d", successCount)
		}
	})
}

// TestBuildPathOperations tests path-related build operations
func TestBuildPathOperations(t *testing.T) {
	t.Run("move operation paths", func(t *testing.T) {
		build := &ActiveBuild{
			Path:            "old/path/file.go",
			IsMoveOp:        true,
			MoveDestination: "new/path/file.go",
		}

		if build.Path != "old/path/file.go" {
			t.Errorf("expected source path 'old/path/file.go', got %q", build.Path)
		}
		if build.MoveDestination != "new/path/file.go" {
			t.Errorf("expected destination 'new/path/file.go', got %q", build.MoveDestination)
		}
	})

	t.Run("build with description", func(t *testing.T) {
		build := &ActiveBuild{
			Path:            "src/utils/helper.go",
			FileDescription: "Helper utilities for string processing",
		}

		if build.FileDescription == "" {
			t.Error("expected file description to be set")
		}
	})
}

// TestBuildReplyTracking tests reply ID tracking in builds
func TestBuildReplyTracking(t *testing.T) {
	t.Run("builds share reply ID", func(t *testing.T) {
		replyId := "reply-abc123"
		builds := []*ActiveBuild{
			{ReplyId: replyId, Path: "file1.go"},
			{ReplyId: replyId, Path: "file2.go"},
			{ReplyId: replyId, Path: "file3.go"},
		}

		for _, b := range builds {
			if b.ReplyId != replyId {
				t.Errorf("expected ReplyId %q, got %q", replyId, b.ReplyId)
			}
		}
	})

	t.Run("different reply IDs for different responses", func(t *testing.T) {
		builds := []*ActiveBuild{
			{ReplyId: "reply-1", Path: "file1.go"},
			{ReplyId: "reply-2", Path: "file1.go"},
		}

		if builds[0].ReplyId == builds[1].ReplyId {
			t.Error("expected different reply IDs")
		}
	})
}

// TestBuildingByPathTracking tests IsBuildingByPath state
func TestBuildingByPathTracking(t *testing.T) {
	t.Run("track building state per path", func(t *testing.T) {
		isBuildingByPath := map[string]bool{
			"file1.go": true,
			"file2.go": false,
			"file3.go": true,
		}

		buildingCount := 0
		for _, isBuilding := range isBuildingByPath {
			if isBuilding {
				buildingCount++
			}
		}

		if buildingCount != 2 {
			t.Errorf("expected 2 files building, got %d", buildingCount)
		}
	})
}

// TestBuiltFilesTracking tests BuiltFiles state
func TestBuiltFilesTracking(t *testing.T) {
	t.Run("track built files", func(t *testing.T) {
		builtFiles := map[string]bool{}

		files := []string{"a.go", "b.go", "c.go"}
		for _, f := range files {
			builtFiles[f] = true
		}

		if len(builtFiles) != 3 {
			t.Errorf("expected 3 built files, got %d", len(builtFiles))
		}

		for _, f := range files {
			if !builtFiles[f] {
				t.Errorf("expected %q to be marked as built", f)
			}
		}
	})

	t.Run("check unbuilt file", func(t *testing.T) {
		builtFiles := map[string]bool{
			"built.go": true,
		}

		if builtFiles["unbuilt.go"] {
			t.Error("expected unbuilt.go to not be in built files")
		}
	})
}
