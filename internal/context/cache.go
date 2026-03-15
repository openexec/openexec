package context

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Cache provides durable, content-addressed storage for context bundles.
// It enables deterministic replay by caching gathered context by hash.
type Cache struct {
	basePath string
}

// CacheEntry holds metadata about a cached context bundle.
type CacheEntry struct {
	Hash      string    `json:"hash"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at"`
}

// NewCache creates a new context cache rooted at basePath.
// The basePath should typically be .openexec/cache/context.
func NewCache(basePath string) (*Cache, error) {
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	return &Cache{basePath: basePath}, nil
}

// Get retrieves a cached context bundle by its hash.
// Returns nil, nil if the entry does not exist.
func (c *Cache) Get(ctx context.Context, hash string) ([]byte, error) {
	if !isValidHash(hash) {
		return nil, fmt.Errorf("invalid hash format: %s", hash)
	}

	path := c.pathForHash(hash)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read cache entry: %w", err)
	}

	// Update access time (best effort)
	_ = c.updateAccessTime(hash)

	return data, nil
}

// Put stores a context bundle and returns its content hash.
// The hash is computed from the bundle content for deduplication.
func (c *Cache) Put(ctx context.Context, bundle []byte) (string, error) {
	hash := computeHash(bundle)
	path := c.pathForHash(hash)

	// Check if already exists (deduplication)
	if _, err := os.Stat(path); err == nil {
		// Update access time and return existing hash
		_ = c.updateAccessTime(hash)
		return hash, nil
	}

	// Write the bundle
	if err := os.WriteFile(path, bundle, 0600); err != nil {
		return "", fmt.Errorf("failed to write cache entry: %w", err)
	}

	// Write metadata
	meta := CacheEntry{
		Hash:      hash,
		Size:      int64(len(bundle)),
		CreatedAt: time.Now(),
		AccessedAt: time.Now(),
	}
	if err := c.writeMeta(hash, &meta); err != nil {
		// Non-fatal - cache still works without metadata
		_ = err
	}

	return hash, nil
}

// Prune removes old or excess entries to keep cache within limits.
// Entries are pruned by LRU (least recently accessed) order.
func (c *Cache) Prune(ctx context.Context, maxAge time.Duration, maxSizeBytes int64) error {
	entries, err := c.listEntries()
	if err != nil {
		return err
	}

	now := time.Now()
	var totalSize int64 = 0
	var toDelete []string

	// Sort by access time (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].AccessedAt.Before(entries[j].AccessedAt)
	})

	// First pass: mark entries older than maxAge
	for _, entry := range entries {
		if maxAge > 0 && now.Sub(entry.AccessedAt) > maxAge {
			toDelete = append(toDelete, entry.Hash)
		} else {
			totalSize += entry.Size
		}
	}

	// Second pass: if still over size limit, delete oldest entries
	if maxSizeBytes > 0 && totalSize > maxSizeBytes {
		for _, entry := range entries {
			// Skip already marked for deletion
			found := false
			for _, h := range toDelete {
				if h == entry.Hash {
					found = true
					break
				}
			}
			if found {
				continue
			}

			if totalSize <= maxSizeBytes {
				break
			}
			toDelete = append(toDelete, entry.Hash)
			totalSize -= entry.Size
		}
	}

	// Delete marked entries
	for _, hash := range toDelete {
		if err := c.delete(hash); err != nil {
			// Log but continue
			_ = err
		}
	}

	return nil
}

// Stats returns cache statistics.
func (c *Cache) Stats(ctx context.Context) (count int, totalSize int64, err error) {
	entries, err := c.listEntries()
	if err != nil {
		return 0, 0, err
	}

	for _, entry := range entries {
		count++
		totalSize += entry.Size
	}

	return count, totalSize, nil
}

// pathForHash returns the file path for a given content hash.
// Uses a two-level directory structure to avoid too many files in one dir.
func (c *Cache) pathForHash(hash string) string {
	if len(hash) >= 4 {
		return filepath.Join(c.basePath, hash[:2], hash[2:4], hash)
	}
	return filepath.Join(c.basePath, hash)
}

// metaPath returns the metadata file path for a hash.
func (c *Cache) metaPath(hash string) string {
	return c.pathForHash(hash) + ".meta"
}

func (c *Cache) writeMeta(hash string, meta *CacheEntry) error {
	dir := filepath.Dir(c.pathForHash(hash))
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return os.WriteFile(c.metaPath(hash), data, 0600)
}

func (c *Cache) readMeta(hash string) (*CacheEntry, error) {
	data, err := os.ReadFile(c.metaPath(hash))
	if err != nil {
		return nil, err
	}

	var meta CacheEntry
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func (c *Cache) updateAccessTime(hash string) error {
	meta, err := c.readMeta(hash)
	if err != nil {
		// Create new metadata if it doesn't exist
		info, statErr := os.Stat(c.pathForHash(hash))
		if statErr != nil {
			return statErr
		}
		meta = &CacheEntry{
			Hash:       hash,
			Size:       info.Size(),
			CreatedAt:  info.ModTime(),
			AccessedAt: time.Now(),
		}
	} else {
		meta.AccessedAt = time.Now()
	}

	return c.writeMeta(hash, meta)
}

func (c *Cache) delete(hash string) error {
	path := c.pathForHash(hash)
	metaPath := c.metaPath(hash)

	// Remove data file
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove metadata (best effort)
	_ = os.Remove(metaPath)

	return nil
}

func (c *Cache) listEntries() ([]CacheEntry, error) {
	var entries []CacheEntry

	err := filepath.Walk(c.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories and metadata files
		if info.IsDir() || filepath.Ext(path) == ".meta" {
			return nil
		}

		hash := filepath.Base(path)
		if !isValidHash(hash) {
			return nil
		}

		// Try to read metadata
		meta, err := c.readMeta(hash)
		if err != nil {
			// Construct from file info
			meta = &CacheEntry{
				Hash:       hash,
				Size:       info.Size(),
				CreatedAt:  info.ModTime(),
				AccessedAt: info.ModTime(),
			}
		}

		entries = append(entries, *meta)
		return nil
	})

	return entries, err
}

// computeHash calculates the SHA-256 hash of data.
func computeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// isValidHash checks if a string looks like a valid SHA-256 hash.
func isValidHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
