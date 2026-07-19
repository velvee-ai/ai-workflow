package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	commitJSON  bool
	commitPaths []string
	commitBase  string
)

var commitCmd = &cobra.Command{
	Use:   "commit <message>",
	Short: "Add, commit, pull, push, and create PR",
	Long: `Automate the git workflow: add all changes, commit, pull with rebase, push, and create a pull request.

This command performs the following steps:
1. git add . (or only --paths, when given)
2. git commit -m "<message>"
3. git pull --rebase
4. git push (with -u if needed)
5. Create a GitHub pull request using gh CLI

Examples:
  work commit "Add new feature"
  work commit "Fix bug in authentication"
  work commit "Fix parser" --paths pkg/parser --json`,
	Args: cobra.ExactArgs(1),
	Run:  runCommit,
}

// commitResult is the machine-readable outcome of a commit run, emitted on
// stdout when --json is set. Callers that drive this command unattended (the
// Slack board daemon) need the PR URL and a reason for failure, neither of
// which is recoverable from scraping human output.
type commitResult struct {
	Branch    string `json:"branch"`
	Committed bool   `json:"committed"`
	Pushed    bool   `json:"pushed"`
	PRURL     string `json:"pr_url,omitempty"`
	Error     string `json:"error,omitempty"`
}

// humanOut is where progress chatter goes. In --json mode stdout is reserved
// for the result object, so chatter is diverted to stderr.
func humanOut() io.Writer {
	if commitJSON {
		return os.Stderr
	}
	return os.Stdout
}

// finish emits the result and exits. In --json mode the result object is always
// written, success or failure, so the caller never has to parse prose.
func finish(res commitResult, code int) {
	if commitJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else if res.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", res.Error)
	}
	os.Exit(code)
}

func runCommit(cmd *cobra.Command, args []string) {
	commitMessage := args[0]
	res := commitResult{}

	if !isInsideGitRepo() {
		res.Error = "not in a git repository"
		finish(res, 1)
	}

	currentBranch := getCurrentBranch(".")
	if currentBranch == "" {
		res.Error = "could not determine current branch"
		finish(res, 1)
	}
	res.Branch = currentBranch

	// Step 1: stage. Default stays `git add .` for interactive use; --paths
	// narrows it so an unattended run cannot sweep in build artifacts, logs,
	// or stray .env files that happen to be sitting in the worktree.
	addArgs := []string{"add"}
	if len(commitPaths) > 0 {
		addArgs = append(addArgs, commitPaths...)
	} else {
		addArgs = append(addArgs, ".")
	}
	fmt.Fprintln(humanOut(), "Adding changes...")
	addCmd := exec.Command("git", addArgs...)
	addCmd.Stdout = humanOut()
	addCmd.Stderr = os.Stderr
	if err := addCmd.Run(); err != nil {
		res.Error = fmt.Sprintf("git add failed: %v", err)
		finish(res, 1)
	}

	// Step 2: commit. An empty commit is not fatal — the branch may already
	// carry commits made earlier in the session that still need pushing.
	fmt.Fprintf(humanOut(), "Committing with message: %s\n", commitMessage)
	// git reports "nothing to commit" on stdout, not stderr, so both streams
	// are captured for the check below.
	var commitOutput bytes.Buffer
	commitCmdExec := exec.Command("git", "commit", "-m", commitMessage)
	commitCmdExec.Stdout = io.MultiWriter(&commitOutput, humanOut())
	commitCmdExec.Stderr = io.MultiWriter(&commitOutput, os.Stderr)
	if err := commitCmdExec.Run(); err != nil {
		if isNothingToCommit(commitOutput.String()) {
			fmt.Fprintln(humanOut(), "Nothing new to commit; continuing with existing commits.")
		} else {
			res.Error = fmt.Sprintf("git commit failed: %v", err)
			finish(res, 1)
		}
	} else {
		res.Committed = true
	}

	// Step 3: rebase onto latest. Skipped for a branch that has never been
	// pushed — there is nothing to rebase against, and `git pull --rebase`
	// hard-fails with "no tracking information". The push below sets the
	// upstream with -u.
	//
	// On conflict, abort rather than leaving the worktree mid-rebase — an
	// unattended caller has nobody to resolve it.
	if hasUpstream() {
		fmt.Fprintln(humanOut(), "Pulling latest changes with rebase...")
		pullCmd := exec.Command("git", "pull", "--rebase")
		pullCmd.Stdout = humanOut()
		pullCmd.Stderr = os.Stderr
		if err := pullCmd.Run(); err != nil {
			aborted := abortRebaseIfInProgress()
			res.Error = fmt.Sprintf("git pull --rebase failed: %v", err)
			if aborted {
				res.Error += " (rebase aborted; worktree left clean)"
			}
			finish(res, 1)
		}
	} else {
		fmt.Fprintln(humanOut(), "Branch has no upstream yet; skipping rebase.")
	}

	// Step 4: push.
	fmt.Fprintln(humanOut(), "Pushing to remote...")
	if err := pushWithRetry(currentBranch); err != nil {
		res.Error = fmt.Sprintf("git push failed: %v", err)
		finish(res, 1)
	}
	res.Pushed = true

	// Step 5: PR. A missing PR is a warning, not a failure — the work is
	// already pushed and recoverable by hand.
	fmt.Fprintln(humanOut(), "\nCreating pull request...")
	prURL, err := createPullRequest(currentBranch, commitMessage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nWarning: Could not create PR: %v\n", err)
		if strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "command not found") {
			fmt.Fprintf(os.Stderr, "The 'gh' CLI is not installed. Install it from: https://cli.github.com/\n")
		}
		res.Error = fmt.Sprintf("PR creation failed: %v", err)
		finish(res, 0) // pushed successfully; surface the PR problem without failing the run
	}
	res.PRURL = prURL
	finish(res, 0)
}

// hasUpstream reports whether the current branch tracks a remote branch.
func hasUpstream() bool {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	return cmd.Run() == nil
}

func isNothingToCommit(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "nothing to commit") ||
		strings.Contains(s, "nothing added to commit") ||
		strings.Contains(s, "no changes added to commit")
}

// abortRebaseIfInProgress cleans up a half-finished rebase. Reports whether an
// abort actually ran, so the caller can say so.
func abortRebaseIfInProgress() bool {
	gitDir, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return false
	}
	dir := strings.TrimSpace(string(gitDir))
	inProgress := false
	for _, marker := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			inProgress = true
			break
		}
	}
	if !inProgress {
		return false
	}
	abort := exec.Command("git", "rebase", "--abort")
	abort.Stdout = humanOut()
	abort.Stderr = os.Stderr
	return abort.Run() == nil
}

// networkErrorPatterns are the git failures worth retrying. Retrying anything
// else — a non-fast-forward reject, a permissions problem, a protected branch —
// just burns 30 seconds before failing anyway.
var networkErrorPatterns = []string{
	"could not resolve host",
	"connection timed out",
	"connection refused",
	"connection reset",
	"operation timed out",
	"temporary failure in name resolution",
	"failed to connect",
	"network is unreachable",
	"the remote end hung up",
	"rpc failed",
	"early eof",
	"ssh: connect to host",
	"unable to access",
}

func isRetryableGitError(stderr string) bool {
	s := strings.ToLower(stderr)
	for _, p := range networkErrorPatterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// pushWithRetry pushes with exponential backoff, retrying only transient
// network failures.
func pushWithRetry(branch string) error {
	maxRetries := 4
	delays := []int{2, 4, 8, 16} // seconds

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var errBuf bytes.Buffer
		pushCmd := exec.Command("git", "push", "-u", "origin", branch)
		pushCmd.Stdout = humanOut()
		pushCmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)

		err := pushCmd.Run()
		if err == nil {
			return nil
		}

		if attempt == maxRetries {
			return fmt.Errorf("push failed after %d attempts: %w", maxRetries+1, err)
		}

		if !isRetryableGitError(errBuf.String()) {
			return err
		}

		delay := delays[attempt]
		fmt.Fprintf(humanOut(), "Push failed (network), retrying in %ds... (attempt %d/%d)\n",
			delay, attempt+1, maxRetries+1)
		time.Sleep(time.Duration(delay) * time.Second)
	}

	return fmt.Errorf("push failed after retries")
}

// createPullRequest creates a PR via gh and returns its URL.
//
// Stdin is detached and --base/--head are always explicit: gh prompts when it
// has to guess either, which hangs forever with no terminal attached.
func createPullRequest(branch string, commitMessage string) (string, error) {
	base := commitBase
	if base == "" {
		base = getDefaultBranch(".")
	}

	commitsCmd := exec.Command("git", "log", fmt.Sprintf("origin/%s..HEAD", base), "--oneline")
	commitsOutput, err := commitsCmd.Output()
	if err != nil {
		commitsOutput = []byte(commitMessage)
	}

	commits := strings.TrimSpace(string(commitsOutput))
	if commits == "" {
		return "", fmt.Errorf("no commits to create PR from")
	}

	prTitle := commitMessage
	prBody := fmt.Sprintf("## Summary\n\n%s\n\n## Commits\n```\n%s\n```", commitMessage, commits)

	tempDir := os.TempDir()
	bodyFile := filepath.Join(tempDir, fmt.Sprintf("pr-body-%d.txt", os.Getpid()))
	if err := os.WriteFile(bodyFile, []byte(prBody), 0644); err != nil {
		return "", fmt.Errorf("failed to write PR body file: %w", err)
	}
	defer os.Remove(bodyFile)

	var prOut bytes.Buffer
	prCmd := exec.Command("gh", "pr", "create",
		"--title", prTitle,
		"--body-file", bodyFile,
		"--base", base,
		"--head", branch,
	)
	prCmd.Stdout = &prOut
	prCmd.Stderr = os.Stderr
	prCmd.Stdin = nil

	runErr := prCmd.Run()
	output := prOut.String()
	if output != "" {
		fmt.Fprint(humanOut(), output)
	}
	if runErr != nil {
		return "", fmt.Errorf("gh pr create failed: %w", runErr)
	}

	return extractPRURL(output), nil
}

// extractPRURL pulls the PR link out of gh's output, which prints it on its own
// line among other chatter.
func extractPRURL(output string) string {
	// Prefer a PR link; fall back to any URL in case the path shape changes.
	if u := findURL(output, "/pull/"); u != "" {
		return u
	}
	return findURL(output, "")
}

// findURL returns the first https URL containing must (empty matches any).
// The URL is not required to start the line — gh usually prints it alone, but
// tolerating a prefix costs nothing.
func findURL(output, must string) string {
	for _, line := range strings.Split(output, "\n") {
		i := strings.Index(line, "https://")
		if i < 0 {
			continue
		}
		u := strings.TrimSpace(line[i:])
		if j := strings.IndexAny(u, " \t"); j >= 0 {
			u = u[:j]
		}
		if must == "" || strings.Contains(u, must) {
			return u
		}
	}
	return ""
}

func init() {
	commitCmd.Flags().BoolVar(&commitJSON, "json", false,
		"emit a machine-readable result on stdout (progress goes to stderr)")
	commitCmd.Flags().StringSliceVar(&commitPaths, "paths", nil,
		"stage only these paths instead of everything (repeatable, or comma-separated)")
	commitCmd.Flags().StringVar(&commitBase, "base", "",
		"base branch for the PR (defaults to the repository's default branch)")

	// Register commit command with root
	rootCmd.AddCommand(commitCmd)
}
