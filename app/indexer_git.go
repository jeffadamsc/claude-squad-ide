package app

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ChangeType indicates how a symbol was affected by a diff.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeModified ChangeType = "modified"
	ChangeDeleted  ChangeType = "deleted"
)

// ChangedSymbol represents a symbol affected by a git diff.
type ChangedSymbol struct {
	Symbol     Symbol     `json:"symbol"`
	ChangeType ChangeType `json:"change_type"`
	HunkHeader string     `json:"hunk_header,omitempty"` // e.g., "@@ -10,5 +10,7 @@"
}

// DiffHunk represents a changed region in a file.
type DiffHunk struct {
	File       string
	OldStart   int
	OldCount   int
	NewStart   int
	NewCount   int
	Header     string
	AddedLines []int // line numbers in new file
	RemovedLines []int // line numbers in old file
}

// GetChangedSymbols returns symbols affected by changes between two commits.
// If toCommit is empty, compares fromCommit to working directory.
// If fromCommit is empty, uses HEAD.
func (idx *TreeSitterIndexer) GetChangedSymbols(fromCommit, toCommit string) ([]ChangedSymbol, error) {
	// Get the diff
	hunks, err := getGitDiffHunks(idx.worktree, fromCommit, toCommit)
	if err != nil {
		return nil, err
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var changed []ChangedSymbol
	seen := make(map[string]bool) // dedupe by file:symbol

	for _, hunk := range hunks {
		// Find symbols in this file that overlap with changed lines
		for _, syms := range idx.symbols {
			for _, sym := range syms {
				if sym.File != hunk.File {
					continue
				}

				key := sym.File + ":" + sym.Name
				if seen[key] {
					continue
				}

				// Check if symbol overlaps with changed lines
				changeType := getChangeType(sym, hunk)
				if changeType != "" {
					seen[key] = true
					changed = append(changed, ChangedSymbol{
						Symbol:     sym,
						ChangeType: changeType,
						HunkHeader: hunk.Header,
					})
				}
			}
		}
	}

	return changed, nil
}

// getChangeType determines how a symbol was affected by a hunk.
func getChangeType(sym Symbol, hunk DiffHunk) ChangeType {
	symStart := sym.Line
	symEnd := sym.EndLine
	if symEnd == 0 {
		symEnd = symStart
	}

	// Check if any added lines fall within the symbol
	hasAdditions := false
	for _, line := range hunk.AddedLines {
		if line >= symStart && line <= symEnd {
			hasAdditions = true
			break
		}
	}

	// Check if any removed lines fall within the symbol's old position
	hasRemovals := false
	for _, line := range hunk.RemovedLines {
		if line >= symStart && line <= symEnd {
			hasRemovals = true
			break
		}
	}

	// Determine change type
	if hasAdditions && hasRemovals {
		return ChangeModified
	} else if hasAdditions {
		// Could be new symbol or modification
		return ChangeModified
	} else if hasRemovals {
		// Lines removed from symbol
		return ChangeModified
	}

	return "" // No change to this symbol
}

// getGitDiffHunks parses git diff output into structured hunks.
func getGitDiffHunks(worktree, fromCommit, toCommit string) ([]DiffHunk, error) {
	args := []string{"diff", "--unified=0"}

	if fromCommit == "" {
		fromCommit = "HEAD"
	}

	if toCommit == "" {
		// Compare to working directory
		args = append(args, fromCommit)
	} else {
		args = append(args, fromCommit, toCommit)
	}

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = worktree
	out, err := cmd.Output()
	if err != nil {
		// Empty diff is not an error
		if len(out) == 0 {
			return nil, nil
		}
		return nil, err
	}

	return parseDiffOutput(string(out)), nil
}

// Regex patterns for parsing diff output
var (
	diffFilePattern = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)
	hunkPattern     = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
)

// parseDiffOutput parses unified diff output into hunks.
func parseDiffOutput(diff string) []DiffHunk {
	var hunks []DiffHunk
	var currentFile string
	var currentHunk *DiffHunk

	scanner := bufio.NewScanner(strings.NewReader(diff))
	addedLineNum := 0
	removedLineNum := 0

	for scanner.Scan() {
		line := scanner.Text()

		// Check for new file
		if matches := diffFilePattern.FindStringSubmatch(line); matches != nil {
			currentFile = matches[2] // Use the "b/" path (new file)
			continue
		}

		// Check for hunk header
		if matches := hunkPattern.FindStringSubmatch(line); matches != nil {
			// Save previous hunk
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}

			oldStart, _ := strconv.Atoi(matches[1])
			oldCount := 1
			if matches[2] != "" {
				oldCount, _ = strconv.Atoi(matches[2])
			}
			newStart, _ := strconv.Atoi(matches[3])
			newCount := 1
			if matches[4] != "" {
				newCount, _ = strconv.Atoi(matches[4])
			}

			currentHunk = &DiffHunk{
				File:     currentFile,
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
				Header:   line,
			}
			addedLineNum = newStart
			removedLineNum = oldStart
			continue
		}

		// Parse diff lines
		if currentHunk != nil && len(line) > 0 {
			switch line[0] {
			case '+':
				currentHunk.AddedLines = append(currentHunk.AddedLines, addedLineNum)
				addedLineNum++
			case '-':
				currentHunk.RemovedLines = append(currentHunk.RemovedLines, removedLineNum)
				removedLineNum++
			case ' ':
				// Context line
				addedLineNum++
				removedLineNum++
			}
		}
	}

	// Don't forget the last hunk
	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

// GetChangedFiles returns the list of files changed between commits.
func (idx *TreeSitterIndexer) GetChangedFiles(fromCommit, toCommit string) ([]string, error) {
	args := []string{"diff", "--name-only"}

	if fromCommit == "" {
		fromCommit = "HEAD"
	}

	if toCommit == "" {
		args = append(args, fromCommit)
	} else {
		args = append(args, fromCommit, toCommit)
	}

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = idx.worktree
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}
