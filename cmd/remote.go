package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/config"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Open the repository URL in browser",
	Long: `Open the GitHub repository URL in your default browser.

This command detects the git remote URL from the current repository and opens it in your browser.
By default, it uses the 'origin' remote, but you can configure a different default remote.

Configuration:
  work config set default_remote <remote-name>  # Set default remote (default: origin)

Examples:
  work remote           # Opens the repository URL in browser
  cd ~/git/myrepo/main && work remote`,
	Run: runRemote,
}

func runRemote(cmd *cobra.Command, args []string) {
	// Check if we're in a git repository
	if !isInsideGitRepo() {
		fmt.Fprintf(os.Stderr, "Error: Not in a git repository\n")
		os.Exit(1)
	}

	// Get the configured default remote, or use "origin"
	remoteName := config.GetString("default_remote")
	if remoteName == "" {
		remoteName = "origin"
	}

	// Get the remote URL
	remoteURL, err := getRemoteURL(remoteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Available remotes:\n")
		listRemotes()
		os.Exit(1)
	}

	// Parse the URL to get browser-friendly format
	browserURL, err := parseGitURLToBrowserURL(remoteURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Opening: %s\n", browserURL)

	// Open in browser
	if err := openBrowser(browserURL); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening browser: %v\n", err)
		fmt.Fprintf(os.Stderr, "URL: %s\n", browserURL)
		os.Exit(1)
	}
}

// getRemoteURL gets the URL for the specified remote
func getRemoteURL(remoteName string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", remoteName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not find remote '%s'", remoteName)
	}
	return strings.TrimSpace(string(output)), nil
}

// listRemotes lists all available remotes
func listRemotes() {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

// parseGitURLToBrowserURL converts a git URL to a browser-friendly URL
func parseGitURLToBrowserURL(gitURL string) (string, error) {
	gitURL = strings.TrimSpace(gitURL)

	// Handle SSH URLs: git@github.com:user/repo.git
	sshPattern := regexp.MustCompile(`^git@([^:]+):(.+?)(?:\.git)?$`)
	if matches := sshPattern.FindStringSubmatch(gitURL); matches != nil {
		host := matches[1]
		path := matches[2]
		return fmt.Sprintf("https://%s/%s", host, path), nil
	}

	// Handle HTTPS URLs: https://github.com/user/repo.git or https://github.com/user/repo
	httpsPattern := regexp.MustCompile(`^https://([^/]+)/(.+?)(?:\.git)?$`)
	if matches := httpsPattern.FindStringSubmatch(gitURL); matches != nil {
		host := matches[1]
		path := matches[2]
		return fmt.Sprintf("https://%s/%s", host, path), nil
	}

	// Handle HTTP URLs (convert to HTTPS): http://github.com/user/repo.git
	httpPattern := regexp.MustCompile(`^http://([^/]+)/(.+?)(?:\.git)?$`)
	if matches := httpPattern.FindStringSubmatch(gitURL); matches != nil {
		host := matches[1]
		path := matches[2]
		return fmt.Sprintf("https://%s/%s", host, path), nil
	}

	return "", fmt.Errorf("unsupported git URL format: %s", gitURL)
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Run()
}

func init() {
	// Register remote command with root
	rootCmd.AddCommand(remoteCmd)
}
