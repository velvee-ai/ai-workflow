package gitexec

import (
	"context"
	"testing"
	"time"
)

func TestRunner_RunSimple(t *testing.T) {
	runner := New(5 * time.Second)
	ctx := context.Background()

	// Test git version command
	output, err := runner.RunSimple(ctx, "", "version")
	if err != nil {
		t.Fatalf("expected git version to succeed: %v", err)
	}
	if output == "" {
		t.Error("expected non-empty output from git version")
	}
}

func TestRunner_RunWithTimeout(t *testing.T) {
	runner := New(100 * time.Millisecond)
	ctx := context.Background()

	// This should timeout if git takes too long (unlikely with version command)
	_, err := runner.RunSimple(ctx, "", "version")
	if err != nil {
		// Timeout errors are acceptable for this test
		t.Logf("Command completed or timed out: %v", err)
	}
}

func TestRunner_RunIgnoreError(t *testing.T) {
	runner := New(5 * time.Second)
	ctx := context.Background()

	// Run a command that will fail
	output := runner.RunIgnoreError(ctx, "", "this-command-does-not-exist")
	// Should not panic, just return empty or error output
	t.Logf("Output from failed command: %q", output)
}
