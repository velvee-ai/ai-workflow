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
	Short: "Reload repository list from GitHub into cache",
	Long: `Fetch repository names from GitHub and store them in the local cache.

This command:
1. Fetches all repositories from your configured GitHub organizations
2. Stores repository names in a local database for fast autocomplete

Branches are fetched on-demand during tab completion from GitHub API.

Run this command:
- After adding new repositories to GitHub
- Periodically to keep your repository list fresh

Example:
  work reload`,
	Run: runReload,
}

func init() {
	rootCmd.AddCommand(reloadCmd)
}

func runReload(cmd *cobra.Command, args []string) {
	fmt.Println("Reloading repository cache from GitHub...")

	// Fetch repositories
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

	// Show cache stats
	fmt.Println("\nCache updated successfully!")
	fmt.Printf("\nNote: Branches are fetched on-demand from GitHub during tab completion.\n")
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

