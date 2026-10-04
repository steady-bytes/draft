// Package gitutil is the small set of git plumbing operations Allele's diagnostic tools (and,
// eventually, AlleleService's own RPC handlers once Phase 7 gives them real worktrees to operate
// on) need: listing what changed between two refs, and reading a path's content at a given ref.
// Shells out to the real `git` binary -- consistent with the plan's own Decisions ("git
// http-backend is wrapped, not reimplemented"), not a second, parallel git implementation.
package gitutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ChangedFiles lists paths that differ between baseRef and compareRef, using "..." (merge-base
// diff: what changed on compareRef since it diverged from baseRef) -- "a worktree's branch against
// its base" everywhere else in this plan.
func ChangedFiles(repoPath, baseRef, compareRef string) ([]string, error) {
	out, err := output(repoPath, "diff", "--name-only", baseRef+"..."+compareRef)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// ShowFile returns path's content at ref. `git show ref:path` fails (non-zero exit) for a path
// that doesn't exist at that ref -- the expected, normal case for a newly added (no old version)
// or deleted (no new version) file, which this function reports as (nil, nil), not an error.
func ShowFile(repoPath, ref, path string) ([]byte, error) {
	cmd := exec.Command("git", "-C", repoPath, "show", ref+":"+path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "does not exist") || strings.Contains(msg, "exists on disk, but not in") {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(msg))
	}
	return stdout.Bytes(), nil
}

// MergeBase returns the merge-base commit of a and b -- the shared ancestor a three-way comparison
// treats as "base" when neither ref is itself already known to be the other's ancestor.
func MergeBase(repoPath, a, b string) (string, error) {
	out, err := output(repoPath, "merge-base", a, b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ListBranches lists every local branch name in repoPath (e.g. "main", "wt-07") -- what worktree
// discovery (store.go's syncWorktrees) walks to find open worktrees, since the bare repo's own ref
// list is the actual source of truth for what's open, not a separately-maintained table that could
// drift from it.
func ListBranches(repoPath string) ([]string, error) {
	out, err := output(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// CommitAuthor returns ref's own latest commit's author name and email -- used to label a
// newly-discovered worktree with *something* identifying who's behind it before Phase 8 gives
// Allele a real, server-enforced identity (the X-Authentik-Username that arrived with the actual
// push) to use instead. A disclosed placeholder, not the real provenance mechanism -- see the
// plan's Phase 8.
func CommitAuthor(repoPath, ref string) (name, email string, err error) {
	out, err := output(repoPath, "log", "-1", "--format=%an\t%ae", ref)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(strings.TrimSpace(out), "\t", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected `git log` output: %q", out)
	}
	return parts[0], parts[1], nil
}

// DiffStat returns the total lines added/removed between baseRef and compareRef (git diff
// --numstat, summed across every changed file) -- plain line-count stats, independent of the
// symbol-level diff (diff.SymbolChanges), which never counts lines at all. A binary file's "-"
// entries are silently skipped (they don't parse as an int), not an error.
func DiffStat(repoPath, baseRef, compareRef string) (added, removed int64, err error) {
	out, err := output(repoPath, "diff", "--numstat", baseRef+"..."+compareRef)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if a, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			added += a
		}
		if r, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
			removed += r
		}
	}
	return added, removed, nil
}

// HeadCommit returns ref's current commit SHA.
func HeadCommit(repoPath, ref string) (string, error) {
	out, err := output(repoPath, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func output(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
