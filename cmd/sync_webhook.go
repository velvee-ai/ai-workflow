package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/velvee-ai/ai-workflow/pkg/config"
)

var syncWebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "GitHub webhook-driven sync via smee.io",
	Long: `Receive GitHub push webhooks through a smee.io channel and sync the
matching local repo's default branch automatically.

Subcommands:
  work sync webhook setup   - Create a smee channel and save the URL
  work sync webhook listen  - Run the listener daemon`,
}

var syncWebhookSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Create a smee.io channel for receiving webhooks",
	Long: `Create a new smee.io channel and save it to the config file.

After running this, add the printed URL as a webhook on each GitHub repo
you want to auto-sync:
  Settings > Webhooks > Add webhook
    Payload URL:  <printed URL>
    Content type: application/json
    Events:       Just the push event

Then run 'work sync webhook listen' to start receiving and syncing.

Security note: the smee channel URL is the only secret — anyone who
knows it can deliver events. Do not share it.`,
	Run: runSyncWebhookSetup,
}

var syncWebhookListenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen for webhooks and sync matching repos",
	Long: `Long-running listener: connects to the configured smee channel,
receives GitHub push events, and syncs the matching local repo's default
branch by running the same logic as 'work sync <repo>'.

Only push events to a repo's default branch trigger a sync. Pushes to
feature branches, pull_request events, pings, etc. are ignored.

Repos are matched by name: a push to owner/foo syncs
<default_git_folder>/foo/main if it exists locally.

Reconnects on disconnect with exponential backoff. Ctrl+C to stop.`,
	Run: runSyncWebhookListen,
}

func runSyncWebhookSetup(cmd *cobra.Command, args []string) {
	existing := config.GetString("smee_channel_url")
	if existing != "" {
		fmt.Printf("Existing smee channel: %s\n", existing)
		fmt.Print("Replace with a new channel? [y/N] ")
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Keeping existing channel.")
			return
		}
	}

	channelURL, err := createSmeeChannel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := config.Set("smee_channel_url", channelURL); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Smee channel created: %s\n", channelURL)
	fmt.Println()
	fmt.Println("Add this as a webhook on each repo you want auto-synced:")
	fmt.Println("  1. GitHub repo > Settings > Webhooks > Add webhook")
	fmt.Printf("  2. Payload URL:  %s\n", channelURL)
	fmt.Println("  3. Content type: application/json")
	fmt.Println("  4. Events:       Just the push event")
	fmt.Println()
	fmt.Println("Then run:")
	fmt.Println("  work sync webhook listen")
}

func runSyncWebhookListen(cmd *cobra.Command, args []string) {
	channelURL := config.GetString("smee_channel_url")
	if channelURL == "" {
		fmt.Fprintln(os.Stderr, "Error: smee_channel_url is not configured")
		fmt.Fprintln(os.Stderr, "Run: work sync webhook setup")
		os.Exit(1)
	}

	gitFolder := resolvedGitFolder()
	if gitFolder == "" {
		fmt.Fprintln(os.Stderr, "Error: default_git_folder is not configured or does not exist")
		fmt.Fprintln(os.Stderr, "Run: work config set default_git_folder <path>")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down.")
		cancel()
	}()

	fmt.Printf("Listening on %s\n", channelURL)
	fmt.Printf("Git folder:  %s\n", gitFolder)
	fmt.Println("Ctrl+C to stop.")

	backoff := time.Second
	const maxBackoff = 30 * time.Second
	var lastCatchup time.Time

	for {
		// Smee has no persistence: any webhook that arrived while we were
		// offline (reboot, sleep, network loss) is gone. Run a catch-up
		// pull before each connection attempt so local repos don't drift.
		// Rate-limit to at most once per minute to avoid hammering during
		// a reconnect storm.
		if time.Since(lastCatchup) > time.Minute {
			runCatchup(ctx, gitFolder)
			lastCatchup = time.Now()
		}

		err := runSmeeLoop(ctx, channelURL, gitFolder)
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "Connection lost: %v (retry in %s)\n", err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// runCatchup pulls --rebase every discovered repo once. Used on daemon
// startup and after reconnects to cover events missed while offline.
func runCatchup(ctx context.Context, gitFolder string) {
	entries, err := os.ReadDir(gitFolder)
	if err != nil {
		return
	}
	catchupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repoPath := filepath.Join(gitFolder, entry.Name())
		if _, err := os.Stat(filepath.Join(repoPath, "main", ".git")); err != nil {
			continue
		}
		result := syncRepository(catchupCtx, repoPath)
		if result.Success && result.Message != "Already up to date" {
			fmt.Printf("↻ catchup %s (%s): %s\n", result.RepoName, result.DefaultBranch, result.Message)
		} else if !result.Success {
			fmt.Fprintf(os.Stderr, "✗ catchup %s: %s\n", result.RepoName, result.Error.Error())
		}
	}
}

// runSmeeLoop opens one SSE connection to the smee channel and dispatches
// events until the connection closes or the context is cancelled.
func runSmeeLoop(ctx context.Context, channelURL, gitFolder string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "work-sync-webhook/1")

	// No client-level timeout — this connection is intentionally long-lived.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("smee returned HTTP %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	var dataBuf strings.Builder
	var eventName string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if dataBuf.Len() > 0 {
				handleSmeeEvent(ctx, eventName, dataBuf.String(), gitFolder)
			}
			dataBuf.Reset()
			eventName = ""
			continue
		}

		// SSE comment / keepalive
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")

		switch field {
		case "event":
			eventName = value
		case "data":
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(value)
		}
	}
}

// handleSmeeEvent parses one smee envelope and, if it's a push to a repo's
// default branch that exists locally, syncs it.
func handleSmeeEvent(ctx context.Context, eventName, data, gitFolder string) {
	// smee-specific events we don't care about (ready, ping, etc.)
	if eventName != "" && eventName != "message" {
		return
	}

	// Smee wraps the webhook in a JSON object with headers at the top level
	// (lowercased) and the parsed body under "body".
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse smee envelope: %v\n", err)
		return
	}

	var ghEvent string
	if raw, ok := env["x-github-event"]; ok {
		_ = json.Unmarshal(raw, &ghEvent)
	}
	if ghEvent != "push" {
		return
	}

	rawBody, ok := env["body"]
	if !ok {
		return
	}

	var body struct {
		Ref        string `json:"ref"`
		Deleted    bool   `json:"deleted"`
		Repository struct {
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse push body: %v\n", err)
		return
	}

	if body.Deleted {
		return
	}
	if body.Repository.Name == "" || body.Repository.DefaultBranch == "" {
		return
	}
	if body.Ref != "refs/heads/"+body.Repository.DefaultBranch {
		// Push to a non-default branch — ignore.
		return
	}

	repoPath := filepath.Join(gitFolder, body.Repository.Name)
	if _, err := os.Stat(filepath.Join(repoPath, "main", ".git")); err != nil {
		fmt.Printf("↷ %s: no local repo at %s, skipping\n", body.Repository.FullName, repoPath)
		return
	}

	fmt.Printf("↻ %s: push to %s\n", body.Repository.FullName, body.Repository.DefaultBranch)
	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result := syncRepository(syncCtx, repoPath)
	if result.Success {
		fmt.Printf("✓ %s (%s): %s\n", result.RepoName, result.DefaultBranch, result.Message)
	} else {
		fmt.Fprintf(os.Stderr, "✗ %s: %s\n", result.RepoName, result.Error.Error())
	}
}

// resolvedGitFolder returns the expanded default_git_folder if it exists.
func resolvedGitFolder() string {
	gitFolder := config.GetString("default_git_folder")
	if gitFolder == "" {
		return ""
	}
	if strings.HasPrefix(gitFolder, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		gitFolder = filepath.Join(home, gitFolder[2:])
	}
	if _, err := os.Stat(gitFolder); err != nil {
		return ""
	}
	return gitFolder
}

func init() {
	syncWebhookCmd.AddCommand(syncWebhookSetupCmd)
	syncWebhookCmd.AddCommand(syncWebhookListenCmd)
	syncCmd.AddCommand(syncWebhookCmd)
}
