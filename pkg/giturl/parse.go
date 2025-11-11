package giturl

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// SSH pattern: git@github.com:user/repo.git
	sshPattern = regexp.MustCompile(`^(?:[\w\.\-]+@)?([^:]+):(.+?)(?:\.git)?$`)

	// HTTP(S) pattern: https://github.com/user/repo.git
	httpsPattern = regexp.MustCompile(`^https?://([^/]+)/(.+?)(?:\.git)?$`)

	// SSH URL pattern: ssh://git@github.com/user/repo.git
	sshURLPattern = regexp.MustCompile(`^ssh://(?:[\w\.\-]+@)?([^/]+)/(.+?)(?:\.git)?$`)
)

// ParsedURL represents a parsed git URL.
type ParsedURL struct {
	Host string
	Path string
	Org  string
	Repo string
}

// Parse parses a git URL and extracts components.
// Supports: ssh (git@...), https, http, and ssh:// formats.
func Parse(gitURL string) (*ParsedURL, error) {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return nil, fmt.Errorf("empty git URL")
	}

	// Try SSH URL pattern first (ssh://...)
	if matches := sshURLPattern.FindStringSubmatch(gitURL); matches != nil {
		return parseMatches(matches[1], matches[2])
	}

	// Try HTTPS pattern
	if matches := httpsPattern.FindStringSubmatch(gitURL); matches != nil {
		return parseMatches(matches[1], matches[2])
	}

	// Try SSH pattern (git@...)
	if matches := sshPattern.FindStringSubmatch(gitURL); matches != nil {
		return parseMatches(matches[1], matches[2])
	}

	return nil, fmt.Errorf("unsupported git URL format: %s", gitURL)
}

func parseMatches(host, path string) (*ParsedURL, error) {
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid path format: %s", path)
	}

	return &ParsedURL{
		Host: host,
		Path: path,
		Org:  parts[0],
		Repo: parts[1],
	}, nil
}

// ExtractRepoName extracts just the repository name from a git URL.
// Example: "https://github.com/user/repo.git" -> "repo"
func ExtractRepoName(gitURL string) string {
	base := filepath.Base(gitURL)
	return strings.TrimSuffix(base, ".git")
}

// IsSSH returns true if the URL is an SSH URL.
func IsSSH(gitURL string) bool {
	return strings.HasPrefix(gitURL, "git@") || strings.HasPrefix(gitURL, "ssh://")
}

// IsHTTPS returns true if the URL is an HTTPS URL.
func IsHTTPS(gitURL string) bool {
	return strings.HasPrefix(gitURL, "https://")
}

// ToSSH converts an HTTPS URL to SSH format.
// Example: "https://github.com/user/repo.git" -> "git@github.com:user/repo.git"
func ToSSH(gitURL string) (string, error) {
	parsed, err := Parse(gitURL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("git@%s:%s.git", parsed.Host, parsed.Path), nil
}

// ToHTTPS converts an SSH URL to HTTPS format.
// Example: "git@github.com:user/repo.git" -> "https://github.com/user/repo.git"
func ToHTTPS(gitURL string) (string, error) {
	parsed, err := Parse(gitURL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s/%s.git", parsed.Host, parsed.Path), nil
}
