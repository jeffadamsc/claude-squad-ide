package git

import (
	"strings"
)

// DiffStats holds statistics about the changes in a diff
type DiffStats struct {
	// Content is the full diff content
	Content string
	// Added is the number of added lines
	Added int
	// Removed is the number of removed lines
	Removed int
	// Error holds any error that occurred during diff computation
	// This allows propagating setup errors (like missing base commit) without breaking the flow
	Error error
}

func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && d.Content == ""
}

// Diff returns the git diff between the worktree and the base branch along with statistics
func (g *GitWorktree) Diff() *DiffStats {
	stats := &DiffStats{}

	// -N stages untracked files (intent to add), including them in the diff
	_, err := g.runGitCommand(g.worktreePath, "add", "-N", ".")
	if err != nil {
		stats.Error = err
		return stats
	}

	content, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", g.GetBaseCommitSHA())
	if err != nil {
		stats.Error = err
		return stats
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.Added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.Removed++
		}
	}
	stats.Content = content

	return stats
}

// AggregatedDiffStats holds diff stats across the parent repo and its submodules.
type AggregatedDiffStats struct {
	Parent     *DiffStats
	Submodules map[string]*DiffStats
}

func (g *GitWorktree) AggregatedDiff() *AggregatedDiffStats {
	result := &AggregatedDiffStats{
		Parent:     g.Diff(),
		Submodules: make(map[string]*DiffStats),
	}
	for path, sw := range g.submodules {
		result.Submodules[path] = sw.Diff()
	}
	return result
}

func (a *AggregatedDiffStats) TotalAdded() int {
	total := 0
	if a.Parent != nil {
		total += a.Parent.Added
	}
	for _, s := range a.Submodules {
		total += s.Added
	}
	return total
}

func (a *AggregatedDiffStats) TotalRemoved() int {
	total := 0
	if a.Parent != nil {
		total += a.Parent.Removed
	}
	for _, s := range a.Submodules {
		total += s.Removed
	}
	return total
}
