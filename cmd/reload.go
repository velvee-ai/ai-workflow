package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/cache"
	"github.com/velvee-ai/ai-workflow/pkg/config"
)

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload repository and branch data from GitHub into cache",
	Long: `Fetch repository and branch information from GitHub and store it in the local cache.

This command:
1. Fetches all repositories from your configured GitHub organizations
2. Fetches branch lists for each repository (in parallel)
3. Stores everything in a local database for fast autocomplete

Run this command:
- After adding new repositories to GitHub
- After creating new branches you want to checkout
- Periodically to keep your cache fresh

Example:
  work reload
  work reload --repos-only  # Only reload repository list`,
	Run: runReload,
}

var (
	reposOnly bool
)

func init() {
	reloadCmd.Flags().BoolVar(&reposOnly, "repos-only", false, "Only reload repository list, skip branches")
	rootCmd.AddCommand(reloadCmd)
}

func runReload(cmd *cobra.Command, args []string) {
	fmt.Println("Reloading cache from GitHub...")

	// Step 1: Fetch repositories
	fmt.Println("\nFetching repositories...")
	repos := fetchRepositoriesFromGitHub()
	if len(repos) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: No repositories found\n")
		fmt.Fprintf(os.Stderr, "Make sure your preferred_orgs are configured: work config set preferred_orgs '[\"org1\",\"org2\"]'\n")
		os.Exit(1)
	}

	// Save repos to cache
	if err := cache.SaveRepoCache(repos); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving repository cache: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Cached %d repositories\n", len(repos))

	// Step 2: Fetch branches (unless --repos-only is set)
	if !reposOnly {
		fmt.Println("\nFetching branches for repositories...")
		fetchBranchesForAllRepos(repos)
	}

	// Show cache stats
	fmt.Println("\nCache updated successfully!")
	showCacheStats()
}

// fetchRepositoriesFromGitHub fetches all repositories from configured GitHub organizations
func fetchRepositoriesFromGitHub() []string {
	preferredOrgs := config.GetStringSlice("preferred_orgs")
	if len(preferredOrgs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No preferred_orgs configured\n")
		return []string{}
	}

	repoMap := make(map[string]bool)
	var repos []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, org := range preferredOrgs {
		if org == "" {
			continue
		}

		wg.Add(1)
		go func(organization string) {
			defer wg.Done()

			fmt.Printf("  Fetching from %s...\n", organization)

			// Use gh CLI to list repos in the organization
			cmd := exec.Command("gh", "repo", "list", organization, "--limit", "1000", "--json", "name", "-q", ".[].name")
			output, err := cmd.Output()
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: Failed to fetch repos from %s: %v\n", organization, err)
				return
			}

			// Parse output
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			count := 0

			mu.Lock()
			for _, line := range lines {
				repoName := strings.TrimSpace(line)
				if repoName != "" && !repoMap[repoName] {
					repoMap[repoName] = true
					repos = append(repos, repoName)
					count++
				}
			}
			mu.Unlock()

			fmt.Printf("  ✓ Found %d repos in %s\n", count, organization)
		}(org)
	}

	wg.Wait()
	return repos
}

// fetchBranchesForAllRepos fetches branches for all repositories in parallel
func fetchBranchesForAllRepos(repos []string) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit to 10 concurrent requests

	successCount := 0
	var mu sync.Mutex

	for _, repo := range repos {
		wg.Add(1)
		go func(repoName string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			branches := fetchBranchesFromGitHub(repoName)
			if len(branches) > 0 {
				if err := cache.SaveBranchCache(repoName, branches); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: Failed to cache branches for %s: %v\n", repoName, err)
					return
				}

				mu.Lock()
				successCount++
				if successCount%10 == 0 {
					fmt.Printf("  Cached branches for %d/%d repositories...\n", successCount, len(repos))
				}
				mu.Unlock()
			}
		}(repo)
	}

	wg.Wait()
	fmt.Printf("✓ Cached branches for %d repositories\n", successCount)
}

// fetchBranchesFromGitHub fetches branches for a specific repository from GitHub
func fetchBranchesFromGitHub(repoName string) []string {
	preferredOrgs := config.GetStringSlice("preferred_orgs")

	// Try each org until we find the repo
	for _, org := range preferredOrgs {
		if org == "" {
			continue
		}

		// Use gh api to list branches sorted by last updated date
		cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s/branches", org, repoName),
			"--paginate",
			"--jq", "sort_by(.commit.commit.committer.date) | reverse | .[].name")
		output, err := cmd.Output()
		if err != nil {
			continue // Try next org
		}

		// Parse output
		var branches []string
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			branch := strings.TrimSpace(line)
			if branch != "" {
				branches = append(branches, branch)
			}
		}

		if len(branches) > 0 {
			return branches
		}
	}

	return []string{}
}

// showCacheStats displays cache statistics
func showCacheStats() {
	stats, err := cache.GetCacheStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not get cache stats: %v\n", err)
		return
	}

	fmt.Println("\nCache Statistics:")
	if count, ok := stats["repo_count"].(int); ok {
		fmt.Printf("  Repositories: %d\n", count)
	}
	if count, ok := stats["cached_repos_with_branches"].(int); ok {
		fmt.Printf("  Repos with branches: %d\n", count)
	}
	if size, ok := stats["db_size_bytes"].(int64); ok {
		fmt.Printf("  Database size: %d KB\n", size/1024)
	}
}
