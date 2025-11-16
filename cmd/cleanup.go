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
	"github.com/velvee-ai/ai-workflow/pkg/config"
	"github.com/velvee-ai/ai-workflow/pkg/services"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleanup merged or deleted branch worktrees",
	Long: `Clean up stale git worktrees that have been merged or deleted.

This command helps keep your git folders clean by removing worktrees for branches
that have been merged to the default branch or deleted from the remote.

Safety:
  - Only removes worktrees with no uncommitted changes (git status is clean)
  - Verifies branch is either merged OR deleted remotely before removal
  - Provides dry-run and interactive modes for safety

Subcommands:
  work cleanup list   - List all worktrees and their status
  work cleanup scan   - Show what would be cleaned (dry-run)
  work cleanup run    - Interactively cleanup stale worktrees`,
}

var cleanupListCmd = &cobra.Command{
	Use:   "list [repo]",
	Short: "List all worktrees and their status",
	Long: `List all worktrees across all repositories (or specific repo) and show their current status.

Status indicators:
  [active]   - Branch is active and up-to-date
  [merged]   - Branch has been merged to default branch
  [deleted]  - Remote branch has been deleted
  [changes]  - Has uncommitted changes (cannot be cleaned)`,
	Run: runCleanupList,
}

var cleanupScanCmd = &cobra.Command{
	Use:   "scan [repo]",
	Short: "Show what would be cleaned (dry-run)",
	Long: `Scan for stale worktrees and show what would be removed without actually deleting anything.

This is a safe way to preview what the cleanup would do before running it.`,
	Run: runCleanupScan,
}

var (
	cleanupForce bool
)

var cleanupRunCmd = &cobra.Command{
	Use:   "run [repo]",
	Short: "Interactively cleanup stale worktrees",
	Long: `Clean up stale worktrees with interactive confirmation for each removal.

The command will:
  1. Scan all repositories for stale worktrees
  2. Identify worktrees that are merged or have deleted remote branches
  3. Skip worktrees with uncommitted changes
  4. Ask for confirmation before removing each worktree (unless --force is used)
  5. Clean up git metadata with 'git worktree prune'`,
	Run: runCleanupRun,
}

// WorktreeInfo holds information about a worktree and its status
type WorktreeInfo struct {
	RepoName      string
	RepoPath      string
	Path          string
	Branch        string
	IsMerged      bool
	IsDeleted     bool
	HasChanges    bool
	Reason        string
	LastModified  time.Time
	SizeBytes     int64
	DefaultBranch string
}

// IsStale returns true if the worktree can be cleaned up
func (w *WorktreeInfo) IsStale() bool {
	return !w.HasChanges && (w.IsMerged || w.IsDeleted)
}

// StatusString returns a colored status string for display
func (w *WorktreeInfo) StatusString() string {
	if w.HasChanges {
		return "[changes]"
	}
	if w.IsMerged {
		return "[merged]"
	}
	if w.IsDeleted {
		return "[deleted]"
	}
	return "[active]"
}

func runCleanupList(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	repoFilter := ""
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

	// Process repositories concurrently
	type repoResult struct {
		repoName  string
		worktrees []WorktreeInfo
		err       error
	}

	var wg sync.WaitGroup
	results := make(chan repoResult, len(repos))

	for _, repoPath := range repos {
		repoName := filepath.Base(repoPath)
		if repoFilter != "" && repoName != repoFilter {
			continue
		}

		wg.Add(1)
		go func(rPath, rName string) {
			defer wg.Done()

			worktrees, err := scanWorktrees(ctx, rPath, rName)
			results <- repoResult{
				repoName:  rName,
				worktrees: worktrees,
				err:       err,
			}
		}(repoPath, repoName)
	}

	// Close results channel after all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and display results
	totalWorktrees := 0
	staleWorktrees := 0
	hasResults := false

	for result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", result.repoName, result.err)
			continue
		}

		if len(result.worktrees) == 0 {
			continue
		}

		hasResults = true
		fmt.Printf("\nRepository: %s\n", result.repoName)
		for _, wt := range result.worktrees {
			totalWorktrees++
			if wt.IsStale() {
				staleWorktrees++
			}

			branchDisplay := filepath.Base(wt.Path)
			fmt.Printf("  %-30s %s", branchDisplay+"/", wt.StatusString())
			if wt.Reason != "" {
				fmt.Printf(" - %s", wt.Reason)
			}
			fmt.Println()
		}
	}

	if !hasResults || totalWorktrees == 0 {
		fmt.Println("\nNo worktrees found")
		return
	}

	fmt.Printf("\nTotal: %d worktrees", totalWorktrees)
	if staleWorktrees > 0 {
		fmt.Printf(" (%d can be cleaned up)\n", staleWorktrees)
		fmt.Println("Run 'work cleanup scan' to see details")
	} else {
		fmt.Println(" (all up-to-date)")
	}
}

func runCleanupScan(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	repoFilter := ""
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

	fmt.Println("Scanning for stale worktrees...")

	// Process repositories concurrently
	type repoResult struct {
		repoName  string
		worktrees []WorktreeInfo
		err       error
	}

	var wg sync.WaitGroup
	results := make(chan repoResult, len(repos))

	for _, repoPath := range repos {
		repoName := filepath.Base(repoPath)
		if repoFilter != "" && repoName != repoFilter {
			continue
		}

		wg.Add(1)
		go func(rPath, rName string) {
			defer wg.Done()

			worktrees, err := scanWorktrees(ctx, rPath, rName)
			results <- repoResult{
				repoName:  rName,
				worktrees: worktrees,
				err:       err,
			}
		}(repoPath, repoName)
	}

	// Close results channel after all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect stale worktrees
	var allStale []WorktreeInfo
	totalSize := int64(0)

	for result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", result.repoName, result.err)
			continue
		}

		for _, wt := range result.worktrees {
			if wt.IsStale() {
				allStale = append(allStale, wt)
				totalSize += wt.SizeBytes
			}
		}
	}

	if len(allStale) == 0 {
		fmt.Println("\nNo stale worktrees found. Everything is clean!")
		return
	}

	fmt.Println()
	currentRepo := ""
	for _, wt := range allStale {
		if currentRepo != wt.RepoName {
			currentRepo = wt.RepoName
			fmt.Printf("%s:\n", wt.RepoName)
		}

		branchDisplay := filepath.Base(wt.Path)
		fmt.Printf("  %s/\n", branchDisplay)
		fmt.Printf("    Reason: %s\n", wt.Reason)
		fmt.Printf("    Last modified: %s\n", wt.LastModified.Format("2006-01-02 15:04"))
		if wt.SizeBytes > 0 {
			fmt.Printf("    Size: %s\n", formatBytes(wt.SizeBytes))
		}
		fmt.Printf("    Safe to remove: ✓\n")
		fmt.Println()
	}

	fmt.Printf("Total: %d worktrees", len(allStale))
	if totalSize > 0 {
		fmt.Printf(" (%s)", formatBytes(totalSize))
	}
	fmt.Println()
	fmt.Println("Run 'work cleanup run' to remove them")
}

func runCleanupRun(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	repoFilter := ""
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

	fmt.Println("Scanning for stale worktrees...")

	// Process repositories concurrently
	type repoResult struct {
		repoName  string
		worktrees []WorktreeInfo
		err       error
	}

	var wg sync.WaitGroup
	results := make(chan repoResult, len(repos))

	for _, repoPath := range repos {
		repoName := filepath.Base(repoPath)
		if repoFilter != "" && repoName != repoFilter {
			continue
		}

		wg.Add(1)
		go func(rPath, rName string) {
			defer wg.Done()

			worktrees, err := scanWorktrees(ctx, rPath, rName)
			results <- repoResult{
				repoName:  rName,
				worktrees: worktrees,
				err:       err,
			}
		}(repoPath, repoName)
	}

	// Close results channel after all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect stale worktrees
	var allStale []WorktreeInfo
	totalSize := int64(0)

	for result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", result.repoName, result.err)
			continue
		}

		for _, wt := range result.worktrees {
			if wt.IsStale() {
				allStale = append(allStale, wt)
				totalSize += wt.SizeBytes
			}
		}
	}

	if len(allStale) == 0 {
		fmt.Println("\nNo stale worktrees found. Everything is clean!")
		return
	}

	fmt.Printf("\nFound %d stale worktrees to clean up\n\n", len(allStale))

	removed := 0
	skipped := 0
	freedSpace := int64(0)

	for _, wt := range allStale {
		branchDisplay := filepath.Base(wt.Path)

		shouldRemove := cleanupForce
		if !cleanupForce {
			fmt.Printf("Remove worktree '%s/%s' (%s)? [y/N] ", wt.RepoName, branchDisplay, wt.Reason)

			var response string
			fmt.Scanln(&response)
			response = strings.ToLower(strings.TrimSpace(response))
			shouldRemove = (response == "y" || response == "yes")
		}

		if shouldRemove {
			if err := removeWorktreeSafely(ctx, wt); err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ Error removing worktree: %v\n", err)
				skipped++
			} else {
				fmt.Printf("  ✓ Removed %s/%s/\n", wt.RepoName, branchDisplay)
				removed++
				freedSpace += wt.SizeBytes
			}
		} else {
			fmt.Println("  Skipped")
			skipped++
		}
		if !cleanupForce {
			fmt.Println()
		}
	}

	fmt.Printf("Cleanup complete!\n")
	fmt.Printf("  Removed: %d worktrees", removed)
	if freedSpace > 0 {
		fmt.Printf(" (%s freed)", formatBytes(freedSpace))
	}
	fmt.Println()
	if skipped > 0 {
		fmt.Printf("  Skipped: %d worktrees\n", skipped)
	}

	// Prune worktree metadata for each repo
	if removed > 0 {
		fmt.Println("\nCleaning up git metadata...")
		processedRepos := make(map[string]bool)
		for _, wt := range allStale {
			if !processedRepos[wt.RepoPath] {
				processedRepos[wt.RepoPath] = true
				runner := services.Get().GitRunner
				mainPath := filepath.Join(wt.RepoPath, "main")
				if err := runner.PruneWorktrees(ctx, mainPath); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: Could not prune %s: %v\n", wt.RepoName, err)
				}
			}
		}
		fmt.Println("  ✓ Metadata cleaned")
	}
}

// discoverRepos finds all git repositories in the default git folder
func discoverRepos() []string {
	gitFolder := config.GetString("default_git_folder")
	if gitFolder == "" {
		fmt.Fprintf(os.Stderr, "Error: default_git_folder not configured\n")
		fmt.Fprintf(os.Stderr, "Run: work config set default_git_folder ~/git\n")
		return nil
	}

	// Expand home directory
	if strings.HasPrefix(gitFolder, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not get home directory: %v\n", err)
			return nil
		}
		gitFolder = filepath.Join(homeDir, gitFolder[2:])
	}

	entries, err := os.ReadDir(gitFolder)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: Git folder does not exist: %s\n", gitFolder)
			fmt.Fprintf(os.Stderr, "Run: work config set default_git_folder <path>\n")
		} else {
			fmt.Fprintf(os.Stderr, "Error reading git folder: %v\n", err)
		}
		return nil
	}

	var repos []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if this is a repo container (has a "main" subdirectory with .git)
		mainPath := filepath.Join(gitFolder, entry.Name(), "main")
		gitPath := filepath.Join(mainPath, ".git")

		if _, err := os.Stat(gitPath); err == nil {
			repos = append(repos, filepath.Join(gitFolder, entry.Name()))
		}
	}

	return repos
}

// scanWorktrees scans a repository for all worktrees and their status
func scanWorktrees(ctx context.Context, repoPath, repoName string) ([]WorktreeInfo, error) {
	runner := services.Get().GitRunner
	mainPath := filepath.Join(repoPath, "main")

	// Get default branch
	defaultBranch, err := runner.GetDefaultBranch(ctx, mainPath)
	if err != nil {
		// Fallback to "main" if we can't determine
		defaultBranch = "main"
	}

	// Fetch and prune to get latest remote state
	if err := runner.FetchPrune(ctx, mainPath); err != nil {
		// Non-fatal, continue without fetch
		fmt.Fprintf(os.Stderr, "  Warning: Could not fetch from remote for %s: %v\n", repoName, err)
	}

	// List all worktrees
	worktrees, err := runner.ListWorktrees(ctx, mainPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var result []WorktreeInfo
	for _, wt := range worktrees {
		// Skip the main worktree
		if filepath.Base(wt.Path) == "main" {
			continue
		}

		info := WorktreeInfo{
			RepoName:      repoName,
			RepoPath:      repoPath,
			Path:          wt.Path,
			Branch:        wt.Branch,
			DefaultBranch: defaultBranch,
		}

		// Get last modified time
		if stat, err := os.Stat(filepath.Join(wt.Path, ".git")); err == nil {
			info.LastModified = stat.ModTime()
		}

		// Calculate directory size (approximate)
		if size, err := getDirSize(wt.Path); err == nil {
			info.SizeBytes = size
		}

		// Check for uncommitted changes
		status, err := runner.GetGitStatus(ctx, wt.Path)
		if err != nil {
			info.HasChanges = true // Assume changes if we can't check
			info.Reason = "Error checking status"
		} else if len(status) > 0 {
			info.HasChanges = true
			info.Reason = "Has uncommitted changes"
		}

		// Only check merge/delete status if no changes
		if !info.HasChanges {
			// Check if merged
			isMerged, err := runner.IsBranchMerged(ctx, mainPath, wt.Branch, defaultBranch)
			if err == nil && isMerged {
				info.IsMerged = true
				info.Reason = fmt.Sprintf("Merged to %s", defaultBranch)
			}

			// Check if remote branch exists
			if !info.IsMerged {
				exists, err := runner.RemoteBranchExists(ctx, wt.Path, wt.Branch)
				if err == nil && !exists {
					info.IsDeleted = true
					info.Reason = "Remote branch deleted"
				}
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// removeWorktreeSafely removes a worktree after safety checks
func removeWorktreeSafely(ctx context.Context, info WorktreeInfo) error {
	runner := services.Get().GitRunner
	mainPath := filepath.Join(info.RepoPath, "main")

	// Double-check git status before removal
	status, err := runner.GetGitStatus(ctx, info.Path)
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	if len(status) > 0 {
		return fmt.Errorf("worktree has uncommitted changes, refusing to remove")
	}

	// Remove the worktree
	if err := runner.RemoveWorktree(ctx, mainPath, info.Path); err != nil {
		return fmt.Errorf("git worktree remove failed: %w", err)
	}

	return nil
}

// getDirSize calculates the approximate size of a directory
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// formatBytes formats bytes as human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func init() {
	// Add subcommands to cleanup command
	cleanupCmd.AddCommand(cleanupListCmd)
	cleanupCmd.AddCommand(cleanupScanCmd)
	cleanupCmd.AddCommand(cleanupRunCmd)

	// Add flags to run command
	cleanupRunCmd.Flags().BoolVarP(&cleanupForce, "force", "f", false, "Skip confirmation prompts and remove all stale worktrees")

	// Register cleanup command with root
	rootCmd.AddCommand(cleanupCmd)
}
