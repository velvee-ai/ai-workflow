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

var (
	syncOff  bool
	syncOnce bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [repo]",
	Short: "Sync default branch and (for a single repo) enable auto-sync",
	Long: `Sync the default branch (main/master) across your repositories.

Without arguments: pull --rebase on every discovered repo (one-shot).

With a repo argument: full auto-sync setup (idempotent) — creates a
smee.io channel if needed, installs a GitHub push webhook on the repo,
starts the launchd listener daemon, and then pulls --rebase once.
After the first run, further pushes to the default branch sync
automatically in the background.

Examples:
  work sync                     # One-shot sync of every repo
  work sync ai-workflow         # Full auto-sync setup + pull for one repo
  work sync ai-workflow --once  # Skip setup, just pull once
  work sync ai-workflow --off   # Remove the GitHub webhook for this repo`,
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
	if syncOff {
		runSyncDisable(cmd, args)
		return
	}
	if len(args) > 0 && !syncOnce {
		runSyncEnable(cmd, args[0])
		return
	}
	runSyncPullOnly(cmd, args)
}

// runSyncEnable is the "do it all" path for a single repo: ensure smee
// channel + GitHub webhook + launchd daemon, then pull once.
func runSyncEnable(cmd *cobra.Command, repoName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	gitFolder := resolvedGitFolder()
	if gitFolder == "" {
		fmt.Fprintln(os.Stderr, "Error: default_git_folder is not configured or does not exist")
		fmt.Fprintln(os.Stderr, "Run: work setup")
		os.Exit(1)
	}

	localPath, err := enableAutoSync(ctx, gitFolder, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	result := syncRepository(ctx, localPath)
	if result.Success {
		fmt.Printf("✓ %s (%s): %s\n", result.RepoName, result.DefaultBranch, result.Message)
		return
	}
	fmt.Fprintf(os.Stderr, "✗ %s: %s\n", result.RepoName, result.Error.Error())
	os.Exit(1)
}

// runSyncDisable removes the webhook for a single repo.
func runSyncDisable(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: --off requires a repo name")
		fmt.Fprintln(os.Stderr, "Example: work sync ai-workflow --off")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	gitFolder := resolvedGitFolder()
	if gitFolder == "" {
		fmt.Fprintln(os.Stderr, "Error: default_git_folder is not configured or does not exist")
		os.Exit(1)
	}
	if err := disableAutoSync(ctx, gitFolder, args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runSyncPullOnly is the classic sync-all (and `--once <repo>`) path.
func runSyncPullOnly(cmd *cobra.Command, args []string) {
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
	syncCmd.Flags().BoolVar(&syncOff, "off", false, "Remove the GitHub webhook for this repo (requires a repo name)")
	syncCmd.Flags().BoolVar(&syncOnce, "once", false, "Skip webhook and daemon setup; just pull --rebase once")
	rootCmd.AddCommand(syncCmd)
}
