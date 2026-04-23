package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/velvee-ai/ai-workflow/pkg/config"
)

// enableAutoSync performs the full idempotent setup for a single repo:
// resolves owner/repo from the local git remote, ensures a smee channel
// URL is saved, installs the GitHub webhook if not already present, and
// ensures the launchd listener daemon is loaded.
//
// Each step prints a line only when it actually performs work. In steady
// state (webhook installed, daemon running) this function is silent.
func enableAutoSync(ctx context.Context, gitFolder, repoName string) (localPath string, err error) {
	localPath = filepath.Join(gitFolder, repoName)
	mainPath := filepath.Join(localPath, "main")
	if _, err := os.Stat(filepath.Join(mainPath, ".git")); err != nil {
		return "", fmt.Errorf("no git repo at %s", mainPath)
	}

	slug, err := resolveRepoSlug(ctx, mainPath)
	if err != nil {
		return "", err
	}

	smeeURL, createdChannel, err := ensureSmeeChannel()
	if err != nil {
		return "", err
	}
	if createdChannel {
		fmt.Printf("✓ Created smee channel: %s\n", smeeURL)
	}

	installed, err := ensureGitHubHook(ctx, slug, smeeURL)
	if err != nil {
		return "", err
	}
	if installed {
		fmt.Printf("✓ Installed webhook on %s\n", slug)
	}

	wasLoaded := launchdLoaded()
	if err := launchdEnsure(); err != nil {
		return "", fmt.Errorf("starting sync daemon: %w", err)
	}
	if !wasLoaded {
		fmt.Printf("✓ Started sync daemon (launchd agent %s)\n", launchdLabel)
	}

	return localPath, nil
}

// disableAutoSync removes the webhook (pointing to our smee channel) from
// the GitHub repo. It does not stop the launchd daemon — other repos may
// still be using it.
func disableAutoSync(ctx context.Context, gitFolder, repoName string) error {
	mainPath := filepath.Join(gitFolder, repoName, "main")
	if _, err := os.Stat(filepath.Join(mainPath, ".git")); err != nil {
		return fmt.Errorf("no git repo at %s", mainPath)
	}

	slug, err := resolveRepoSlug(ctx, mainPath)
	if err != nil {
		return err
	}

	smeeURL := config.GetString("smee_channel_url")
	if smeeURL == "" {
		fmt.Println("No smee channel configured; nothing to remove.")
		return nil
	}

	hooks, err := listGitHubHooks(ctx, slug)
	if err != nil {
		return err
	}

	removed := 0
	for _, h := range hooks {
		if h.Config.URL == smeeURL {
			if err := deleteGitHubHook(ctx, slug, h.ID); err != nil {
				return err
			}
			removed++
		}
	}
	if removed == 0 {
		fmt.Printf("No matching webhook on %s.\n", slug)
	} else {
		fmt.Printf("✓ Removed %d webhook(s) from %s\n", removed, slug)
	}
	return nil
}

// resolveRepoSlug returns "owner/repo" from the git remote of the repo at
// path. Uses `gh repo view` which reads the origin remote.
func resolveRepoSlug(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	cmd.Dir = path
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh repo view failed in %s: %w (%s)", path, err, strings.TrimSpace(stderr.String()))
	}
	slug := strings.TrimSpace(string(out))
	if slug == "" {
		return "", fmt.Errorf("gh repo view returned empty slug for %s", path)
	}
	return slug, nil
}

// ensureSmeeChannel returns the configured channel URL, creating and
// persisting a new one if none is set. `created` is true iff this call
// generated a new channel.
func ensureSmeeChannel() (url string, created bool, err error) {
	url = config.GetString("smee_channel_url")
	if url != "" {
		return url, false, nil
	}
	url, err = createSmeeChannel()
	if err != nil {
		return "", false, err
	}
	if err := config.Set("smee_channel_url", url); err != nil {
		return "", false, fmt.Errorf("saving smee_channel_url: %w", err)
	}
	return url, true, nil
}

// createSmeeChannel hits smee.io/new and returns the redirect Location.
func createSmeeChannel() (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get("https://smee.io/new")
	if err != nil {
		return "", fmt.Errorf("contacting smee.io: %w", err)
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("smee.io returned HTTP %d with no Location header", resp.StatusCode)
	}
	return loc, nil
}

type ghHook struct {
	ID     int `json:"id"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
}

func listGitHubHooks(ctx context.Context, slug string) ([]ghHook, error) {
	cmd := exec.CommandContext(ctx, "gh", "api", "--paginate", "/repos/"+slug+"/hooks")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing webhooks on %s: %w (%s)", slug, err, strings.TrimSpace(stderr.String()))
	}
	var hooks []ghHook
	if err := json.Unmarshal(out, &hooks); err != nil {
		return nil, fmt.Errorf("parsing webhook list for %s: %w", slug, err)
	}
	return hooks, nil
}

// ensureGitHubHook creates a push webhook pointing to smeeURL if one does
// not already exist. Returns true iff it created a new hook.
func ensureGitHubHook(ctx context.Context, slug, smeeURL string) (bool, error) {
	hooks, err := listGitHubHooks(ctx, slug)
	if err != nil {
		return false, err
	}
	for _, h := range hooks {
		if h.Config.URL == smeeURL {
			return false, nil
		}
	}
	if err := createGitHubHook(ctx, slug, smeeURL); err != nil {
		return false, err
	}
	return true, nil
}

func createGitHubHook(ctx context.Context, slug, smeeURL string) error {
	payload := map[string]interface{}{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]string{
			"url":          smeeURL,
			"content_type": "json",
			"insecure_ssl": "0",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "POST", "/repos/"+slug+"/hooks", "--input", "-")
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "admin:repo_hook") || strings.Contains(msg, "Resource not accessible") {
			return fmt.Errorf("creating webhook on %s: %w\n%s\nRun 'work doctor' to check your gh token scopes", slug, err, msg)
		}
		return fmt.Errorf("creating webhook on %s: %w (%s)", slug, err, msg)
	}
	return nil
}

func deleteGitHubHook(ctx context.Context, slug string, id int) error {
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "DELETE", fmt.Sprintf("/repos/%s/hooks/%d", slug, id))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deleting webhook %d on %s: %w (%s)", id, slug, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
