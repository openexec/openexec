// Package cache provides caching mechanisms for OpenExec to improve performance
// and reduce redundant computations.
package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// KnowledgeCache provides persistent caching for knowledge index operations.
// It stores indexed symbols and file metadata to avoid re-indexing unchanged files.
type KnowledgeCache struct {
	db  *sql.DB
	ttl time.Duration
}

// KnowledgeCacheEntry represents a cached index entry
type KnowledgeCacheEntry struct {
	ProjectPath string    `json:"project_path"`
	FilePath    string    `json:"file_path"`
	FileHash    string    `json:"file_hash"`
	Symbols     []byte    `json:"symbols"` // JSON-encoded symbols
	IndexedAt   time.Time `json:"indexed_at"`
}

// NewKnowledgeCache creates a new knowledge cache with the specified TTL.
// The cache is stored in SQLite for persistence across sessions.
func NewKnowledgeCache(projectDir string, ttl time.Duration) (*KnowledgeCache, error) {
	dbPath := filepath.Join(projectDir, ".openexec", "cache.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open cache db: %w", err)
	}

	cache := &KnowledgeCache{db: db, ttl: ttl}
	if err := cache.migrate(); err != nil {
		return nil, err
	}

	return cache, nil
}

// NewKnowledgeCacheWithDB creates a cache using an existing database connection.
func NewKnowledgeCacheWithDB(db *sql.DB, ttl time.Duration) (*KnowledgeCache, error) {
	cache := &KnowledgeCache{db: db, ttl: ttl}
	if err := cache.migrate(); err != nil {
		return nil, err
	}
	return cache, nil
}

func (c *KnowledgeCache) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS knowledge_cache (
			project_path TEXT NOT NULL,
			file_path TEXT NOT NULL,
			file_hash TEXT NOT NULL,
			symbols BLOB NOT NULL,
			indexed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (project_path, file_path)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_cache_time 
			ON knowledge_cache(project_path, indexed_at);`,
	}

	for _, q := range queries {
		if _, err := c.db.Exec(q); err != nil {
			return fmt.Errorf("cache migration failed: %w", err)
		}
	}
	return nil
}

// Get retrieves cached symbols for a file if they exist and are not expired.
// Returns nil if no valid cache entry exists.
func (c *KnowledgeCache) Get(projectPath, filePath string, currentHash string) ([]byte, error) {
	var entry KnowledgeCacheEntry
	var symbols []byte

	query := `SELECT project_path, file_path, file_hash, symbols, indexed_at 
	          FROM knowledge_cache 
	          WHERE project_path = ? AND file_path = ?`
	
	err := c.db.QueryRow(query, projectPath, filePath).Scan(
		&entry.ProjectPath,
		&entry.FilePath,
		&entry.FileHash,
		&symbols,
		&entry.IndexedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("cache get failed: %w", err)
	}

	// Check if file has changed
	if entry.FileHash != currentHash {
		return nil, nil // File changed, cache invalid
	}

	// Check if cache entry has expired
	if time.Since(entry.IndexedAt) > c.ttl {
		return nil, nil // Cache expired
	}

	return symbols, nil
}

// Set stores symbols in the cache for a file.
func (c *KnowledgeCache) Set(projectPath, filePath, fileHash string, symbols []byte) error {
	query := `INSERT OR REPLACE INTO knowledge_cache 
	          (project_path, file_path, file_hash, symbols, indexed_at) 
	          VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`
	
	_, err := c.db.Exec(query, projectPath, filePath, fileHash, symbols)
	if err != nil {
		return fmt.Errorf("cache set failed: %w", err)
	}
	return nil
}

// Invalidate removes a cached entry for a specific file.
func (c *KnowledgeCache) Invalidate(projectPath, filePath string) error {
	query := `DELETE FROM knowledge_cache WHERE project_path = ? AND file_path = ?`
	_, err := c.db.Exec(query, projectPath, filePath)
	if err != nil {
		return fmt.Errorf("cache invalidate failed: %w", err)
	}
	return nil
}

// InvalidateProject removes all cached entries for a project.
func (c *KnowledgeCache) InvalidateProject(projectPath string) error {
	query := `DELETE FROM knowledge_cache WHERE project_path = ?`
	_, err := c.db.Exec(query, projectPath)
	if err != nil {
		return fmt.Errorf("cache invalidate project failed: %w", err)
	}
	return nil
}

// Cleanup removes expired entries from the cache.
func (c *KnowledgeCache) Cleanup() error {
	cutoff := time.Now().Add(-c.ttl)
	query := `DELETE FROM knowledge_cache WHERE indexed_at < ?`
	_, err := c.db.Exec(query, cutoff)
	if err != nil {
		return fmt.Errorf("cache cleanup failed: %w", err)
	}
	return nil
}

// Stats returns cache statistics.
func (c *KnowledgeCache) Stats() (total int, expired int, err error) {
	// Total entries
	err = c.db.QueryRow(`SELECT COUNT(*) FROM knowledge_cache`).Scan(&total)
	if err != nil {
		return 0, 0, fmt.Errorf("cache stats failed: %w", err)
	}

	// Expired entries
	cutoff := time.Now().Add(-c.ttl).UTC().Format("2006-01-02 15:04:05")
	err = c.db.QueryRow(`SELECT COUNT(*) FROM knowledge_cache WHERE indexed_at < ?`, cutoff).Scan(&expired)
	if err != nil {
		return 0, 0, fmt.Errorf("cache stats failed: %w", err)
	}

	return total, expired, nil
}

// Close closes the cache database connection.
func (c *KnowledgeCache) Close() error {
	return c.db.Close()
}

// ComputeFileHash computes a hash of file content for cache invalidation.
func ComputeFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// ComputeFileHashString computes a hash from a string.
func ComputeFileHashString(content string) string {
	return ComputeFileHash([]byte(content))
}

// SerializeSymbols serializes symbol records to JSON for caching.
func SerializeSymbols(symbols interface{}) ([]byte, error) {
	return json.Marshal(symbols)
}

// DeserializeSymbols deserializes symbol records from JSON.
func DeserializeSymbols(data []byte, dest interface{}) error {
	return json.Unmarshal(data, dest)
}
