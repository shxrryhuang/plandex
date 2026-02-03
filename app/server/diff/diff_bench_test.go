package diff

import (
	"fmt"
	"strings"
	"testing"
)

func generateFile(lines int) string {
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		sb.WriteString(fmt.Sprintf("line %04d: the quick brown fox jumps over the lazy dog\n", i))
	}
	return sb.String()
}

// mutateEveryNth returns a copy of original where every nth line is changed.
func mutateEveryNth(original string, n int) string {
	lines := splitLines(original)
	for i := n - 1; i < len(lines); i += n {
		lines[i] = fmt.Sprintf("line %04d: MODIFIED content with different text here", i)
	}
	return strings.Join(lines, "\n") + "\n"
}

// --- Small (50 lines) benchmarks ---

func BenchmarkDiff_PureGo_Small(b *testing.B) {
	orig := generateFile(50)
	upd := mutateEveryNth(orig, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pureGoDiffReplacements(orig, upd)
	}
}

func BenchmarkDiff_Git_Small(b *testing.B) {
	orig := generateFile(50)
	upd := mutateEveryNth(orig, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gitDiffReplacements(orig, upd)
	}
}

// --- Medium (500 lines) benchmarks ---

func BenchmarkDiff_PureGo_Medium(b *testing.B) {
	orig := generateFile(500)
	upd := mutateEveryNth(orig, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pureGoDiffReplacements(orig, upd)
	}
}

func BenchmarkDiff_Git_Medium(b *testing.B) {
	orig := generateFile(500)
	upd := mutateEveryNth(orig, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gitDiffReplacements(orig, upd)
	}
}

// --- Large (2000 lines) benchmarks ---

func BenchmarkDiff_PureGo_Large(b *testing.B) {
	orig := generateFile(2000)
	upd := mutateEveryNth(orig, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pureGoDiffReplacements(orig, upd)
	}
}

func BenchmarkDiff_Git_Large(b *testing.B) {
	orig := generateFile(2000)
	upd := mutateEveryNth(orig, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gitDiffReplacements(orig, upd)
	}
}

// TestPureGoDiffCorrectness verifies that applying the pure-Go replacements
// to the original text produces the expected updated text.
func TestPureGoDiffCorrectness(t *testing.T) {
	sizes := []int{20, 100, 500}
	mutations := []int{5, 10, 20}

	for _, size := range sizes {
		for _, mut := range mutations {
			t.Run(fmt.Sprintf("lines=%d_mutEvery=%d", size, mut), func(t *testing.T) {
				orig := generateFile(size)
				upd := mutateEveryNth(orig, mut)

				res, err := pureGoDiffReplacements(orig, upd)
				if err != nil {
					t.Fatalf("pureGoDiffReplacements: %v", err)
				}
				if len(res) == 0 {
					t.Fatal("expected replacements, got none")
				}

				// Verify each Old is a substring of the original
				for i, r := range res {
					if !strings.Contains(orig, r.Old) {
						t.Errorf("replacement[%d].Old not found in original:\n  Old=%q", i, r.Old)
					}
				}

				// Apply replacements serially and verify result matches updated
				got := orig
				for i, r := range res {
					idx := strings.Index(got, r.Old)
					if idx == -1 {
						t.Fatalf("replacement[%d].Old not found during serial apply:\n  Old=%q", i, r.Old)
					}
					got = got[:idx] + r.New + got[idx+len(r.Old):]
				}

				// Normalize trailing newline
				if !strings.HasSuffix(got, "\n") {
					got += "\n"
				}
				if got != upd {
					t.Errorf("applied result does not match updated (first 200 chars):\n  got =%q\n  want=%q",
						got[:min(len(got), 200)], upd[:min(len(upd), 200)])
				}
			})
		}
	}
}

// TestPureGoDiffEdgeCases covers boundary conditions.
func TestPureGoDiffEdgeCases(t *testing.T) {
	t.Run("identical", func(t *testing.T) {
		res, err := pureGoDiffReplacements("hello\nworld\n", "hello\nworld\n")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 0 {
			t.Fatalf("expected no replacements for identical input, got %d", len(res))
		}
	})

	t.Run("empty_to_content", func(t *testing.T) {
		res, err := pureGoDiffReplacements("", "hello\nworld\n")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 replacement, got %d", len(res))
		}
		if res[0].Old != "" {
			t.Errorf("Old should be empty for insertion into empty file, got %q", res[0].Old)
		}
	})

	t.Run("content_to_empty", func(t *testing.T) {
		res, err := pureGoDiffReplacements("hello\nworld\n", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 replacement, got %d", len(res))
		}
		if res[0].New != "" {
			t.Errorf("New should be empty for deletion to empty file, got %q", res[0].New)
		}
	})

	t.Run("single_line_change", func(t *testing.T) {
		orig := "aaa\nbbb\nccc\nddd\neee\n"
		upd := "aaa\nbbb\nXXX\nddd\neee\n"
		res, err := pureGoDiffReplacements(orig, upd)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 replacement, got %d", len(res))
		}
		if !strings.Contains(res[0].Old, "ccc") {
			t.Errorf("Old should contain 'ccc', got %q", res[0].Old)
		}
		if !strings.Contains(res[0].New, "XXX") {
			t.Errorf("New should contain 'XXX', got %q", res[0].New)
		}
	})

	t.Run("append_line", func(t *testing.T) {
		orig := "aaa\nbbb\nccc\n"
		upd := "aaa\nbbb\nccc\nddd\n"
		res, err := pureGoDiffReplacements(orig, upd)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 replacement, got %d", len(res))
		}
		got := orig
		idx := strings.Index(got, res[0].Old)
		got = got[:idx] + res[0].New + got[idx+len(res[0].Old):]
		if !strings.HasSuffix(got, "\n") {
			got += "\n"
		}
		if got != upd {
			t.Errorf("append: got %q, want %q", got, upd)
		}
	})
}
