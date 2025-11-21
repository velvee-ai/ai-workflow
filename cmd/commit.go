package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var commitCmd = &cobra.Command{
	Use:   "commit <message>",
	Short: "Add, commit, pull, push, and create PR",
	Long: `Automate the git workflow: add all changes, commit, pull with rebase, push, and create a pull request.

This command performs the following steps:
1. git add .
2. git commit -m "<message>"
3. git pull --rebase
4. git push (with -u if needed)
5. Create a GitHub pull request using gh CLI

Examples:
  work commit "Add new feature"
  work commit "Fix bug in authentication"`,
	Args: cobra.ExactArgs(1),
	Run:  runCommit,
}

func runCommit(cmd *cobra.Command, args []string) {
	commitMessage := args[0]

	// Check if we're in a git repository
	if !isInsideGitRepo() {
		fmt.Fprintf(os.Stderr, "Error: Not in a git repository\n")
		os.Exit(1)
	}

	// Get current branch name
	currentBranch := getCurrentBranch(".")
	if currentBranch == "" {
		fmt.Fprintf(os.Stderr, "Error: Could not determine current branch\n")
		os.Exit(1)
	}

	// Step 1: git add .
	fmt.Println("Adding all changes...")
	addCmd := exec.Command("git", "add", ".")
	addCmd.Stdout = os.Stdout
	addCmd.Stderr = os.Stderr
	if err := addCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: git add failed: %v\n", err)
		os.Exit(1)
	}

	// Step 2: git commit
	fmt.Printf("Committing with message: %s\n", commitMessage)
	commitCmd := exec.Command("git", "commit", "-m", commitMessage)
	commitCmd.Stdout = os.Stdout
	commitCmd.Stderr = os.Stderr
	if err := commitCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: git commit failed: %v\n", err)
		os.Exit(1)
	}

	// Step 3: git pull --rebase
	fmt.Println("Pulling latest changes with rebase...")
	pullCmd := exec.Command("git", "pull", "--rebase")
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: git pull --rebase failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please resolve conflicts and push manually\n")
		os.Exit(1)
	}

	// Step 4: git push (with retry logic)
	fmt.Println("Pushing to remote...")
	if err := pushWithRetry(currentBranch); err != nil {
		fmt.Fprintf(os.Stderr, "Error: git push failed: %v\n", err)
		os.Exit(1)
	}

	// Step 5: Create pull request using gh CLI
	fmt.Println("\nCreating pull request...")
	if err := createPullRequest(currentBranch, commitMessage); err != nil {
		fmt.Fprintf(os.Stderr, "\nWarning: Could not create PR: %v\n", err)
		if strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "command not found") {
			fmt.Fprintf(os.Stderr, "The 'gh' CLI is not installed. Install it from: https://cli.github.com/\n")
		}
		fmt.Fprintf(os.Stderr, "You can create the PR manually at: https://github.com/compare/%s\n", currentBranch)
		return
	}
}

// pushWithRetry attempts to push with exponential backoff retry logic
func pushWithRetry(branch string) error {
	maxRetries := 4
	delays := []int{2, 4, 8, 16} // seconds

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Try to push with -u flag to set upstream if needed
		pushCmd := exec.Command("git", "push", "-u", "origin", branch)
		pushCmd.Stdout = os.Stdout
		pushCmd.Stderr = os.Stderr

		err := pushCmd.Run()
		if err == nil {
			return nil // Success
		}

		// If this was the last attempt, return the error
		if attempt == maxRetries {
			return fmt.Errorf("push failed after %d attempts", maxRetries+1)
		}

		// Check if it's a network error (exit code 128 often indicates network issues)
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode == 128 || exitCode == 1 {
				// Network error, retry with exponential backoff
				delay := delays[attempt]
				fmt.Printf("Push failed, retrying in %d seconds... (attempt %d/%d)\n", delay, attempt+1, maxRetries+1)

				// Sleep for the delay (cross-platform)
				time.Sleep(time.Duration(delay) * time.Second)
				continue
			}
		}

		// Non-network error, don't retry
		return err
	}

	return fmt.Errorf("push failed after retries")
}

// createPullRequest creates a pull request using gh CLI
func createPullRequest(branch string, commitMessage string) error {
	// Get default branch for comparison
	defaultBranch := getDefaultBranch(".")

	// Get all commits in this branch that aren't in the base branch
	commitsCmd := exec.Command("git", "log", fmt.Sprintf("origin/%s..HEAD", defaultBranch), "--oneline")
	commitsOutput, err := commitsCmd.Output()
	if err != nil {
		// If we can't get commits, just use the latest commit message
		commitsOutput = []byte(commitMessage)
	}

	commits := strings.TrimSpace(string(commitsOutput))
	if commits == "" {
		return fmt.Errorf("no commits to create PR from")
	}

	// Use the commit message as the PR title
	prTitle := commitMessage

	// Create PR body with summary of commits
	prBody := fmt.Sprintf("## Summary\n\n%s\n\n## Commits\n```\n%s\n```", commitMessage, commits)

	// Create the PR using gh CLI with heredoc for body
	// Using bash to handle heredoc properly
	bashScript := fmt.Sprintf(`gh pr create --title "%s" --body "$(cat <<'EOF'
%s
EOF
)"`, prTitle, prBody)

	prCmd := exec.Command("bash", "-c", bashScript)
	prCmd.Stdout = os.Stdout
	prCmd.Stderr = os.Stderr
	prCmd.Stdin = os.Stdin

	if err := prCmd.Run(); err != nil {
		return fmt.Errorf("gh pr create failed: %w", err)
	}

	return nil
}

func init() {
	// Register commit command with root
	rootCmd.AddCommand(commitCmd)
}
