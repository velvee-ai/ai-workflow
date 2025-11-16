package gitexec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes git commands with context support and configurable options.
type Runner struct {
	timeout time.Duration
}

// Result holds the output of a git command execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// New creates a new Runner with the given default timeout.
func New(timeout time.Duration) *Runner {
	return &Runner{timeout: timeout}
}

// Run executes a git command with the given arguments in the specified working directory.
// If workDir is empty, uses the current directory.
func (r *Runner) Run(ctx context.Context, workDir string, args ...string) (*Result, error) {
	// Apply timeout if not already set in context
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	result := &Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
	}

	if err != nil && exitCode != 0 {
		return result, fmt.Errorf("git %s failed (exit %d): %s", strings.Join(args, " "), exitCode, result.Stderr)
	}

	return result, nil
}

// RunSimple executes a git command and returns only stdout, erroring on non-zero exit.
func (r *Runner) RunSimple(ctx context.Context, workDir string, args ...string) (string, error) {
	result, err := r.Run(ctx, workDir, args...)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// RunIgnoreError executes a git command and returns stdout, ignoring errors.
func (r *Runner) RunIgnoreError(ctx context.Context, workDir string, args ...string) string {
	result, _ := r.Run(ctx, workDir, args...)
	return result.Stdout
}

// IsInsideWorkTree checks if the given directory is inside a git work tree.
func (r *Runner) IsInsideWorkTree(ctx context.Context, workDir string) bool {
	result, err := r.Run(ctx, workDir, "rev-parse", "--is-inside-work-tree")
	return err == nil && result.Stdout == "true"
}

// GetGitRoot returns the root directory of the git repository.
func (r *Runner) GetGitRoot(ctx context.Context, workDir string) (string, error) {
	return r.RunSimple(ctx, workDir, "rev-parse", "--show-toplevel")
}

// GetCurrentBranch returns the current branch name.
func (r *Runner) GetCurrentBranch(ctx context.Context, workDir string) (string, error) {
	return r.RunSimple(ctx, workDir, "branch", "--show-current")
}

// BranchExists checks if a branch exists locally.
func (r *Runner) BranchExists(ctx context.Context, workDir, branch string) bool {
	_, err := r.Run(ctx, workDir, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
	return err == nil
}

// IsWorktree checks if the given path is a git worktree.
func (r *Runner) IsWorktree(ctx context.Context, path string) bool {
	return r.IsInsideWorkTree(ctx, path)
}

// GetDefaultBranch returns the default branch name (e.g., "main" or "master").
// It uses the GitHub CLI to query the repository's default branch.
func (r *Runner) GetDefaultBranch(ctx context.Context, workDir string) (string, error) {
	// Apply timeout if not already set in context
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("gh repo view failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	branch := strings.TrimSpace(stdout.String())
	if branch == "" {
		return "", fmt.Errorf("could not determine default branch")
	}

	return branch, nil
}

// Worktree represents a git worktree entry.
type Worktree struct {
	Path   string
	Branch string
	Commit string
}

// ListWorktrees returns all worktrees for the repository.
func (r *Runner) ListWorktrees(ctx context.Context, workDir string) ([]Worktree, error) {
	output, err := r.RunSimple(ctx, workDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch ")
			// Remove refs/heads/ prefix if present
			current.Branch = strings.TrimPrefix(branch, "refs/heads/")
		}
	}

	// Add last worktree if exists
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// RemoveWorktree removes a worktree at the given path.
func (r *Runner) RemoveWorktree(ctx context.Context, repoPath, worktreePath string) error {
	_, err := r.RunSimple(ctx, repoPath, "worktree", "remove", worktreePath)
	return err
}

// PruneWorktrees removes worktree metadata for deleted worktrees.
func (r *Runner) PruneWorktrees(ctx context.Context, repoPath string) error {
	_, err := r.RunSimple(ctx, repoPath, "worktree", "prune")
	return err
}

// IsBranchMerged checks if a branch is merged into the base branch.
func (r *Runner) IsBranchMerged(ctx context.Context, repoPath, branchName, baseBranch string) (bool, error) {
	output, err := r.RunSimple(ctx, repoPath, "branch", "--merged", baseBranch)
	if err != nil {
		return false, err
	}

	// Check if branch name appears in merged branches
	for _, line := range strings.Split(output, "\n") {
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		branch = strings.TrimSpace(branch)
		if branch == branchName {
			return true, nil
		}
	}

	return false, nil
}

// GetGitStatus returns the porcelain status output lines.
// Empty slice means working tree is clean.
func (r *Runner) GetGitStatus(ctx context.Context, workDir string) ([]string, error) {
	output, err := r.RunSimple(ctx, workDir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return []string{}, nil
	}

	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}

	return result, nil
}

// FetchPrune fetches from remote and prunes deleted branches.
func (r *Runner) FetchPrune(ctx context.Context, workDir string) error {
	_, err := r.RunSimple(ctx, workDir, "fetch", "--prune")
	return err
}

// RemoteBranchExists checks if a branch exists on the remote.
func (r *Runner) RemoteBranchExists(ctx context.Context, workDir, branchName string) (bool, error) {
	output, err := r.RunSimple(ctx, workDir, "branch", "-r")
	if err != nil {
		return false, err
	}

	// Look for origin/<branchName>
	target := "origin/" + branchName
	for _, line := range strings.Split(output, "\n") {
		branch := strings.TrimSpace(line)
		if branch == target {
			return true, nil
		}
	}

	return false, nil
}
