package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	repoBucket    = []byte("repos")
	branchBucket  = []byte("branches")
	metadataBucket = []byte("metadata")
)

// RepoCache stores repository names with metadata
type RepoCache struct {
	Repos     []string  `json:"repos"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BranchCache stores branches for a specific repository with metadata
type BranchCache struct {
	Branches  []string  `json:"branches"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetCacheDir returns the cache directory path
func GetCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".work", "cache"), nil
}

// ensureCacheDir ensures the cache directory exists
func ensureCacheDir() error {
	cacheDir, err := GetCacheDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(cacheDir, 0755)
}

// getCacheDBPath returns the path to the bbolt database file
func getCacheDBPath() (string, error) {
	cacheDir, err := GetCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "work.db"), nil
}

// openDB opens the bbolt database
func openDB() (*bolt.DB, error) {
	if err := ensureCacheDir(); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath, err := getCacheDBPath()
	if err != nil {
		return nil, err
	}

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open cache database: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(repoBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(branchBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(metadataBucket); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize buckets: %w", err)
	}

	return db, nil
}

// SaveRepoCache saves the repository list to the cache database
func SaveRepoCache(repos []string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	cache := RepoCache{
		Repos:     repos,
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal repo cache: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(repoBucket)
		return b.Put([]byte("all"), data)
	})
	if err != nil {
		return fmt.Errorf("failed to save repo cache: %w", err)
	}

	return nil
}

// LoadRepoCache loads the repository list from the cache database
func LoadRepoCache() ([]string, error) {
	db, err := openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var cache RepoCache
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(repoBucket)
		data := b.Get([]byte("all"))
		if data == nil {
			return nil // No cache exists yet
		}
		return json.Unmarshal(data, &cache)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load repo cache: %w", err)
	}

	return cache.Repos, nil
}

// SaveBranchCache saves the branch list for a repository to the cache database
func SaveBranchCache(repoName string, branches []string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	cache := BranchCache{
		Branches:  branches,
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("failed to marshal branch cache: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(branchBucket)
		return b.Put([]byte(repoName), data)
	})
	if err != nil {
		return fmt.Errorf("failed to save branch cache: %w", err)
	}

	return nil
}

// LoadBranchCache loads the branch list for a repository from the cache database
func LoadBranchCache(repoName string) ([]string, error) {
	db, err := openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var cache BranchCache
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(branchBucket)
		data := b.Get([]byte(repoName))
		if data == nil {
			return nil // No cache exists for this repo yet
		}
		return json.Unmarshal(data, &cache)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load branch cache: %w", err)
	}

	return cache.Branches, nil
}

// ClearCache removes all cached data by deleting the database file
func ClearCache() error {
	dbPath, err := getCacheDBPath()
	if err != nil {
		return err
	}

	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	return nil
}

// GetCacheStats returns statistics about the cache
func GetCacheStats() (map[string]interface{}, error) {
	db, err := openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	stats := make(map[string]interface{})

	err = db.View(func(tx *bolt.Tx) error {
		// Count repos
		repoBkt := tx.Bucket(repoBucket)
		var repoCache RepoCache
		data := repoBkt.Get([]byte("all"))
		if data != nil {
			if err := json.Unmarshal(data, &repoCache); err == nil {
				stats["repo_count"] = len(repoCache.Repos)
				stats["repos_updated_at"] = repoCache.UpdatedAt
			}
		}

		// Count branches
		branchBkt := tx.Bucket(branchBucket)
		branchCount := 0
		branchBkt.ForEach(func(k, v []byte) error {
			branchCount++
			return nil
		})
		stats["cached_repos_with_branches"] = branchCount

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Get DB file size
	dbPath, err := getCacheDBPath()
	if err == nil {
		if info, err := os.Stat(dbPath); err == nil {
			stats["db_size_bytes"] = info.Size()
		}
	}

	return stats, nil
}
