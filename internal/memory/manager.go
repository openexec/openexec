package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// MemoryManager provides high-level memory management with persistence.
type MemoryManager struct {
	system *MemorySystem
	db     *sql.DB
	mu     sync.RWMutex
}

// NewMemoryManager creates a new memory manager.
func NewMemoryManager(projectDir string) (*MemoryManager, error) {
	system := NewMemorySystem(projectDir)

	dbPath := filepath.Join(projectDir, ".openexec", "memory.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open memory db: %w", err)
	}

	manager := &MemoryManager{
		system: system,
		db:     db,
	}

	if err := manager.migrate(); err != nil {
		return nil, err
	}

	return manager, nil
}

// NewMemoryManagerWithDB creates a manager using an existing database.
func NewMemoryManagerWithDB(system *MemorySystem, db *sql.DB) (*MemoryManager, error) {
	manager := &MemoryManager{
		system: system,
		db:     db,
	}

	if err := manager.migrate(); err != nil {
		return nil, err
	}

	return manager, nil
}

func (m *MemoryManager) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS memory_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			source TEXT,
			layer TEXT NOT NULL,
			extracted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			access_count INTEGER DEFAULT 0,
			last_accessed DATETIME
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_entries_key 
			ON memory_entries(category, key, layer);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_entries_category 
			ON memory_entries(category);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_entries_layer 
			ON memory_entries(layer);`,
		`CREATE TABLE IF NOT EXISTS memory_sessions (
			id TEXT PRIMARY KEY,
			start_time DATETIME DEFAULT CURRENT_TIMESTAMP,
			end_time DATETIME,
			summary TEXT,
			decisions TEXT, -- JSON array
			patterns TEXT,  -- JSON array
			preferences TEXT -- JSON array
		);`,
		`CREATE TABLE IF NOT EXISTS memory_access_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entry_id INTEGER,
			accessed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			context TEXT,
			FOREIGN KEY (entry_id) REFERENCES memory_entries(id)
		);`,
	}

	for _, q := range queries {
		if _, err := m.db.Exec(q); err != nil {
			return fmt.Errorf("memory migration failed: %w", err)
		}
	}
	return nil
}

// LoadContext loads the full memory context for a session.
func (m *MemoryManager) LoadContext() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.system.LoadMerged()
}

// GetEntry retrieves a specific memory entry.
func (m *MemoryManager) GetEntry(category, key string) (*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry := &MemoryEntry{}
	var accessCount int
	var lastAccessed sql.NullTime

	query := `SELECT category, key, value, source, layer, extracted_at 
	          FROM memory_entries WHERE category = ? AND key = ?`
	
	err := m.db.QueryRow(query, category, key).Scan(
		&entry.Category,
		&entry.Key,
		&entry.Value,
		&entry.Source,
		&entry.Layer,
		&entry.ExtractedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get entry: %w", err)
	}

	// Update access stats
	_, _ = m.db.Exec(
		`UPDATE memory_entries SET access_count = access_count + 1, last_accessed = CURRENT_TIMESTAMP 
		 WHERE category = ? AND key = ?`,
		category, key,
	)

	// Log access
	_, _ = m.db.Exec(
		`INSERT INTO memory_access_log (entry_id, context) 
		 SELECT id, ? FROM memory_entries WHERE category = ? AND key = ?`,
		"retrieval", category, key,
	)

	return entry, nil
}

// StoreEntry stores a memory entry.
func (m *MemoryManager) StoreEntry(entry *MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `INSERT OR REPLACE INTO memory_entries 
	          (category, key, value, source, layer, extracted_at)
	          VALUES (?, ?, ?, ?, ?, ?)`
	
	_, err := m.db.Exec(query,
		entry.Category,
		entry.Key,
		entry.Value,
		entry.Source,
		entry.Layer,
		entry.ExtractedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to store entry: %w", err)
	}

	return nil
}

// StoreEntries stores multiple memory entries.
func (m *MemoryManager) StoreEntries(entries []*MemoryEntry) error {
	for _, entry := range entries {
		if err := m.StoreEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

// ListEntries lists all memory entries, optionally filtered by category.
func (m *MemoryManager) ListEntries(category string) ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var query string
	var args []interface{}

	if category != "" {
		query = `SELECT category, key, value, source, layer, extracted_at 
		         FROM memory_entries WHERE category = ? ORDER BY extracted_at DESC`
		args = append(args, category)
	} else {
		query = `SELECT category, key, value, source, layer, extracted_at 
		         FROM memory_entries ORDER BY extracted_at DESC`
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}
	defer rows.Close()

	return m.scanEntries(rows)
}

// Search searches memory entries.
func (m *MemoryManager) Search(query string) ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	searchPattern := "%" + query + "%"
	
	sqlQuery := `SELECT category, key, value, source, layer, extracted_at 
	             FROM memory_entries 
	             WHERE key LIKE ? OR value LIKE ? OR category LIKE ?
	             ORDER BY 
	               CASE WHEN key LIKE ? THEN 1 ELSE 2 END,
	               access_count DESC`
	
	rows, err := m.db.Query(sqlQuery, searchPattern, searchPattern, searchPattern, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search entries: %w", err)
	}
	defer rows.Close()

	return m.scanEntries(rows)
}

// DeleteEntry deletes a memory entry.
func (m *MemoryManager) DeleteEntry(category, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `DELETE FROM memory_entries WHERE category = ? AND key = ?`
	_, err := m.db.Exec(query, category, key)
	if err != nil {
		return fmt.Errorf("failed to delete entry: %w", err)
	}

	return nil
}

// RecordSession records a session for later analysis.
func (m *MemoryManager) RecordSession(session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	decisionsJSON, _ := json.Marshal(session.Decisions)
	patternsJSON, _ := json.Marshal(session.Patterns)
	preferencesJSON, _ := json.Marshal(session.Preferences)

	query := `INSERT OR REPLACE INTO memory_sessions 
	          (id, start_time, end_time, summary, decisions, patterns, preferences)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`
	
	_, err := m.db.Exec(query,
		session.ID,
		session.StartTime,
		session.EndTime,
		"", // Summary could be generated
		string(decisionsJSON),
		string(patternsJSON),
		string(preferencesJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to record session: %w", err)
	}

	return nil
}

// ExtractFromSession extracts memories from a recorded session.
func (m *MemoryManager) ExtractFromSession(sessionID string) ([]*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var decisionsJSON, patternsJSON, preferencesJSON string
	
	query := `SELECT decisions, patterns, preferences FROM memory_sessions WHERE id = ?`
	err := m.db.QueryRow(query, sessionID).Scan(&decisionsJSON, &patternsJSON, &preferencesJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var entries []*MemoryEntry
	now := time.Now().UTC()

	// Extract decisions
	var decisions []Decision
	if err := json.Unmarshal([]byte(decisionsJSON), &decisions); err == nil {
		for _, d := range decisions {
			entries = append(entries, &MemoryEntry{
				Category:    "decision",
				Key:         d.Topic,
				Value:       d.Rationale,
				Source:      "session:" + sessionID,
				Layer:       LayerProject.String(),
				ExtractedAt: now,
			})
		}
	}

	// Extract patterns
	var patterns []Pattern
	if err := json.Unmarshal([]byte(patternsJSON), &patterns); err == nil {
		for _, p := range patterns {
			entries = append(entries, &MemoryEntry{
				Category:    "pattern",
				Key:         p.Name,
				Value:       p.Description,
				Source:      "session:" + sessionID,
				Layer:       LayerProject.String(),
				ExtractedAt: now,
			})
		}
	}

	// Extract preferences
	var preferences []Preference
	if err := json.Unmarshal([]byte(preferencesJSON), &preferences); err == nil {
		for _, p := range preferences {
			entries = append(entries, &MemoryEntry{
				Category:    "preference",
				Key:         p.Key,
				Value:       p.Value,
				Source:      "session:" + sessionID,
				Layer:       LayerUser.String(),
				ExtractedAt: now,
			})
		}
	}

	return entries, nil
}

// GetStats returns memory statistics.
func (m *MemoryManager) GetStats() (*MemoryStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &MemoryStats{}

	// Total entries
	err := m.db.QueryRow(`SELECT COUNT(*) FROM memory_entries`).Scan(&stats.TotalEntries)
	if err != nil {
		return nil, err
	}

	// Entries by category
	rows, err := m.db.Query(`SELECT category, COUNT(*) FROM memory_entries GROUP BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats.ByCategory = make(map[string]int)
	for rows.Next() {
		var category string
		var count int
		if err := rows.Scan(&category, &count); err != nil {
			return nil, err
		}
		stats.ByCategory[category] = count
	}

	// Entries by layer
	rows, err = m.db.Query(`SELECT layer, COUNT(*) FROM memory_entries GROUP BY layer`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats.ByLayer = make(map[string]int)
	for rows.Next() {
		var layer string
		var count int
		if err := rows.Scan(&layer, &count); err != nil {
			return nil, err
		}
		stats.ByLayer[layer] = count
	}

	// Total sessions
	err = m.db.QueryRow(`SELECT COUNT(*) FROM memory_sessions`).Scan(&stats.TotalSessions)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// Cleanup removes old entries and sessions.
func (m *MemoryManager) Cleanup(olderThan time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Delete old entries
	_, err := m.db.Exec(`DELETE FROM memory_entries WHERE extracted_at < ?`, olderThan)
	if err != nil {
		return fmt.Errorf("failed to cleanup entries: %w", err)
	}

	// Delete old sessions
	_, err = m.db.Exec(`DELETE FROM memory_sessions WHERE end_time < ?`, olderThan)
	if err != nil {
		return fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	// Delete old access logs
	_, err = m.db.Exec(`DELETE FROM memory_access_log WHERE accessed_at < ?`, olderThan)
	if err != nil {
		return fmt.Errorf("failed to cleanup access logs: %w", err)
	}

	return nil
}

// Close closes the memory manager.
func (m *MemoryManager) Close() error {
	return m.db.Close()
}

func (m *MemoryManager) scanEntries(rows *sql.Rows) ([]*MemoryEntry, error) {
	var entries []*MemoryEntry

	for rows.Next() {
		entry := &MemoryEntry{}
		err := rows.Scan(
			&entry.Category,
			&entry.Key,
			&entry.Value,
			&entry.Source,
			&entry.Layer,
			&entry.ExtractedAt,
		)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// MemoryStats contains statistics about the memory system.
type MemoryStats struct {
	TotalEntries  int
	ByCategory    map[string]int
	ByLayer       map[string]int
	TotalSessions int
}
