package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"skip node_modules", "node_modules", true},
		{"skip .git", ".git", true},
		{"skip nested node_modules", "foo/node_modules", true},
		{"skip venv", "venv", true},
		{"skip __pycache__", "__pycache__", true},
		{"skip .plandex-v2", ".plandex-v2", true},
		{"allow src", "src", false},
		{"allow lib", "lib", false},
		{"allow regular dir", "myproject", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldSkipDir(tt.path)
			if result != tt.expected {
				t.Errorf("ShouldSkipDir(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsInSkippedDir(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"file in node_modules", "node_modules/package/index.js", true},
		{"file in .git", ".git/config", true},
		{"file in venv", "venv/lib/python3.9/site.py", true},
		{"file in src", "src/main.go", false},
		{"file in root", "README.md", false},
		{"nested file", "app/lib/utils.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInSkippedDir(tt.path)
			if result != tt.expected {
				t.Errorf("IsInSkippedDir(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsSubpathOf(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		parent   string
		child    string
		baseDir  string
		expected bool
		wantErr  bool
	}{
		{
			name:     "child is inside parent",
			parent:   "src",
			child:    "src/main.go",
			baseDir:  tempDir,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "child equals parent",
			parent:   "src",
			child:    "src",
			baseDir:  tempDir,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "child is outside parent",
			parent:   "src",
			child:    "lib/util.go",
			baseDir:  tempDir,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "child is parent of parent",
			parent:   "src/components",
			child:    "src",
			baseDir:  tempDir,
			expected: false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := IsSubpathOf(tt.parent, tt.child, tt.baseDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsSubpathOf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("IsSubpathOf(%q, %q, %q) = %v, want %v",
					tt.parent, tt.child, tt.baseDir, result, tt.expected)
			}
		})
	}
}

func TestGetPlandexIgnore(t *testing.T) {
	t.Run("returns nil when no .plandexignore file exists", func(t *testing.T) {
		tempDir := t.TempDir()
		result, err := GetPlandexIgnore(tempDir)
		if err != nil {
			t.Errorf("GetPlandexIgnore() unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("GetPlandexIgnore() = %v, want nil", result)
		}
	})

	t.Run("parses .plandexignore file correctly", func(t *testing.T) {
		tempDir := t.TempDir()
		ignoreFile := filepath.Join(tempDir, ".plandexignore")
		err := os.WriteFile(ignoreFile, []byte("*.log\ntmp/\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create .plandexignore: %v", err)
		}

		result, err := GetPlandexIgnore(tempDir)
		if err != nil {
			t.Errorf("GetPlandexIgnore() unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("GetPlandexIgnore() returned nil, expected ignore matcher")
		}

		// Test that patterns work
		if !result.MatchesPath("debug.log") {
			t.Error("Expected *.log pattern to match debug.log")
		}
		if !result.MatchesPath("tmp/cache") {
			t.Error("Expected tmp/ pattern to match tmp/cache")
		}
		if result.MatchesPath("src/main.go") {
			t.Error("Did not expect src/main.go to be ignored")
		}
	})
}

func TestGetBaseDirForFilePaths(t *testing.T) {
	// Save original ProjectRoot and restore after test
	originalProjectRoot := ProjectRoot
	defer func() { ProjectRoot = originalProjectRoot }()

	ProjectRoot = "/home/user/project"

	tests := []struct {
		name     string
		paths    []string
		expected string
	}{
		{
			name:     "no paths returns project root",
			paths:    []string{},
			expected: "/home/user/project",
		},
		{
			name:     "paths within project",
			paths:    []string{"src/main.go", "lib/util.go"},
			expected: "/home/user/project",
		},
		{
			name:     "path with parent reference",
			paths:    []string{"../sibling/file.go"},
			expected: "/home/user",
		},
		{
			name:     "multiple parent references",
			paths:    []string{"../../other/file.go"},
			expected: "/home",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBaseDirForFilePaths(tt.paths)
			if result != tt.expected {
				t.Errorf("GetBaseDirForFilePaths(%v) = %q, want %q",
					tt.paths, result, tt.expected)
			}
		})
	}
}
