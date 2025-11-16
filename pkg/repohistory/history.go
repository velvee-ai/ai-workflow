package repohistory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// MaxRecentRepos is the maximum number of recently used repos to track
	MaxRecentRepos = 100
	// HistoryFileName is the name of the file storing repo history
	HistoryFileName = "recent_repos.json"
)

// RepoEntry represents a repository access entry
type RepoEntry struct {
	Name         string    `json:"name"`
	LastAccessed time.Time `json:"last_accessed"`
	AccessCount  int       `json:"access_count"`
}

// History manages recently used repositories
type History struct {
	mu      sync.RWMutex
	entries map[string]*RepoEntry
	path    string
}

var (
	instance *History
	once     sync.Once
)

// GetInstance returns the singleton history instance
func GetInstance(configDir string) *History {
	once.Do(func() {
		instance = &History{
			entries: make(map[string]*RepoEntry),
			path:    filepath.Join(configDir, HistoryFileName),
		}
		instance.load()
	})
	return instance
}

// RecordAccess records that a repository was accessed
func (h *History) RecordAccess(repoName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if entry, exists := h.entries[repoName]; exists {
		entry.LastAccessed = time.Now()
		entry.AccessCount++
	} else {
		h.entries[repoName] = &RepoEntry{
			Name:         repoName,
			LastAccessed: time.Now(),
			AccessCount:  1,
		}
	}

	return h.save()
}

// GetRecentRepos returns the N most recently accessed repositories
func (h *History) GetRecentRepos(limit int) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Convert map to slice
	entries := make([]*RepoEntry, 0, len(h.entries))
	for _, entry := range h.entries {
		entries = append(entries, entry)
	}

	// Sort by last accessed time (most recent first)
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].LastAccessed.After(entries[i].LastAccessed) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Take top N
	result := make([]string, 0, limit)
	for i := 0; i < limit && i < len(entries); i++ {
		result = append(result, entries[i].Name)
	}

	return result
}

// load reads the history from disk
func (h *History) load() error {
	data, err := os.ReadFile(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's okay
			return nil
		}
		return err
	}

	var entries []*RepoEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	h.entries = make(map[string]*RepoEntry)
	for _, entry := range entries {
		h.entries[entry.Name] = entry
	}

	return nil
}

// save writes the history to disk
func (h *History) save() error {
	// Convert map to slice
	entries := make([]*RepoEntry, 0, len(h.entries))
	for _, entry := range h.entries {
		entries = append(entries, entry)
	}

	// Keep only the most recent MaxRecentRepos entries
	if len(entries) > MaxRecentRepos {
		// Sort by last accessed time (most recent first)
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].LastAccessed.After(entries[i].LastAccessed) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		entries = entries[:MaxRecentRepos]

		// Rebuild the map with only kept entries
		h.entries = make(map[string]*RepoEntry)
		for _, entry := range entries {
			h.entries[entry.Name] = entry
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(h.path), 0755); err != nil {
		return err
	}

	return os.WriteFile(h.path, data, 0644)
}
