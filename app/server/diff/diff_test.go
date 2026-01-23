package diff

import (
	"strings"
	"testing"
)

func TestGetDiffs(t *testing.T) {
	tests := []struct {
		name     string
		original string
		updated  string
		wantDiff bool
		contains []string
	}{
		{
			name:     "identical files",
			original: "hello world",
			updated:  "hello world",
			wantDiff: false,
		},
		{
			name:     "single line change",
			original: "hello world",
			updated:  "hello universe",
			wantDiff: true,
			contains: []string{"-hello world", "+hello universe"},
		},
		{
			name:     "add line",
			original: "line1\nline2",
			updated:  "line1\nline2\nline3",
			wantDiff: true,
			contains: []string{"+line3"},
		},
		{
			name:     "remove line",
			original: "line1\nline2\nline3",
			updated:  "line1\nline3",
			wantDiff: true,
			contains: []string{"-line2"},
		},
		{
			name:     "empty original",
			original: "",
			updated:  "new content",
			wantDiff: true,
			contains: []string{"+new content"},
		},
		{
			name:     "empty updated",
			original: "old content",
			updated:  "",
			wantDiff: true,
			contains: []string{"-old content"},
		},
		{
			name:     "multiline change",
			original: "func main() {\n    fmt.Println(\"Hello\")\n}",
			updated:  "func main() {\n    fmt.Println(\"World\")\n}",
			wantDiff: true,
			contains: []string{"-    fmt.Println(\"Hello\")", "+    fmt.Println(\"World\")"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := GetDiffs(tt.original, tt.updated)
			if err != nil {
				t.Fatalf("GetDiffs() error = %v", err)
			}

			hasDiff := len(diff) > 0
			if hasDiff != tt.wantDiff {
				t.Errorf("GetDiffs() hasDiff = %v, want %v", hasDiff, tt.wantDiff)
			}

			for _, expected := range tt.contains {
				if !strings.Contains(diff, expected) {
					t.Errorf("GetDiffs() diff missing %q\nGot:\n%s", expected, diff)
				}
			}
		})
	}
}

func TestGetDiffReplacements(t *testing.T) {
	tests := []struct {
		name           string
		original       string
		updated        string
		wantCount      int
		checkOldNew    bool
		expectedOld    string
		expectedNew    string
	}{
		{
			name:      "identical files",
			original:  "hello world",
			updated:   "hello world",
			wantCount: 0,
		},
		{
			name:        "single replacement",
			original:    "hello world",
			updated:     "hello universe",
			wantCount:   1,
			checkOldNew: true,
			expectedOld: "hello world",
			expectedNew: "hello universe",
		},
		{
			name:      "multiple line replacement",
			original:  "line1\nline2\nline3",
			updated:   "line1\nmodified\nline3",
			wantCount: 1,
		},
		{
			name:      "add content",
			original:  "existing",
			updated:   "existing\nnew line",
			wantCount: 1,
		},
		{
			name:      "remove content",
			original:  "keep\nremove\nkeep",
			updated:   "keep\nkeep",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replacements, err := GetDiffReplacements(tt.original, tt.updated)
			if err != nil {
				t.Fatalf("GetDiffReplacements() error = %v", err)
			}

			if len(replacements) != tt.wantCount {
				t.Errorf("GetDiffReplacements() count = %d, want %d", len(replacements), tt.wantCount)
			}

			if tt.checkOldNew && len(replacements) > 0 {
				if replacements[0].Old != tt.expectedOld {
					t.Errorf("Old = %q, want %q", replacements[0].Old, tt.expectedOld)
				}
				if replacements[0].New != tt.expectedNew {
					t.Errorf("New = %q, want %q", replacements[0].New, tt.expectedNew)
				}
			}

			// Check that all replacements have unique IDs
			ids := make(map[string]bool)
			for _, r := range replacements {
				if r.Id == "" {
					t.Error("replacement has empty Id")
				}
				if ids[r.Id] {
					t.Errorf("duplicate Id found: %s", r.Id)
				}
				ids[r.Id] = true
			}
		})
	}
}

func TestProcessHunk(t *testing.T) {
	tests := []struct {
		name      string
		oldLines  []string
		newLines  []string
		startLine int
		wantNil   bool
		wantOld   string
		wantNew   string
	}{
		{
			name:      "both empty",
			oldLines:  []string{},
			newLines:  []string{},
			startLine: 1,
			wantNil:   true,
		},
		{
			name:      "old lines only",
			oldLines:  []string{"removed line"},
			newLines:  []string{},
			startLine: 5,
			wantNil:   false,
			wantOld:   "removed line",
			wantNew:   "",
		},
		{
			name:      "new lines only",
			oldLines:  []string{},
			newLines:  []string{"added line"},
			startLine: 10,
			wantNil:   false,
			wantOld:   "",
			wantNew:   "added line",
		},
		{
			name:      "both old and new",
			oldLines:  []string{"old1", "old2"},
			newLines:  []string{"new1", "new2", "new3"},
			startLine: 1,
			wantNil:   false,
			wantOld:   "old1\nold2",
			wantNew:   "new1\nnew2\nnew3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processHunk(tt.oldLines, tt.newLines, tt.startLine)

			if tt.wantNil {
				if result != nil {
					t.Errorf("processHunk() = %+v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatal("processHunk() = nil, want non-nil")
			}

			if result.Old != tt.wantOld {
				t.Errorf("Old = %q, want %q", result.Old, tt.wantOld)
			}
			if result.New != tt.wantNew {
				t.Errorf("New = %q, want %q", result.New, tt.wantNew)
			}
			if result.Line != tt.startLine {
				t.Errorf("Line = %d, want %d", result.Line, tt.startLine)
			}
			if result.Length != len(tt.oldLines) {
				t.Errorf("Length = %d, want %d", result.Length, len(tt.oldLines))
			}
		})
	}
}

func TestGetDiffsWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		original string
		updated  string
	}{
		{
			name:     "tabs",
			original: "line\twith\ttabs",
			updated:  "line\twith\tmodified\ttabs",
		},
		{
			name:     "unicode",
			original: "Hello 世界",
			updated:  "Hello 宇宙",
		},
		{
			name:     "special chars",
			original: "a < b && c > d",
			updated:  "a <= b || c >= d",
		},
		{
			name:     "quotes",
			original: `say "hello"`,
			updated:  `say "goodbye"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := GetDiffs(tt.original, tt.updated)
			if err != nil {
				t.Fatalf("GetDiffs() error = %v", err)
			}
			if diff == "" {
				t.Error("GetDiffs() returned empty diff for different content")
			}
		})
	}
}

func TestGetDiffReplacementsLargeFile(t *testing.T) {
	// Generate a large file
	var original, updated strings.Builder
	for i := 0; i < 100; i++ {
		original.WriteString("line ")
		original.WriteString(strings.Repeat("x", i))
		original.WriteString("\n")

		updated.WriteString("line ")
		if i == 50 {
			updated.WriteString("MODIFIED ")
		}
		updated.WriteString(strings.Repeat("x", i))
		updated.WriteString("\n")
	}

	replacements, err := GetDiffReplacements(original.String(), updated.String())
	if err != nil {
		t.Fatalf("GetDiffReplacements() error = %v", err)
	}

	if len(replacements) == 0 {
		t.Error("GetDiffReplacements() returned no replacements for modified large file")
	}
}
