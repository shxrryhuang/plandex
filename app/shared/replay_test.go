package shared

import (
	"testing"
	"time"
)

func TestReplayStepIsDestructive(t *testing.T) {
	tests := []struct {
		name      string
		stepType  ReplayStepType
		buildInfo *ReplayBuildInfo
		expected  bool
	}{
		{
			name:     "model request is not destructive",
			stepType: ReplayStepTypeModelRequest,
			expected: false,
		},
		{
			name:     "model response is not destructive",
			stepType: ReplayStepTypeModelResponse,
			expected: false,
		},
		{
			name:     "file diff is destructive",
			stepType: ReplayStepTypeFileDiff,
			expected: true,
		},
		{
			name:     "file write is destructive",
			stepType: ReplayStepTypeFileWrite,
			expected: true,
		},
		{
			name:     "file remove is destructive",
			stepType: ReplayStepTypeFileRemove,
			expected: true,
		},
		{
			name:     "file move is destructive",
			stepType: ReplayStepTypeFileMove,
			expected: true,
		},
		{
			name:     "build start is not destructive",
			stepType: ReplayStepTypeBuildStart,
			expected: false,
		},
		{
			name:      "successful build complete is destructive",
			stepType:  ReplayStepTypeBuildComplete,
			buildInfo: &ReplayBuildInfo{Success: true},
			expected:  true,
		},
		{
			name:      "failed build complete is not destructive",
			stepType:  ReplayStepTypeBuildComplete,
			buildInfo: &ReplayBuildInfo{Success: false},
			expected:  false,
		},
		{
			name:     "context load is not destructive",
			stepType: ReplayStepTypeContextLoad,
			expected: false,
		},
		{
			name:     "subtask start is not destructive",
			stepType: ReplayStepTypeSubtaskStart,
			expected: false,
		},
		{
			name:     "user prompt is not destructive",
			stepType: ReplayStepTypeUserPrompt,
			expected: false,
		},
		{
			name:     "error is not destructive",
			stepType: ReplayStepTypeError,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &ReplayStep{
				Type:      tt.stepType,
				BuildInfo: tt.buildInfo,
			}
			result := step.IsDestructive()
			if result != tt.expected {
				t.Errorf("IsDestructive() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestReplayStepIsModelInteraction(t *testing.T) {
	tests := []struct {
		stepType ReplayStepType
		expected bool
	}{
		{ReplayStepTypeModelRequest, true},
		{ReplayStepTypeModelResponse, true},
		{ReplayStepTypeFileDiff, false},
		{ReplayStepTypeFileWrite, false},
		{ReplayStepTypeBuildStart, false},
		{ReplayStepTypeBuildComplete, false},
		{ReplayStepTypeContextLoad, false},
		{ReplayStepTypeSubtaskStart, false},
		{ReplayStepTypeUserPrompt, false},
		{ReplayStepTypeError, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.stepType), func(t *testing.T) {
			step := &ReplayStep{Type: tt.stepType}
			result := step.IsModelInteraction()
			if result != tt.expected {
				t.Errorf("IsModelInteraction() = %v, want %v for type %s", result, tt.expected, tt.stepType)
			}
		})
	}
}

func TestReplayStepGetDuration(t *testing.T) {
	t.Run("completed step", func(t *testing.T) {
		startTime := time.Now()
		endTime := startTime.Add(5 * time.Second)
		step := &ReplayStep{
			StartedAt:   startTime,
			CompletedAt: &endTime,
		}

		duration := step.GetDuration()
		if duration != 5*time.Second {
			t.Errorf("GetDuration() = %v, want %v", duration, 5*time.Second)
		}
	})

	t.Run("incomplete step", func(t *testing.T) {
		step := &ReplayStep{
			StartedAt: time.Now(),
		}

		duration := step.GetDuration()
		if duration != 0 {
			t.Errorf("GetDuration() for incomplete step = %v, want 0", duration)
		}
	})
}

func TestReplayExecutionStateHasDivergences(t *testing.T) {
	t.Run("no divergences", func(t *testing.T) {
		state := &ReplayExecutionState{
			Divergences: []ReplayDivergence{},
		}
		if state.HasDivergences() {
			t.Error("HasDivergences() should return false for empty divergences")
		}
	})

	t.Run("with divergences", func(t *testing.T) {
		state := &ReplayExecutionState{
			Divergences: []ReplayDivergence{
				{StepNumber: 1, Type: "content_mismatch"},
			},
		}
		if !state.HasDivergences() {
			t.Error("HasDivergences() should return true when divergences exist")
		}
	})
}

func TestReplayOptionsIsSafeMode(t *testing.T) {
	tests := []struct {
		mode     ReplayMode
		expected bool
	}{
		{ReplayModeReadOnly, true},
		{ReplayModeSimulate, true},
		{ReplayModeApply, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			opts := &ReplayOptions{Mode: tt.mode}
			result := opts.IsSafeMode()
			if result != tt.expected {
				t.Errorf("IsSafeMode() = %v, want %v for mode %s", result, tt.expected, tt.mode)
			}
		})
	}
}

func TestDefaultReplayOptions(t *testing.T) {
	opts := DefaultReplayOptions()

	t.Run("default mode is read_only", func(t *testing.T) {
		if opts.Mode != ReplayModeReadOnly {
			t.Errorf("Default Mode = %s, want %s", opts.Mode, ReplayModeReadOnly)
		}
	})

	t.Run("auto advance is disabled", func(t *testing.T) {
		if opts.AutoAdvance {
			t.Error("Default AutoAdvance should be false")
		}
	})

	t.Run("capture snapshots is enabled", func(t *testing.T) {
		if !opts.CaptureSnapshots {
			t.Error("Default CaptureSnapshots should be true")
		}
	})

	t.Run("validate checksums is enabled", func(t *testing.T) {
		if !opts.ValidateChecksums {
			t.Error("Default ValidateChecksums should be true")
		}
	})

	t.Run("stop on divergence is disabled", func(t *testing.T) {
		if opts.StopOnDivergence {
			t.Error("Default StopOnDivergence should be false")
		}
	})
}

func TestReplaySessionVersion(t *testing.T) {
	if ReplaySessionVersion == "" {
		t.Error("ReplaySessionVersion should not be empty")
	}

	// Should be in semver format
	expected := "1.0.0"
	if ReplaySessionVersion != expected {
		t.Errorf("ReplaySessionVersion = %s, want %s", ReplaySessionVersion, expected)
	}
}

func TestReplayModeConstants(t *testing.T) {
	// Verify mode string values
	if ReplayModeReadOnly != "read_only" {
		t.Errorf("ReplayModeReadOnly = %s, want read_only", ReplayModeReadOnly)
	}
	if ReplayModeSimulate != "simulate" {
		t.Errorf("ReplayModeSimulate = %s, want simulate", ReplayModeSimulate)
	}
	if ReplayModeApply != "apply" {
		t.Errorf("ReplayModeApply = %s, want apply", ReplayModeApply)
	}
}

func TestReplaySessionStatusConstants(t *testing.T) {
	statuses := []ReplaySessionStatus{
		ReplaySessionStatusRecording,
		ReplaySessionStatusCompleted,
		ReplaySessionStatusFailed,
		ReplaySessionStatusReplaying,
		ReplaySessionStatusPaused,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("ReplaySessionStatus should not be empty")
		}
	}
}

func TestReplayStepStatusConstants(t *testing.T) {
	statuses := []ReplayStepStatus{
		ReplayStepStatusPending,
		ReplayStepStatusRunning,
		ReplayStepStatusCompleted,
		ReplayStepStatusSkipped,
		ReplayStepStatusFailed,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("ReplayStepStatus should not be empty")
		}
	}
}

func TestReplayStepTypeConstants(t *testing.T) {
	types := []ReplayStepType{
		ReplayStepTypeModelRequest,
		ReplayStepTypeModelResponse,
		ReplayStepTypeFileDiff,
		ReplayStepTypeFileWrite,
		ReplayStepTypeFileRemove,
		ReplayStepTypeFileMove,
		ReplayStepTypeContextLoad,
		ReplayStepTypeContextUpdate,
		ReplayStepTypeBuildStart,
		ReplayStepTypeBuildComplete,
		ReplayStepTypeBuildError,
		ReplayStepTypeSubtaskStart,
		ReplayStepTypeSubtaskComplete,
		ReplayStepTypePlanningPhase,
		ReplayStepTypeImplementation,
		ReplayStepTypeUserPrompt,
		ReplayStepTypeMissingFilePrompt,
		ReplayStepTypeError,
	}

	for _, stepType := range types {
		if stepType == "" {
			t.Error("ReplayStepType should not be empty")
		}
	}
}
