package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/config"
	"github.com/velvee-ai/ai-workflow/pkg/gitexec"
)

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git operations",
	Long:  `Perform git operations with enhanced functionality.`,
}

var gitStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show git status",
	Long:  `Execute git status and display repository status.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runGitCommand("status"); err != nil {
			fmt.Fprintf(os.Stderr, "Error running git status: %v\n", err)
			os.Exit(1)
		}
	},
}

var gitBranchCmd = &cobra.Command{
	Use:   "branch",
	Short: "List git branches",
	Long:  `List all git branches in the repository.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runGitCommand("branch", "-a"); err != nil {
			fmt.Fprintf(os.Stderr, "Error listing branches: %v\n", err)
			os.Exit(1)
		}
	},
}

// runGitCommand executes a git command with the given arguments
func runGitCommand(args ...string) error {
	gitCmd := exec.Command("git", args...)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	gitCmd.Stdin = os.Stdin
	return gitCmd.Run()
}

// getDefaultBranch returns the repository's default branch name.
// It attempts to detect it using gh CLI, falling back to config, then "main".
func getDefaultBranch(workDir string) string {
	// Try to detect using gitexec package with gh CLI
	runner := gitexec.New(5 * time.Second)
	ctx := context.Background()

	if branch, err := runner.GetDefaultBranch(ctx, workDir); err == nil && branch != "" {
		return branch
	}

	// Fall back to configured checkout_base_branch
	if baseBranch := config.GetString("checkout_base_branch"); baseBranch != "" {
		return baseBranch
	}

	// Final fallback to "main"
	return "main"
}

func init() {
	// Add subcommands to git command
	gitCmd.AddCommand(gitStatusCmd)
	gitCmd.AddCommand(gitBranchCmd)

	// Register git command with root
	rootCmd.AddCommand(gitCmd)
}
