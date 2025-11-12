package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/config"
)

// Cache for repo list and branch lists to speed up autocomplete
var (
	repoListCache      []string
	repoListCacheTime  time.Time
	repoListCacheTTL   = 5 * time.Minute
	branchListCache    = make(map[string]branchCacheEntry)
	branchListCacheTTL = 5 * time.Minute
)

type branchCacheEntry struct {
	branches  []string
	fetchedAt time.Time
}

var checkoutCmd = &cobra.Command{
	Use:   "checkout [repo] [branch]",
	Short: "Git checkout operations with worktree support",
	Long: `Manage git repositories and branches using worktrees for parallel development.

Direct Usage (with autocomplete):
  work checkout <repo> <branch>

  This will create or switch to a worktree for the specified branch in the given repository.
  The repo name should match a directory in your configured git folder.

Subcommands:
  work checkout root <url>     - Clone a new repository
  work checkout branch <name>  - Checkout branch in current repo`,
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeGitRepos,
	Run:               runCheckoutDirect,
}

var checkoutRootCmd = &cobra.Command{
	Use:   "root <git-clone-url>",
	Short: "Clone repository into structured folder layout",
	Long: `Clone a repository into a structured folder:
- Creates a container folder named after the repository
- Clones the repo into a 'main' subfolder within that container
- Sets up the foundation for branch worktrees

Example:
  work checkout root https://github.com/user/repo.git

This creates:
  repo/
    └── main/  (cloned repository)`,
	Args: cobra.ExactArgs(1),
	Run:  runCheckoutRoot,
}

var checkoutBranchCmd = &cobra.Command{
	Use:   "branch <branch-name-or-github-issue-url>",
	Short: "Checkout branch using git worktree",
	Long: `Create or switch to a git worktree for a branch.

Supports:
- Branch names: Creates/switches to worktree for the branch
- GitHub issue URLs: Creates branch from issue and sets up worktree

Example:
  work checkout branch feature-123
  work checkout branch https://github.com/user/repo/issues/42

This creates a worktree in the container folder:
  repo/
    ├── main/
    └── feature-123/  (worktree)`,
	Args: cobra.ExactArgs(1),
	Run:  runCheckoutBranch,
}

var checkoutCacheClearCmd = &cobra.Command{
	Use:   "cache-clear",
	Short: "Clear autocomplete cache for repos and branches",
	Long: `Clear the cached repository and branch lists used for autocomplete.

This is useful when you want to immediately see new repositories or branches
without waiting for the cache to expire (default: 5 minutes).

Example:
  work checkout cache-clear`,
	Run: runCacheClear,
}

var checkoutNewCmd = &cobra.Command{
	Use:   "new <repo> <branch>",
	Short: "Create a remote branch via GitHub and checkout locally",
	Long: `Create a new branch on GitHub from the base branch (default: main) and then
create a local worktree for it.

This command:
1. Creates the branch remotely via GitHub CLI (gh)
2. Uses the configured checkout_base_branch (default: main) as the base
3. Creates/switches to a local worktree for the new branch
4. Opens the worktree in your configured IDE

Example:
  work checkout new myrepo feature-new-api
  work checkout new backend bugfix-123

The branch will be created from the base branch's current HEAD.
If the branch already exists remotely, the command will continue and just
create the local worktree.`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeGitRepos,
	Run:               runCheckoutNew,
}

func runCheckoutDirect(cmd *cobra.Command, args []string) {
	// If no args, show help
	if len(args) == 0 {
		cmd.Help()
		return
	}

	// If only 1 arg, it might be a subcommand (handled by cobra) or invalid
	if len(args) == 1 {
		fmt.Fprintf(os.Stderr, "Error: Please provide both repo and branch name\n")
		fmt.Fprintf(os.Stderr, "Usage: work checkout <repo> <branch>\n")
		os.Exit(1)
	}

	repoName := args[0]
	branchName := args[1]

	checkoutRepoBranch(repoName, branchName)
}

// checkoutRepoBranch performs the actual checkout/worktree creation logic.
// This is the shared implementation used by both direct checkout and new branch creation.
func checkoutRepoBranch(repoName, branchName string) {
	// Get git folder from config
	gitFolder := config.GetString("default_git_folder")
	if gitFolder == "" {
		fmt.Fprintf(os.Stderr, "Error: default_git_folder not configured\n")
		fmt.Fprintf(os.Stderr, "Run: work config set default_git_folder ~/git\n")
		os.Exit(1)
	}

	// Expand home directory if needed
	if strings.HasPrefix(gitFolder, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not get home directory: %v\n", err)
			os.Exit(1)
		}
		gitFolder = filepath.Join(homeDir, gitFolder[2:])
	}

	// Build paths
	containerRoot := filepath.Join(gitFolder, repoName)
	gitRoot := filepath.Join(containerRoot, "main")

	// Check if repo exists, if not, try to auto-clone
	if _, err := os.Stat(gitRoot); os.IsNotExist(err) {
		fmt.Printf("Repository '%s' not found locally, attempting to clone...\n", repoName)

		// Get clone URL from GitHub
		cloneURL := getRepoCloneURL(repoName)
		if cloneURL == "" {
			fmt.Fprintf(os.Stderr, "Error: Could not find repository '%s' in configured orgs\n", repoName)
			fmt.Fprintf(os.Stderr, "Run: work checkout root <git-url> to clone manually\n")
			os.Exit(1)
		}

		// Clone the repository
		if err := cloneRepository(cloneURL, repoName, gitFolder); err != nil {
			fmt.Fprintf(os.Stderr, "Error cloning repository: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully cloned '%s'\n", repoName)
	}

	// Change to git root for operations
	if err := os.Chdir(gitRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Error changing to git root: %v\n", err)
		os.Exit(1)
	}

	// Switch to main and pull latest
	if err := runGitCommand("switch", "main"); err != nil {
		fmt.Fprintf(os.Stderr, "Error switching to main: %v\n", err)
		os.Exit(1)
	}

	if err := runGitCommand("pull", "--rebase"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not pull latest changes: %v\n", err)
	}

	// Create worktree path
	worktreePath := filepath.Join(containerRoot, branchName)

	// Check if worktree already exists
	var worktreeExists bool
	if info, err := os.Stat(worktreePath); err == nil && info.IsDir() {
		if isGitWorktree(worktreePath) {
			currentBranch := getCurrentBranch(worktreePath)
			if currentBranch == branchName {
				worktreeExists = true
				fmt.Printf("Switching to existing worktree for branch '%s'\n", branchName)
			} else {
				fmt.Fprintf(os.Stderr, "Error: Folder '%s' exists but is on branch '%s', not '%s'\n",
					worktreePath, currentBranch, branchName)
				fmt.Fprintf(os.Stderr, "Please clean up with: git worktree remove '%s'\n", worktreePath)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: Folder '%s' exists but is not a git worktree\n", worktreePath)
			os.Exit(1)
		}
	} else {
		// Create new worktree
		if err := runGitCommand("worktree", "add", worktreePath, branchName); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created worktree for branch '%s'\n", branchName)
	}

	// Change to the worktree directory
	if err := os.Chdir(worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error changing to worktree: %v\n", err)
		os.Exit(1)
	}

	// If worktree already existed, try to sync it
	if worktreeExists {
		// Try to pull latest changes
		cmd := exec.Command("git", "pull", "--rebase")
		cmd.Dir = worktreePath
		if err := cmd.Run(); err != nil {
			// Silently ignore errors (uncommitted changes, etc.)
			fmt.Printf("Note: Could not sync with remote (you may have uncommitted changes)\n")
		} else {
			fmt.Printf("Synced with remote\n")
		}
	}

	absPath, _ := filepath.Abs(worktreePath)
	fmt.Printf("Path: %s\n", absPath)

	// Run post-checkout actions (custom script or IDE fallback)
	runPostCheckoutActions(worktreePath)
}

func runCheckoutRoot(cmd *cobra.Command, args []string) {
	gitURL := args[0]

	// Extract repo name from URL
	repoName := extractRepoName(gitURL)
	if repoName == "" {
		fmt.Fprintf(os.Stderr, "Error: Could not extract repository name from URL\n")
		os.Exit(1)
	}

	// Get git folder from config
	gitFolder := config.GetString("default_git_folder")
	if gitFolder == "" {
		fmt.Fprintf(os.Stderr, "Error: default_git_folder not configured\n")
		fmt.Fprintf(os.Stderr, "Run: work config set default_git_folder ~/git\n")
		os.Exit(1)
	}

	// Expand home directory if needed
	if strings.HasPrefix(gitFolder, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not get home directory: %v\n", err)
			os.Exit(1)
		}
		gitFolder = filepath.Join(homeDir, gitFolder[2:])
	}

	// Clone the repository
	if err := cloneRepository(gitURL, repoName, gitFolder); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	mainPath := filepath.Join(gitFolder, repoName, "main")
	absPath, _ := filepath.Abs(mainPath)
	fmt.Printf("Repository cloned to %s\n", absPath)
}

func runCheckoutBranch(cmd *cobra.Command, args []string) {
	arg := args[0]
	var branchName string
	var gitRoot, containerRoot string

	// Determine if we're in a git repo or container folder
	if isInsideGitRepo() {
		// We're inside a git repo (branch folder)
		var err error
		gitRoot, err = getGitRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		containerRoot = filepath.Dir(gitRoot)
	} else if _, err := os.Stat("main/.git"); err == nil {
		// We're in container folder with main subfolder
		containerRoot, _ = os.Getwd()
		gitRoot = filepath.Join(containerRoot, "main")
	} else {
		fmt.Fprintf(os.Stderr, "Error: Not in a git repo or container folder with main subfolder\n")
		os.Exit(1)
	}

	// Change to git root for operations
	if err := os.Chdir(gitRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Error changing to git root: %v\n", err)
		os.Exit(1)
	}

	// Switch to main and pull latest
	if err := runGitCommand("switch", "main"); err != nil {
		fmt.Fprintf(os.Stderr, "Error switching to main: %v\n", err)
		os.Exit(1)
	}

	if err := runGitCommand("pull", "--rebase"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not pull latest changes: %v\n", err)
	}

	// Handle GitHub issue URL vs regular branch name
	if isGitHubIssueURL(arg) {
		branchName = handleGitHubIssue(arg)
	} else {
		branchName = arg
	}

	// Create worktree path
	worktreePath := filepath.Join(containerRoot, branchName)

	// Check if worktree already exists
	var worktreeExists bool
	if info, err := os.Stat(worktreePath); err == nil && info.IsDir() {
		if isGitWorktree(worktreePath) {
			currentBranch := getCurrentBranch(worktreePath)
			if currentBranch == branchName {
				worktreeExists = true
				fmt.Printf("Switching to existing worktree for branch '%s'\n", branchName)
			} else {
				fmt.Fprintf(os.Stderr, "Error: Folder '%s' exists but is on branch '%s', not '%s'\n",
					worktreePath, currentBranch, branchName)
				fmt.Fprintf(os.Stderr, "Please clean up with: git worktree remove '%s'\n", worktreePath)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: Folder '%s' exists but is not a git worktree\n", worktreePath)
			os.Exit(1)
		}
	} else {
		// Create new worktree
		if err := runGitCommand("worktree", "add", worktreePath, branchName); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created worktree for branch '%s'\n", branchName)
	}

	// Change to the worktree directory
	if err := os.Chdir(worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error changing to worktree: %v\n", err)
		os.Exit(1)
	}

	// If worktree already existed, try to sync it
	if worktreeExists {
		// Try to pull latest changes
		cmd := exec.Command("git", "pull", "--rebase")
		cmd.Dir = worktreePath
		if err := cmd.Run(); err != nil {
			// Silently ignore errors (uncommitted changes, etc.)
			fmt.Printf("Note: Could not sync with remote (you may have uncommitted changes)\n")
		} else {
			fmt.Printf("Synced with remote\n")
		}
	}

	absPath, _ := filepath.Abs(worktreePath)
	fmt.Printf("Path: %s\n", absPath)

	// Run post-checkout actions (custom script or IDE fallback)
	runPostCheckoutActions(worktreePath)
}

func runCheckoutNew(cmd *cobra.Command, args []string) {
	repoName := args[0]
	branchName := args[1]

	// Step 1: Determine which org the repo belongs to by fetching clone URL
	cloneURL := getRepoCloneURL(repoName)
	if cloneURL == "" {
		fmt.Fprintf(os.Stderr, "Error: Could not find repository '%s' in configured orgs\n", repoName)
		fmt.Fprintf(os.Stderr, "Run: work checkout root <git-url> to clone manually\n")
		os.Exit(1)
	}

	// Step 2: Extract owner from clone URL
	owner := extractOwnerFromCloneURL(cloneURL)
	if owner == "" {
		fmt.Fprintf(os.Stderr, "Error: Could not extract owner from clone URL: %s\n", cloneURL)
		os.Exit(1)
	}

	// Step 3: Get base branch from config
	baseBranch := config.GetString("checkout_base_branch")
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Step 4: Get base branch SHA
	fmt.Printf("Fetching base branch '%s' SHA...\n", baseBranch)
	shaCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s/git/ref/heads/%s", owner, repoName, baseBranch), "--jq", ".object.sha")
	shaOutput, err := shaCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not fetch base branch '%s' SHA: %v\n", baseBranch, err)
		fmt.Fprintf(os.Stderr, "Make sure the base branch exists and you have access to the repository.\n")
		os.Exit(1)
	}
	baseSHA := strings.TrimSpace(string(shaOutput))
	if baseSHA == "" {
		fmt.Fprintf(os.Stderr, "Error: Empty SHA returned for base branch '%s'\n", baseBranch)
		os.Exit(1)
	}

	// Step 5: Create remote branch
	fmt.Printf("Creating remote branch '%s' from '%s' (SHA: %s)...\n", branchName, baseBranch, baseSHA[:7])
	createCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s/git/refs", owner, repoName),
		"--method", "POST",
		"-f", fmt.Sprintf("ref=refs/heads/%s", branchName),
		"-f", fmt.Sprintf("sha=%s", baseSHA))

	createOutput, err := createCmd.CombinedOutput()
	if err != nil {
		// Check if branch already exists (422 status)
		outputStr := string(createOutput)
		if strings.Contains(outputStr, "Reference already exists") || strings.Contains(outputStr, "422") {
			fmt.Printf("Branch '%s' already exists remotely; continuing with checkout\n", branchName)
		} else {
			fmt.Fprintf(os.Stderr, "Error: Could not create remote branch: %v\n", err)
			fmt.Fprintf(os.Stderr, "Output: %s\n", outputStr)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Created remote branch '%s/%s:%s'\n", owner, repoName, branchName)
	}

	// Step 6: Perform local checkout using shared logic
	fmt.Printf("Creating local worktree...\n")
	checkoutRepoBranch(repoName, branchName)
}

// Helper functions

// getRepoCloneURL tries to find the clone URL for a repository from configured orgs
func getRepoCloneURL(repoName string) string {
	preferredOrgs := config.GetStringSlice("preferred_orgs")

	for _, org := range preferredOrgs {
		if org == "" {
			continue
		}

		// Use gh api to get repo info
		// gh api repos/OWNER/REPO --jq '.clone_url'
		cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", org, repoName), "--jq", ".clone_url")
		output, err := cmd.Output()
		if err != nil {
			// Try next org if this fails
			continue
		}

		cloneURL := strings.TrimSpace(string(output))
		if cloneURL != "" {
			return cloneURL
		}
	}

	return ""
}

// cloneRepository clones a git repository into the structured folder layout
func cloneRepository(gitURL, repoName, gitFolder string) error {
	// Ensure git folder exists
	if err := os.MkdirAll(gitFolder, 0755); err != nil {
		return fmt.Errorf("creating git folder: %w", err)
	}

	// Create container folder in git folder
	containerPath := filepath.Join(gitFolder, repoName)
	if err := os.MkdirAll(containerPath, 0755); err != nil {
		return fmt.Errorf("creating folder '%s': %w", repoName, err)
	}

	// Clone into main subfolder
	mainPath := filepath.Join(containerPath, "main")
	cloneCmd := exec.Command("git", "clone", gitURL, mainPath)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr

	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("cloning repository: %w", err)
	}

	return nil
}

// listGitRepos returns a list of git repositories from local folder and GitHub orgs
func listGitRepos() []string {
	// Check if cache is still valid
	if time.Since(repoListCacheTime) < repoListCacheTTL && len(repoListCache) > 0 {
		return repoListCache
	}

	repoMap := make(map[string]bool) // Use map to avoid duplicates
	var repos []string

	// 1. List local repositories from configured git folder
	gitFolder := config.GetString("default_git_folder")
	if gitFolder != "" {
		// Expand home directory if needed
		if strings.HasPrefix(gitFolder, "~/") {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				gitFolder = filepath.Join(homeDir, gitFolder[2:])
			}
		}

		// Check if directory exists
		if info, err := os.Stat(gitFolder); err == nil && info.IsDir() {
			// List all directories in the git folder
			if entries, err := os.ReadDir(gitFolder); err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						// Check if it has a main subfolder (our container structure)
						mainPath := filepath.Join(gitFolder, entry.Name(), "main")
						if _, err := os.Stat(mainPath); err == nil {
							repoName := entry.Name()
							if !repoMap[repoName] {
								repoMap[repoName] = true
								repos = append(repos, repoName)
							}
						}
					}
				}
			}
		}
	}

	// 2. Fetch repositories from preferred GitHub organizations
	preferredOrgs := config.GetStringSlice("preferred_orgs")
	for _, org := range preferredOrgs {
		if org == "" {
			continue
		}

		// Use gh CLI to list repos in the organization
		// gh repo list <org> --limit 1000 --json name -q '.[].name'
		cmd := exec.Command("gh", "repo", "list", org, "--limit", "1000", "--json", "name", "-q", ".[].name")
		output, err := cmd.Output()
		if err != nil {
			// Skip this org if gh command fails (not authenticated, org doesn't exist, etc.)
			continue
		}

		// Parse the output (one repo name per line)
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			repoName := strings.TrimSpace(line)
			if repoName != "" && !repoMap[repoName] {
				repoMap[repoName] = true
				repos = append(repos, repoName)
			}
		}
	}

	// Update cache
	repoListCache = repos
	repoListCacheTime = time.Now()

	return repos
}

// listBranchesForRepo returns a list of branches for a given repository
func listBranchesForRepo(repoName string) []string {
	// Check cache first
	if entry, ok := branchListCache[repoName]; ok {
		if time.Since(entry.fetchedAt) < branchListCacheTTL {
			return entry.branches
		}
	}

	branches := []string{}

	// Try GitHub API first (works even if not cloned locally)
	ghBranches := listBranchesFromGitHub(repoName)
	if len(ghBranches) > 0 {
		// Update cache
		branchListCache[repoName] = branchCacheEntry{
			branches:  ghBranches,
			fetchedAt: time.Now(),
		}
		return ghBranches
	}

	// Fall back to local git repo
	gitFolder := config.GetString("default_git_folder")
	if gitFolder == "" {
		return []string{}
	}

	// Expand home directory if needed
	if strings.HasPrefix(gitFolder, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return []string{}
		}
		gitFolder = filepath.Join(homeDir, gitFolder[2:])
	}

	// Build path to repo's main folder
	gitRoot := filepath.Join(gitFolder, repoName, "main")

	// Check if repo exists
	if _, err := os.Stat(gitRoot); os.IsNotExist(err) {
		return []string{}
	}

	// Prune stale remote-tracking branches first
	pruneCmd := exec.Command("git", "-C", gitRoot, "remote", "prune", "origin")
	pruneCmd.Run() // Ignore errors, continue even if prune fails

	// List remote branches using git branch -r
	cmd := exec.Command("git", "-C", gitRoot, "branch", "-r")
	output, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	// Parse the output
	branchMap := make(map[string]bool) // Use map to avoid duplicates
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		branch := strings.TrimSpace(line)
		// Remove "origin/" prefix
		branch = strings.TrimPrefix(branch, "origin/")

		// Skip empty lines and HEAD references
		if branch != "" && !strings.Contains(branch, "HEAD") && !branchMap[branch] {
			branchMap[branch] = true
			branches = append(branches, branch)
		}
	}

	// Update cache with local git results
	if len(branches) > 0 {
		branchListCache[repoName] = branchCacheEntry{
			branches:  branches,
			fetchedAt: time.Now(),
		}
	}

	return branches
}

// listBranchesFromGitHub fetches branches from GitHub using gh CLI
func listBranchesFromGitHub(repoName string) []string {
	preferredOrgs := config.GetStringSlice("preferred_orgs")

	for _, org := range preferredOrgs {
		if org == "" {
			continue
		}

		// Use gh api to list branches for this repo in the org
		// gh api repos/OWNER/REPO/branches --paginate --jq '.[].name'
		cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s/branches", org, repoName), "--paginate", "--jq", ".[].name")
		output, err := cmd.Output()
		if err != nil {
			// Try next org if this fails
			continue
		}

		// Parse the output (one branch per line)
		branches := []string{}
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			branch := strings.TrimSpace(line)
			if branch != "" {
				branches = append(branches, branch)
			}
		}

		// Return branches from first org that has this repo
		if len(branches) > 0 {
			return branches
		}
	}

	return []string{}
}

// completeGitRepos is a completion function for git repositories and branches
func completeGitRepos(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// First argument: complete repo names
	if len(args) == 0 {
		repos := listGitRepos()
		return repos, cobra.ShellCompDirectiveNoFileComp
	}

	// Second argument: complete branch names for the specified repo
	if len(args) == 1 {
		repoName := args[0]
		branches := listBranchesForRepo(repoName)
		return branches, cobra.ShellCompDirectiveNoFileComp
	}

	// No completion for additional arguments
	return []string{}, cobra.ShellCompDirectiveNoFileComp
}

func extractRepoName(gitURL string) string {
	// Handle various Git URL formats
	// https://github.com/user/repo.git
	// git@github.com:user/repo.git
	// https://github.com/user/repo

	base := filepath.Base(gitURL)
	return strings.TrimSuffix(base, ".git")
}

// extractOwnerFromCloneURL extracts the owner/org from a GitHub clone URL.
// Example: "https://github.com/velvee-ai/repo.git" -> "velvee-ai"
func extractOwnerFromCloneURL(cloneURL string) string {
	// Remove .git suffix
	url := strings.TrimSuffix(cloneURL, ".git")

	// Handle HTTPS format: https://github.com/owner/repo
	if strings.Contains(url, "://") {
		parts := strings.Split(url, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}

	// Handle SSH format: git@github.com:owner/repo
	if strings.Contains(url, ":") {
		parts := strings.Split(url, ":")
		if len(parts) == 2 {
			ownerRepo := parts[1]
			ownerParts := strings.Split(ownerRepo, "/")
			if len(ownerParts) >= 1 {
				return ownerParts[0]
			}
		}
	}

	return ""
}

func isInsideGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
	return err == nil
}

func getGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(output)), nil
}

func isGitHubIssueURL(url string) bool {
	// Match: https://github.com/user/repo/issues/123
	pattern := `^https://github\.com/.+/.+/issues/\d+$`
	matched, _ := regexp.MatchString(pattern, url)
	return matched
}

func handleGitHubIssue(issueURL string) string {
	// Extract issue number
	parts := strings.Split(issueURL, "/")
	issueNumber := parts[len(parts)-1]

	// Check for existing branch related to this issue
	cmd := exec.Command("git", "branch", "-a")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, issueNumber+"-") {
				// Extract branch name
				branch := strings.TrimSpace(line)
				branch = strings.TrimPrefix(branch, "* ")
				branch = strings.TrimPrefix(branch, "remotes/origin/")
				if branch != "" {
					fmt.Printf("Found existing branch: %s\n", branch)

					// Fetch the branch if it doesn't exist locally
					if !branchExistsLocally(branch) {
						exec.Command("git", "fetch", "origin", fmt.Sprintf("%s:%s", branch, branch)).Run()
					}
					return branch
				}
			}
		}
	}

	// Create branch from GitHub issue using gh CLI
	fmt.Printf("Creating branch from GitHub issue #%s...\n", issueNumber)
	createCmd := exec.Command("gh", "issue", "develop", issueURL, "--checkout", "--base", "main")
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating branch from issue: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure 'gh' CLI is installed and authenticated\n")
		os.Exit(1)
	}

	// Assign the issue to yourself
	assignCmd := exec.Command("gh", "issue", "edit", issueURL, "--add-assignee", "@me")
	assignCmd.Run() // Don't fail if this doesn't work

	// Get the current branch name
	branchName := getCurrentBranch(".")

	// Switch back to main for worktree creation
	runGitCommand("switch", "main")

	return branchName
}

func branchExistsLocally(branch string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return cmd.Run() == nil
}

func isGitWorktree(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}

func getCurrentBranch(path string) string {
	cmd := exec.Command("git", "-C", path, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func openInIDE(path string) {
	preferredIDE := config.GetString("preferred_ide")

	// If no IDE is configured or set to "none", skip
	if preferredIDE == "" || preferredIDE == "none" {
		return
	}

	var command string
	switch preferredIDE {
	case "vscode":
		command = "code"
	case "cursor":
		command = "cursor"
	default:
		// Unknown IDE, skip silently
		return
	}

	// Try to open in the configured IDE (optional, don't fail if not available)
	cmd := exec.Command(command, path)
	if err := cmd.Run(); err != nil {
		// Silently ignore errors if IDE is not available
		// User can see the path printed anyway
	}
}

func runPostCheckoutActions(worktreePath string) {
	// Build path to post-checkout script
	scriptPath := filepath.Join(worktreePath, ".work", "post_checkout.sh")

	// Check if the script exists
	info, err := os.Stat(scriptPath)
	if err == nil && !info.IsDir() {
		// Script exists, run it
		fmt.Printf("Running .work/post_checkout.sh…\n")
		cmd := exec.Command("bash", scriptPath)
		cmd.Dir = worktreePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: .work/post_checkout.sh failed: %v\n", err)
			// Do NOT fall back to IDE - repository controls its own flow
		}
		return
	}

	// Script doesn't exist, fall back to IDE behavior
	openInIDE(worktreePath)
}

func runCacheClear(cmd *cobra.Command, args []string) {
	// Clear repo list cache
	repoListCache = []string{}
	repoListCacheTime = time.Time{}

	// Clear branch list cache
	branchListCache = make(map[string]branchCacheEntry)

	fmt.Println("Cache cleared successfully!")
	fmt.Println("Next autocomplete will fetch fresh data from GitHub and local repos.")
}

func init() {
	// Add subcommands to checkout command
	checkoutCmd.AddCommand(checkoutRootCmd)
	checkoutCmd.AddCommand(checkoutBranchCmd)
	checkoutCmd.AddCommand(checkoutNewCmd)
	checkoutCmd.AddCommand(checkoutCacheClearCmd)

	// Register checkout command with root
	rootCmd.AddCommand(checkoutCmd)
}
