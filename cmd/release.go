package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/services"
)

var releaseCmd = &cobra.Command{
	Use:   "release <repo>",
	Short: "Create and publish a new release",
	Long: `Create a new release by incrementing the version and creating a git tag.

This command will:
1. Switch to the default branch of the repository
2. Pull the latest changes
3. Find the latest release version
4. Increment the version (patch by default)
5. Create and push a new tag

Examples:
  work release myrepo              # Increment patch version (v1.0.0 -> v1.0.1)
  work release myrepo --minor      # Increment minor version (v1.0.0 -> v1.1.0)
  work release myrepo --major      # Increment major version (v1.0.0 -> v2.0.0)
`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeGitRepos,
	Run:               runRelease,
}

var (
	majorRelease bool
	minorRelease bool
)

func init() {
	rootCmd.AddCommand(releaseCmd)
	releaseCmd.Flags().BoolVar(&majorRelease, "major", false, "Increment major version")
	releaseCmd.Flags().BoolVar(&minorRelease, "minor", false, "Increment minor version")
}

func runRelease(cmd *cobra.Command, args []string) {
	repoName := args[0]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get the repository directory
	workDir, err := getRepoWorkDir(repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Ensure we're in a git repository
	gitRunner := services.Get().GitRunner
	if !gitRunner.IsInsideWorkTree(ctx, workDir) {
		fmt.Fprintf(os.Stderr, "Error: %s is not a git repository\n", workDir)
		os.Exit(1)
	}

	fmt.Printf("📦 Preparing release for %s\n\n", repoName)

	// Step 1: Get the default branch
	fmt.Println("1️⃣  Getting default branch...")
	defaultBranch, err := gitRunner.GetDefaultBranch(ctx, workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting default branch: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   Default branch: %s\n\n", defaultBranch)

	// Step 2: Switch to default branch if not already on it
	currentBranch, err := gitRunner.GetCurrentBranch(ctx, workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current branch: %v\n", err)
		os.Exit(1)
	}

	if currentBranch != defaultBranch {
		fmt.Printf("2️⃣  Switching to %s branch...\n", defaultBranch)
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", defaultBranch)
		checkoutCmd.Dir = workDir
		checkoutCmd.Stdout = os.Stdout
		checkoutCmd.Stderr = os.Stderr
		if err := checkoutCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error checking out %s: %v\n", defaultBranch, err)
			os.Exit(1)
		}
		fmt.Println()
	} else {
		fmt.Printf("2️⃣  Already on %s branch\n\n", defaultBranch)
	}

	// Step 3: Pull latest changes
	fmt.Println("3️⃣  Pulling latest changes...")
	pullCmd := exec.CommandContext(ctx, "git", "pull", "--rebase")
	pullCmd.Dir = workDir
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not pull latest changes: %v\n", err)
	}
	fmt.Println()

	// Step 4: Get the latest release
	fmt.Println("4️⃣  Finding latest release...")
	latestVersion, err := getLatestRelease(ctx, workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting latest release: %v\n", err)
		os.Exit(1)
	}

	if latestVersion == "" {
		latestVersion = "v0.0.0"
		fmt.Println("   No previous releases found, starting from v0.0.0")
	} else {
		fmt.Printf("   Latest release: %s\n", latestVersion)
	}
	fmt.Println()

	// Step 5: Increment version
	fmt.Println("5️⃣  Incrementing version...")
	newVersion, err := incrementVersion(latestVersion, majorRelease, minorRelease)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error incrementing version: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   New version: %s\n\n", newVersion)

	// Step 6: Create and push tag
	fmt.Printf("6️⃣  Creating and pushing tag %s...\n", newVersion)

	// Create the tag
	tagCmd := exec.CommandContext(ctx, "git", "tag", "-a", newVersion, "-m", fmt.Sprintf("Release %s", newVersion))
	tagCmd.Dir = workDir
	tagCmd.Stdout = os.Stdout
	tagCmd.Stderr = os.Stderr
	if err := tagCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tag: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   ✓ Tag %s created\n", newVersion)

	// Push the tag
	pushCmd := exec.CommandContext(ctx, "git", "push", "origin", newVersion)
	pushCmd.Dir = workDir
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr
	if err := pushCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error pushing tag: %v\n", err)
		fmt.Fprintf(os.Stderr, "Tag created locally but not pushed. You can push it manually with:\n")
		fmt.Fprintf(os.Stderr, "  git push origin %s\n", newVersion)
		os.Exit(1)
	}
	fmt.Printf("   ✓ Tag %s pushed to remote\n\n", newVersion)

	fmt.Printf("✅ Release %s created successfully!\n", newVersion)
	fmt.Println("The release workflow should now be triggered automatically.")
}

// getRepoWorkDir returns the working directory for a repository
func getRepoWorkDir(repoName string) (string, error) {
	// First, try to find the main worktree directory
	mainDir := fmt.Sprintf("%s/main", repoName)
	if _, err := os.Stat(mainDir); err == nil {
		return mainDir, nil
	}

	// Try the repo name directly (in case it's a simple clone)
	if _, err := os.Stat(repoName); err == nil {
		return repoName, nil
	}

	// Try current directory if repo name matches
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if current directory contains the repo name
	if strings.Contains(cwd, repoName) {
		return cwd, nil
	}

	return "", fmt.Errorf("repository directory not found for %s (tried: %s/main, %s, current directory)",
		repoName, repoName, repoName)
}

// getLatestRelease queries GitHub for the latest release
func getLatestRelease(ctx context.Context, workDir string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "latestRelease")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		// If gh command fails, it might mean no releases exist
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "no releases") || strings.Contains(stderr, "not found") {
				return "", nil
			}
		}
		return "", fmt.Errorf("failed to get latest release: %w", err)
	}

	var result struct {
		LatestRelease *struct {
			TagName string `json:"tagName"`
		} `json:"latestRelease"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse release data: %w", err)
	}

	if result.LatestRelease == nil {
		return "", nil
	}

	return result.LatestRelease.TagName, nil
}

// incrementVersion increments a semantic version string
func incrementVersion(version string, major, minor bool) (string, error) {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Parse version using regex to handle semver
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(version)

	if len(matches) != 4 {
		return "", fmt.Errorf("invalid version format: %s (expected format: v1.2.3)", version)
	}

	majorVer, _ := strconv.Atoi(matches[1])
	minorVer, _ := strconv.Atoi(matches[2])
	patchVer, _ := strconv.Atoi(matches[3])

	if major {
		majorVer++
		minorVer = 0
		patchVer = 0
	} else if minor {
		minorVer++
		patchVer = 0
	} else {
		// Default to patch increment
		patchVer++
	}

	return fmt.Sprintf("v%d.%d.%d", majorVer, minorVer, patchVer), nil
}
