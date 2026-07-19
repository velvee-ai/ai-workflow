package cmd

import "testing"

func TestIsRetryableGitError(t *testing.T) {
	// Transient network failures are worth another attempt.
	retryable := []string{
		"ssh: connect to host github.com port 22: Operation timed out",
		"fatal: unable to access 'https://github.com/x/y.git/': Could not resolve host: github.com",
		"fatal: the remote end hung up unexpectedly",
		"error: RPC failed; curl 56 Recv failure: Connection reset by peer",
		"fatal: early EOF",
		"ssh: connect to host github.com port 22: Network is unreachable",
	}
	for _, s := range retryable {
		if !isRetryableGitError(s) {
			t.Errorf("expected retryable: %q", s)
		}
	}

	// These will fail identically on every attempt. Retrying them only burns
	// ~30s of backoff before reporting the same error.
	terminal := []string{
		"! [rejected]        main -> main (non-fast-forward)",
		"remote: Permission to x/y.git denied to user.",
		"remote: error: GH006: Protected branch update failed",
		"error: failed to push some refs to 'origin'",
		"remote: Repository not found.",
	}
	for _, s := range terminal {
		if isRetryableGitError(s) {
			t.Errorf("expected terminal, got retryable: %q", s)
		}
	}
}

func TestIsNothingToCommit(t *testing.T) {
	// git reports these on stdout, which is why both streams are captured
	// at the call site.
	yes := []string{
		"On branch feature-x\nnothing to commit, working tree clean",
		"no changes added to commit (use \"git add\")",
		"Untracked files:\n  f.txt\nnothing added to commit but untracked files present",
	}
	for _, s := range yes {
		if !isNothingToCommit(s) {
			t.Errorf("expected nothing-to-commit: %q", s)
		}
	}

	no := []string{
		"error: pathspec 'nope' did not match any file(s) known to git",
		"fatal: not a git repository",
		"",
	}
	for _, s := range no {
		if isNothingToCommit(s) {
			t.Errorf("expected real failure, got nothing-to-commit: %q", s)
		}
	}
}

func TestExtractPRURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "url among chatter",
			in:   "Warning: 3 uncommitted changes\nhttps://github.com/velvee-ai/ai-workflow/pull/42\n",
			want: "https://github.com/velvee-ai/ai-workflow/pull/42",
		},
		{
			name: "url alone",
			in:   "https://github.com/velvee-ai/ai-workflow/pull/7",
			want: "https://github.com/velvee-ai/ai-workflow/pull/7",
		},
		{
			name: "prefers pull url over other links",
			in:   "See https://docs.github.com/pr\nhttps://github.com/o/r/pull/1\n",
			want: "https://github.com/o/r/pull/1",
		},
		{
			name: "falls back to any url if path shape changes",
			in:   "Created: https://github.example.com/o/r/merge_requests/9\n",
			want: "https://github.example.com/o/r/merge_requests/9",
		},
		{
			name: "no url",
			in:   "something went sideways",
			want: "",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractPRURL(tt.in); got != tt.want {
				t.Errorf("extractPRURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
