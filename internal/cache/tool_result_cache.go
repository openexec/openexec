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

// ToolResultCache provides caching for tool execution results.
// This avoids re-running deterministic tools with the same inputs.
type ToolResultCache struct {
	db  *sql.DB
	ttl time.Duration
}

// ToolResultCacheEntry represents a cached tool execution result
type ToolResultCacheEntry struct {
	ToolName    string    `json:"tool_name"`
	InputHash   string    `json:"input_hash"`
	Result      []byte    `json:"result"` // JSON-encoded result
	ExecutedAt  time.Time `json:"executed_at"`
	ExecutionMs int64     `json:"execution_ms"`
}

// NewToolResultCache creates a new tool result cache with the specified TTL.
func NewToolResultCache(projectDir string, ttl time.Duration) (*ToolResultCache, error) {
	dbPath := filepath.Join(projectDir, ".openexec", "cache.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open tool cache db: %w", err)
	}

	cache := &ToolResultCache{db: db, ttl: ttl}
	if err := cache.migrate(); err != nil {
		return nil, err
	}

	return cache, nil
}

// NewToolResultCacheWithDB creates a cache using an existing database connection.
func NewToolResultCacheWithDB(db *sql.DB, ttl time.Duration) (*ToolResultCache, error) {
	cache := &ToolResultCache{db: db, ttl: ttl}
	if err := cache.migrate(); err != nil {
		return nil, err
	}
	return cache, nil
}

func (c *ToolResultCache) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS tool_result_cache (
			tool_name TEXT NOT NULL,
			input_hash TEXT NOT NULL,
			result BLOB NOT NULL,
			executed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			execution_ms INTEGER DEFAULT 0,
			PRIMARY KEY (tool_name, input_hash)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tool_cache_time 
			ON tool_result_cache(tool_name, executed_at);`,
		`CREATE INDEX IF NOT EXISTS idx_tool_cache_lookup 
			ON tool_result_cache(tool_name, input_hash);`,
	}

	for _, q := range queries {
		if _, err := c.db.Exec(q); err != nil {
			return fmt.Errorf("tool cache migration failed: %w", err)
		}
	}
	return nil
}

// Get retrieves a cached tool result if it exists and is not expired.
// Returns nil if no valid cache entry exists.
func (c *ToolResultCache) Get(toolName string, input map[string]interface{}) ([]byte, error) {
	inputHash := computeInputHash(input)
	
	var entry ToolResultCacheEntry
	var result []byte

	query := `SELECT tool_name, input_hash, result, executed_at, execution_ms 
	          FROM tool_result_cache 
	          WHERE tool_name = ? AND input_hash = ?`
	
	err := c.db.QueryRow(query, toolName, inputHash).Scan(
		&entry.ToolName,
		&entry.InputHash,
		&result,
		&entry.ExecutedAt,
		&entry.ExecutionMs,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("tool cache get failed: %w", err)
	}

	// Check if cache entry has expired
	if time.Since(entry.ExecutedAt) > c.ttl {
		return nil, nil // Cache expired
	}

	return result, nil
}

// GetWithMetadata retrieves a cached result with execution metadata.
func (c *ToolResultCache) GetWithMetadata(toolName string, input map[string]interface{}) (*ToolResultCacheEntry, error) {
	inputHash := computeInputHash(input)
	
	var entry ToolResultCacheEntry
	var result []byte

	query := `SELECT tool_name, input_hash, result, executed_at, execution_ms 
	          FROM tool_result_cache 
	          WHERE tool_name = ? AND input_hash = ?`
	
	err := c.db.QueryRow(query, toolName, inputHash).Scan(
		&entry.ToolName,
		&entry.InputHash,
		&result,
		&entry.ExecutedAt,
		&entry.ExecutionMs,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("tool cache get failed: %w", err)
	}

	// Check if cache entry has expired
	if time.Since(entry.ExecutedAt) > c.ttl {
		return nil, nil // Cache expired
	}

	entry.Result = result
	return &entry, nil
}

// Set stores a tool result in the cache.
func (c *ToolResultCache) Set(toolName string, input map[string]interface{}, result []byte, executionMs int64) error {
	inputHash := computeInputHash(input)
	
	query := `INSERT OR REPLACE INTO tool_result_cache 
	          (tool_name, input_hash, result, executed_at, execution_ms) 
	          VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)`
	
	_, err := c.db.Exec(query, toolName, inputHash, result, executionMs)
	if err != nil {
		return fmt.Errorf("tool cache set failed: %w", err)
	}
	return nil
}

// Invalidate removes a cached entry for a specific tool and input.
func (c *ToolResultCache) Invalidate(toolName string, input map[string]interface{}) error {
	inputHash := computeInputHash(input)
	query := `DELETE FROM tool_result_cache WHERE tool_name = ? AND input_hash = ?`
	_, err := c.db.Exec(query, toolName, inputHash)
	if err != nil {
		return fmt.Errorf("tool cache invalidate failed: %w", err)
	}
	return nil
}

// InvalidateTool removes all cached entries for a specific tool.
func (c *ToolResultCache) InvalidateTool(toolName string) error {
	query := `DELETE FROM tool_result_cache WHERE tool_name = ?`
	_, err := c.db.Exec(query, toolName)
	if err != nil {
		return fmt.Errorf("tool cache invalidate tool failed: %w", err)
	}
	return nil
}

// Cleanup removes expired entries from the cache.
func (c *ToolResultCache) Cleanup() error {
	cutoff := time.Now().Add(-c.ttl)
	query := `DELETE FROM tool_result_cache WHERE executed_at < ?`
	_, err := c.db.Exec(query, cutoff)
	if err != nil {
		return fmt.Errorf("tool cache cleanup failed: %w", err)
	}
	return nil
}

// Stats returns cache statistics.
func (c *ToolResultCache) Stats() (total int, expired int, err error) {
	// Total entries
	err = c.db.QueryRow(`SELECT COUNT(*) FROM tool_result_cache`).Scan(&total)
	if err != nil {
		return 0, 0, fmt.Errorf("tool cache stats failed: %w", err)
	}

	// Expired entries
	cutoff := time.Now().Add(-c.ttl).UTC().Format("2006-01-02 15:04:05")
	err = c.db.QueryRow(`SELECT COUNT(*) FROM tool_result_cache WHERE executed_at < ?`, cutoff).Scan(&expired)
	if err != nil {
		return 0, 0, fmt.Errorf("tool cache stats failed: %w", err)
	}

	return total, expired, nil
}

// Close closes the cache database connection.
func (c *ToolResultCache) Close() error {
	return c.db.Close()
}

// computeInputHash computes a hash of tool input for cache keys.
func computeInputHash(input map[string]interface{}) string {
	// Sort keys for consistent hashing
	h := sha256.New()
	
	// Marshal to JSON for consistent serialization
	data, err := json.Marshal(input)
	if err != nil {
		// Fallback: hash the error string
		h.Write([]byte(err.Error()))
		return hex.EncodeToString(h.Sum(nil))
	}
	
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// ShouldCache determines if a tool result should be cached based on tool name.
// Some tools (like network calls, time-dependent) should not be cached.
func ShouldCache(toolName string) bool {
	// Tools that should NOT be cached
	nonCacheable := map[string]bool{
		"deploy":        true, // Deployment has side effects
		"chat":          true, // Conversational
		"time":          true, // Time-dependent
		"random":        true, // Non-deterministic
		"network_fetch": true, // External state
	}
	
	return !nonCacheable[toolName]
}

// CacheableResult wraps a result with cache metadata
type CacheableResult struct {
	Data        interface{} `json:"data"`
	Cached      bool        `json:"cached"`
	ExecutedAt  time.Time   `json:"executed_at"`
	ExecutionMs int64       `json:"execution_ms"`
}
