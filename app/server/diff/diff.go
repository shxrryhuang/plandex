package diff

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"plandex-server/perf"

	shared "plandex-shared"

	"github.com/google/uuid"
)

const (
	// maxPureGoLines is the maximum number of lines in the changed middle
	// section (per side) that the in-process LCS algorithm will handle.
	// Beyond this we fall back to git diff --no-index.
	maxPureGoLines = 5000

	// diffContextLines is the number of unchanged lines of context around
	// each change group, matching git diff's default of 3.
	diffContextLines = 3
)

// errTooLargeForPureGo signals that the changed region exceeds maxPureGoLines.
var errTooLargeForPureGo = fmt.Errorf("middle section exceeds %d lines per side", maxPureGoLines)

type editKind int

const (
	editEqual editKind = iota
	editDelete
	editInsert
)

type editEntry struct {
	kind editKind
	line string
}

type diffHunk struct {
	oldText string
	newText string
}

// GetDiffs returns the raw unified-diff text by spawning git diff --no-index.
// Kept for callers that need the raw unified-diff format (e.g. build_validate_and_fix).
func GetDiffs(original, updated string) (string, error) {
	tempDirPath, err := os.MkdirTemp("", "tmp-diffs-*")
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDirPath)

	err = os.WriteFile(filepath.Join(tempDirPath, "original"), []byte(original), 0644)
	if err != nil {
		return "", fmt.Errorf("error writing original file: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDirPath, "updated"), []byte(updated), 0644)
	if err != nil {
		return "", fmt.Errorf("error writing updated file: %v", err)
	}

	cmd := exec.Command("git", "-C", tempDirPath, "diff", "--no-color", "--no-index", "original", "updated")
	res, err := cmd.CombinedOutput()
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if ok && exitError.ExitCode() == 1 {
			// exit 1 means diffs were found — expected
		} else {
			log.Printf("Error getting diffs: %v\n", err)
			log.Printf("Diff output: %s\n", res)
			return "", fmt.Errorf("error getting diffs: %v", err)
		}
	}
	return string(res), nil
}

// GetDiffReplacements computes the set of replacements needed to transform
// original into updated.  It uses an in-process LCS algorithm by default and
// falls back to git diff --no-index only when the changed region exceeds
// maxPureGoLines lines per side.
func GetDiffReplacements(original, updated string) ([]*shared.Replacement, error) {
	done := perf.Timer(perf.CatDiff, "get_replacements")
	defer done()

	if original == updated {
		return nil, nil
	}

	replacements, err := pureGoDiffReplacements(original, updated)
	if err == errTooLargeForPureGo {
		log.Println("diff: middle section too large, falling back to git subprocess")
		return gitDiffReplacements(original, updated)
	}
	if err != nil {
		return nil, err
	}
	return replacements, nil
}

// pureGoDiffReplacements computes replacements using an in-process LCS diff.
func pureGoDiffReplacements(original, updated string) ([]*shared.Replacement, error) {
	origLines := splitLines(original)
	updLines := splitLines(updated)

	prefixLen, suffixLen := commonPrefixSuffix(origLines, updLines)

	// Entirely identical
	if prefixLen+suffixLen >= len(origLines) && prefixLen+suffixLen >= len(updLines) {
		return nil, nil
	}

	origEnd := len(origLines) - suffixLen
	updEnd := len(updLines) - suffixLen
	origMid := origLines[prefixLen:origEnd]
	updMid := updLines[prefixLen:updEnd]

	if len(origMid) > maxPureGoLines || len(updMid) > maxPureGoLines {
		return nil, errTooLargeForPureGo
	}

	midEdits := computeEditScript(origMid, updMid)

	// Build full edit sequence: prefix (Equal) + midEdits + suffix (Equal)
	fullEdits := make([]editEntry, 0, prefixLen+len(midEdits)+suffixLen)
	for i := 0; i < prefixLen; i++ {
		fullEdits = append(fullEdits, editEntry{kind: editEqual, line: origLines[i]})
	}
	fullEdits = append(fullEdits, midEdits...)
	for i := origEnd; i < len(origLines); i++ {
		fullEdits = append(fullEdits, editEntry{kind: editEqual, line: origLines[i]})
	}

	hunks := groupIntoHunks(fullEdits, diffContextLines)

	replacements := make([]*shared.Replacement, len(hunks))
	for i, h := range hunks {
		replacements[i] = &shared.Replacement{
			Id:  uuid.New().String(),
			Old: h.oldText,
			New: h.newText,
		}
	}
	return replacements, nil
}

// splitLines splits s on "\n".  A trailing newline does not produce a trailing
// empty element, matching the semantic line count of typical source files.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// commonPrefixSuffix returns the length of the longest equal prefix and suffix
// of a and b.  The prefix and suffix regions do not overlap.
func commonPrefixSuffix(a, b []string) (prefix, suffix int) {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for prefix < minLen && a[prefix] == b[prefix] {
		prefix++
	}
	maxSuffix := minLen - prefix
	for suffix < maxSuffix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	return
}

// computeEditScript runs an O(m*n) LCS dynamic-programming table on a and b,
// then backtracks to produce a sequence of Equal / Delete / Insert entries.
func computeEditScript(a, b []string) []editEntry {
	m, n := len(a), len(b)

	// dp[i][j] = LCS length for a[:i] and b[:j]
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to build edit script (in reverse order)
	edits := make([]editEntry, 0, m+n)
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && a[i-1] == b[j-1] {
			edits = append(edits, editEntry{kind: editEqual, line: a[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			edits = append(edits, editEntry{kind: editInsert, line: b[j-1]})
			j--
		} else {
			edits = append(edits, editEntry{kind: editDelete, line: a[i-1]})
			i--
		}
	}

	// Reverse to get chronological order
	for l, r := 0, len(edits)-1; l < r; l, r = l+1, r-1 {
		edits[l], edits[r] = edits[r], edits[l]
	}
	return edits
}

// groupIntoHunks splits the edit script into hunks at runs of more than
// 2*ctx consecutive Equal entries, then expands each hunk by ctx lines
// of context on each side.
func groupIntoHunks(edits []editEntry, ctx int) []diffHunk {
	// Find first and last change indices
	firstChange, lastChange := -1, -1
	for i, e := range edits {
		if e.kind != editEqual {
			if firstChange == -1 {
				firstChange = i
			}
			lastChange = i
		}
	}
	if firstChange == -1 {
		return nil // no changes at all
	}

	// Walk from firstChange to lastChange; split at runs of > 2*ctx Equal entries
	type changeRange struct{ start, end int }
	var ranges []changeRange
	currentStart := firstChange
	i := firstChange
	for i <= lastChange {
		if edits[i].kind != editEqual {
			i++
			continue
		}
		// Count consecutive Equal entries
		runStart := i
		for i <= lastChange && edits[i].kind == editEqual {
			i++
		}
		if i > lastChange {
			break // trailing equals past the last change — not a split point
		}
		if i-runStart > 2*ctx {
			// Hunk boundary
			ranges = append(ranges, changeRange{currentStart, runStart - 1})
			currentStart = i
		}
	}
	ranges = append(ranges, changeRange{currentStart, lastChange})

	// Convert each range into a hunk with surrounding context
	hunks := make([]diffHunk, len(ranges))
	for idx, r := range ranges {
		start := r.start - ctx
		if start < 0 {
			start = 0
		}
		end := r.end + ctx
		if end >= len(edits) {
			end = len(edits) - 1
		}

		var oldLines, newLines []string
		for j := start; j <= end; j++ {
			switch edits[j].kind {
			case editEqual:
				oldLines = append(oldLines, edits[j].line)
				newLines = append(newLines, edits[j].line)
			case editDelete:
				oldLines = append(oldLines, edits[j].line)
			case editInsert:
				newLines = append(newLines, edits[j].line)
			}
		}

		hunks[idx] = diffHunk{
			oldText: strings.Join(oldLines, "\n"),
			newText: strings.Join(newLines, "\n"),
		}
	}
	return hunks
}

// gitDiffReplacements is the git-subprocess fallback, used when the changed
// region is too large for in-process LCS.
func gitDiffReplacements(original, updated string) ([]*shared.Replacement, error) {
	diff, err := GetDiffs(original, updated)
	if err != nil {
		return nil, fmt.Errorf("error getting git diffs: %v", err)
	}

	var changes []*change
	scanner := bufio.NewScanner(strings.NewReader(diff))

	var currentHunk *change
	var oldLines, newLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				ch := processHunk(oldLines, newLines, currentHunk.Line)
				if ch != nil {
					changes = append(changes, ch)
				}
			}

			lineInfo := strings.Split(line, " ")[1:]
			if len(lineInfo) == 0 {
				continue
			}
			oldInfo := strings.Split(lineInfo[0], ",")
			startLine, _ := strconv.Atoi(strings.TrimPrefix(oldInfo[0], "-"))

			currentHunk = &change{Line: startLine}
			oldLines = []string{}
			newLines = []string{}
			continue
		}

		if currentHunk == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "-"):
			oldLines = append(oldLines, strings.TrimPrefix(line, "-"))
		case strings.HasPrefix(line, "+"):
			newLines = append(newLines, strings.TrimPrefix(line, "+"))
		case strings.HasPrefix(line, " "):
			line = strings.TrimPrefix(line, " ")
			oldLines = append(oldLines, line)
			newLines = append(newLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning diff: %v", err)
	}

	if currentHunk != nil {
		ch := processHunk(oldLines, newLines, currentHunk.Line)
		if ch != nil {
			changes = append(changes, ch)
		}
	}

	replacements := make([]*shared.Replacement, len(changes))
	for i, change := range changes {
		replacements[i] = &shared.Replacement{
			Id:  uuid.New().String(),
			Old: change.Old,
			New: change.New,
		}
	}
	return replacements, nil
}

type change struct {
	Old    string
	New    string
	Line   int
	Length int
}

func processHunk(oldLines, newLines []string, startLine int) *change {
	if len(oldLines) == 0 && len(newLines) == 0 {
		return nil
	}

	return &change{
		Old:    strings.Join(oldLines, "\n"),
		New:    strings.Join(newLines, "\n"),
		Line:   startLine,
		Length: len(oldLines),
	}
}
