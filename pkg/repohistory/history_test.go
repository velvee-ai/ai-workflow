package repohistory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepoHistory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "repohistory-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a new history instance
	history := &History{
		entries: make(map[string]*RepoEntry),
		path:    filepath.Join(tempDir, HistoryFileName),
	}

	// Test recording access
	repos := []string{"repo1", "repo2", "repo3", "repo1", "repo2", "repo1"}
	for i, repo := range repos {
		if err := history.RecordAccess(repo); err != nil {
			t.Errorf("Failed to record access for %s: %v", repo, err)
		}
		// Sleep a tiny bit to ensure different timestamps
		if i < len(repos)-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Test GetRecentRepos
	recent := history.GetRecentRepos(10)
	if len(recent) != 3 {
		t.Errorf("Expected 3 unique repos, got %d", len(recent))
	}

	// repo1 was accessed last, so it should be first
	if recent[0] != "repo1" {
		t.Errorf("Expected repo1 to be first, got %s", recent[0])
	}

	// Test limiting to specific number
	recent = history.GetRecentRepos(2)
	if len(recent) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(recent))
	}

	// Test access counts
	if history.entries["repo1"].AccessCount != 3 {
		t.Errorf("Expected repo1 to have 3 accesses, got %d", history.entries["repo1"].AccessCount)
	}
	if history.entries["repo2"].AccessCount != 2 {
		t.Errorf("Expected repo2 to have 2 accesses, got %d", history.entries["repo2"].AccessCount)
	}
	if history.entries["repo3"].AccessCount != 1 {
		t.Errorf("Expected repo3 to have 1 access, got %d", history.entries["repo3"].AccessCount)
	}

	// Test persistence
	if err := history.save(); err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Create a new history instance and load
	history2 := &History{
		entries: make(map[string]*RepoEntry),
		path:    filepath.Join(tempDir, HistoryFileName),
	}
	if err := history2.load(); err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	// Verify data was persisted
	if len(history2.entries) != 3 {
		t.Errorf("Expected 3 repos after load, got %d", len(history2.entries))
	}
	if history2.entries["repo1"].AccessCount != 3 {
		t.Errorf("Expected repo1 to have 3 accesses after load, got %d", history2.entries["repo1"].AccessCount)
	}
}

func TestMaxRecentRepos(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "repohistory-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	history := &History{
		entries: make(map[string]*RepoEntry),
		path:    filepath.Join(tempDir, HistoryFileName),
	}

	// Add more than MaxRecentRepos entries
	for i := 0; i < MaxRecentRepos+10; i++ {
		repoName := string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
		if err := history.RecordAccess(repoName); err != nil {
			t.Errorf("Failed to record access: %v", err)
		}
	}

	// Should only keep MaxRecentRepos
	if len(history.entries) != MaxRecentRepos {
		t.Errorf("Expected %d repos after pruning, got %d", MaxRecentRepos, len(history.entries))
	}
}
