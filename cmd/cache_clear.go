package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var cacheClearCmd = &cobra.Command{
	Use:   "cache-clear",
	Short: "Clear autocomplete cache and run commands with fresh data",
	Long: `Clear the cached repository and branch lists used for autocomplete.

This command can be used as a prefix to other commands to ensure fresh data is fetched.

Examples:
  work cache-clear checkout <repo> <branch>   # Clear cache then checkout`,
}

var cacheClearCheckoutCmd = &cobra.Command{
	Use:   "checkout [repo] [branch]",
	Short: "Clear cache and then checkout with fresh branch data",
	Long: `Clear the autocomplete cache and then perform a checkout operation.

This is useful when you want to immediately see new repositories or branches
without waiting for the cache to expire (default: 5 minutes).

Example:
  work cache-clear checkout my-repo feature-branch`,
	Args:              cobra.MaximumNArgs(2),
	ValidArgsFunction: completeCacheClearCheckout,
	Run:               runCacheClearCheckout,
}

func runCacheClearCheckout(cmd *cobra.Command, args []string) {
	// First clear the cache
	clearCache()

	// Then run the normal checkout logic
	runCheckoutDirect(cmd, args)
}

func clearCache() {
	// Clear repo list cache
	repoListCache = []string{}
	repoListCacheTime = time.Time{}

	// Clear branch list cache
	branchListCache = make(map[string]branchCacheEntry)

	fmt.Println("Cache cleared - fetching fresh data...")
}

// completeCacheClearCheckout provides autocomplete with fresh data (cache cleared)
func completeCacheClearCheckout(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Clear cache before autocomplete to ensure fresh data
	repoListCache = []string{}
	repoListCacheTime = time.Time{}
	branchListCache = make(map[string]branchCacheEntry)

	// Use the same completion logic as regular checkout
	return completeGitRepos(cmd, args, toComplete)
}

func init() {
	// Add checkout subcommand to cache-clear
	cacheClearCmd.AddCommand(cacheClearCheckoutCmd)

	// Register cache-clear command with root
	rootCmd.AddCommand(cacheClearCmd)
}
