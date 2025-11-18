package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/services"
)

var syncCmd = &cobra.Command{
	Use:   "sync [repo]",
	Short: "Sync default branch across repositories",
	Long: `Sync the default branch (main/master) across all repositories or a specific repository.

This command helps keep your default branches up-to-date by:
  - Switching to the default branch in the main worktree
  - Pulling the latest changes with rebase
  - Reporting any errors or conflicts

Examples:
  work sync              # Sync all repositories
  work sync ai-workflow  # Sync specific repository`,
	ValidArgsFunction: completeReposForSync,
	Run:               runSync,
}

// SyncResult holds the result of syncing a repository
type SyncResult struct {
	RepoName      string
	DefaultBranch string
	Success       bool
	Error         error
	Message       string
}

func runSync(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var repoFilter string
	if len(args) > 0 {
		repoFilter = args[0]
	}

	repos := discoverRepos()
	if repos == nil {
		return
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found in git folder")
		return
	}

	// Filter to specific repo if provided
	var reposToSync []string
	if repoFilter != "" {
		found := false
		for _, repoPath := range repos {
			if filepath.Base(repoPath) == repoFilter {
				reposToSync = []string{repoPath}
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "Error: Repository '%s' not found\n", repoFilter)
			os.Exit(1)
		}
	} else {
		reposToSync = repos
	}

	if len(reposToSync) == 1 {
		fmt.Printf("Syncing %s...\n", filepath.Base(reposToSync[0]))
	} else {
		fmt.Printf("Syncing %d repositories...\n", len(reposToSync))
	}

	// Process repositories concurrently
	var wg sync.WaitGroup
	results := make(chan SyncResult, len(reposToSync))

	for _, repoPath := range reposToSync {
		wg.Add(1)
		go func(rPath string) {
			defer wg.Done()
			result := syncRepository(ctx, rPath)
			results <- result
		}(repoPath)
	}

	// Close results channel after all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and display results
	successCount := 0
	errorCount := 0
	var errors []SyncResult

	for result := range results {
		if result.Success {
			successCount++
			fmt.Printf("✓ %s (%s): %s\n", result.RepoName, result.DefaultBranch, result.Message)
		} else {
			errorCount++
			errors = append(errors, result)
			fmt.Fprintf(os.Stderr, "✗ %s: %s\n", result.RepoName, result.Error.Error())
		}
	}

	// Summary
	fmt.Println()
	if errorCount == 0 {
		fmt.Printf("All %d repositories synced successfully\n", successCount)
	} else {
		fmt.Printf("Synced: %d successful, %d failed\n", successCount, errorCount)
		if len(errors) > 0 {
			fmt.Println("\nFailed repositories:")
			for _, err := range errors {
				fmt.Printf("  - %s: %s\n", err.RepoName, err.Error.Error())
			}
		}
	}
}

// syncRepository syncs the default branch of a single repository
func syncRepository(ctx context.Context, repoPath string) SyncResult {
	repoName := filepath.Base(repoPath)
	mainPath := filepath.Join(repoPath, "main")

	result := SyncResult{
		RepoName: repoName,
	}

	runner := services.Get().GitRunner

	// Get default branch
	defaultBranch, err := runner.GetDefaultBranch(ctx, mainPath)
	if err != nil {
		// Fallback to checking locally
		defaultBranch = getLocalDefaultBranch(ctx, mainPath)
		if defaultBranch == "" {
			result.Error = fmt.Errorf("could not determine default branch")
			return result
		}
	}
	result.DefaultBranch = defaultBranch

	// Get current branch
	currentBranch, err := runner.GetCurrentBranch(ctx, mainPath)
	if err != nil {
		result.Error = fmt.Errorf("could not get current branch: %w", err)
		return result
	}

	// Switch to default branch if not already on it
	if currentBranch != defaultBranch {
		_, err = runner.Run(ctx, mainPath, "switch", defaultBranch)
		if err != nil {
			result.Error = fmt.Errorf("could not switch to %s: %w", defaultBranch, err)
			return result
		}
	}

	// Check for uncommitted changes
	status, err := runner.GetGitStatus(ctx, mainPath)
	if err != nil {
		result.Error = fmt.Errorf("could not check git status: %w", err)
		return result
	}

	if len(status) > 0 {
		result.Error = fmt.Errorf("has uncommitted changes, refusing to sync")
		return result
	}

	// Pull with rebase
	pullResult, err := runner.Run(ctx, mainPath, "pull", "--rebase")
	if err != nil {
		result.Error = fmt.Errorf("failed to pull: %w", err)
		return result
	}

	// Determine what happened
	if strings.Contains(pullResult.Stdout, "Already up to date") {
		result.Message = "Already up to date"
	} else if strings.Contains(pullResult.Stdout, "Fast-forward") {
		result.Message = "Updated"
	} else {
		result.Message = "Synced"
	}

	result.Success = true
	return result
}

// getLocalDefaultBranch attempts to determine the default branch locally
func getLocalDefaultBranch(ctx context.Context, repoPath string) string {
	runner := services.Get().GitRunner

	// Try common default branches
	for _, branch := range []string{"main", "master"} {
		if runner.BranchExists(ctx, repoPath, branch) {
			return branch
		}
	}

	// Try to get from remote HEAD
	output, err := runner.RunSimple(ctx, repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// Output will be like "refs/remotes/origin/main"
		branch := strings.TrimPrefix(output, "refs/remotes/origin/")
		if branch != "" && branch != output {
			return branch
		}
	}

	return ""
}

// completeReposForSync provides tab completion for repository names
func completeReposForSync(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete the first argument (repo name)
	if len(args) > 0 {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	repos := discoverRepos()
	if repos == nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	// Extract just the repo names
	var repoNames []string
	for _, repoPath := range repos {
		repoNames = append(repoNames, filepath.Base(repoPath))
	}

	return repoNames, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
