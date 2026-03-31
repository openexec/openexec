// Package context provides intelligent context pruning for OpenExec.
// It reduces token usage by selecting only the most relevant files for a given task.
package context

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/memory"
	_ "github.com/mattn/go-sqlite3"
)

// Pruner intelligently selects relevant files to minimize token usage.
type Pruner struct {
	knowledgeStore *knowledge.Store
	memoryManager  *memory.MemoryManager
	db             *sql.DB
	config         *PrunerConfig
}

// PrunerConfig contains configuration for the pruner.
type PrunerConfig struct {
	// MaxTokens is the maximum tokens to include in context.
	MaxTokens int

	// MaxFiles is the maximum number of files to include.
	MaxFiles int

	// MinRelevanceScore is the minimum score for a file to be included (0-100).
	MinRelevanceScore float64

	// SymbolMatchWeight is the weight given to symbol name matches.
	SymbolMatchWeight float64

	// ContentMatchWeight is the weight given to content similarity.
	ContentMatchWeight float64

	// PathMatchWeight is the weight given to path relevance.
	PathMatchWeight float64

	// RecencyWeight is the weight given to recent file access.
	RecencyWeight float64

	// EnableCaching enables result caching.
	EnableCaching bool

	// CacheTTL is how long to cache pruning results.
	CacheTTL time.Duration
}

// DefaultPrunerConfig returns default pruner configuration.
func DefaultPrunerConfig() *PrunerConfig {
	return &PrunerConfig{
		MaxTokens:          100000, // ~25k tokens for context
		MaxFiles:           20,
		MinRelevanceScore:  10.0,
		SymbolMatchWeight:  10.0,
		ContentMatchWeight: 5.0,
		PathMatchWeight:    3.0,
		RecencyWeight:      2.0,
		EnableCaching:      true,
		CacheTTL:           5 * time.Minute,
	}
}

// FileScore represents a file's relevance score.
type FileScore struct {
	Path       string
	Content    string
	TokenCount int
	Score      float64
	Breakdown  ScoreBreakdown
}

// ScoreBreakdown shows how the score was calculated.
type ScoreBreakdown struct {
	SymbolScore   float64
	ContentScore  float64
	PathScore     float64
	RecencyScore  float64
}

// PruneResult contains the result of a pruning operation.
type PruneResult struct {
	Files          []FileScore
	TotalTokens    int
	TotalFiles     int
	OriginalFiles  int
	Query          string
	CacheHit       bool
	ProcessingTime time.Duration
}

// NewPruner creates a new context pruner.
func NewPruner(projectDir string, knowledgeStore *knowledge.Store, memoryManager *memory.MemoryManager, config *PrunerConfig) (*Pruner, error) {
	if config == nil {
		config = DefaultPrunerConfig()
	}

	dbPath := filepath.Join(projectDir, ".openexec", "pruner.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open pruner db: %w", err)
	}

	pruner := &Pruner{
		knowledgeStore: knowledgeStore,
		memoryManager:  memoryManager,
		db:             db,
		config:         config,
	}

	if err := pruner.migrate(); err != nil {
		return nil, err
	}

	return pruner, nil
}

func (p *Pruner) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS prune_cache (
			query_hash TEXT PRIMARY KEY,
			query TEXT NOT NULL,
			result TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			access_count INTEGER DEFAULT 0,
			last_accessed DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_prune_cache_time 
			ON prune_cache(created_at);`,
	}

	for _, q := range queries {
		if _, err := p.db.Exec(q); err != nil {
			return fmt.Errorf("pruner migration failed: %w", err)
		}
	}
	return nil
}

// Prune selects the most relevant files for a given query.
func (p *Pruner) Prune(files []FileInfo, query string) (*PruneResult, error) {
	start := time.Now()

	// Check cache
	if p.config.EnableCaching {
		if cached, err := p.getCachedResult(query); err == nil && cached != nil {
			cached.CacheHit = true
			cached.ProcessingTime = time.Since(start)
			return cached, nil
		}
	}

	// Score all files
	scoredFiles := p.scoreFiles(files, query)

	// Sort by score (descending)
	sort.Slice(scoredFiles, func(i, j int) bool {
		return scoredFiles[i].Score > scoredFiles[j].Score
	})

	// Select files that fit in token budget
	var selected []FileScore
	totalTokens := 0

	for _, file := range scoredFiles {
		// Skip if below minimum relevance
		if file.Score < p.config.MinRelevanceScore {
			continue
		}

		// Check token budget
		if totalTokens+file.TokenCount > p.config.MaxTokens {
			break
		}

		// Check file limit
		if len(selected) >= p.config.MaxFiles {
			break
		}

		selected = append(selected, file)
		totalTokens += file.TokenCount
	}

	result := &PruneResult{
		Files:          selected,
		TotalTokens:    totalTokens,
		TotalFiles:     len(selected),
		OriginalFiles:  len(files),
		Query:          query,
		CacheHit:       false,
		ProcessingTime: time.Since(start),
	}

	// Cache result
	if p.config.EnableCaching {
		_ = p.cacheResult(query, result)
	}

	return result, nil
}

// scoreFiles calculates relevance scores for all files.
func (p *Pruner) scoreFiles(files []FileInfo, query string) []FileScore {
	scored := make([]FileScore, 0, len(files))
	queryTerms := p.extractTerms(query)

	for _, file := range files {
		score, breakdown := p.calculateScore(file, query, queryTerms)
		scored = append(scored, FileScore{
			Path:       file.Path,
			Content:    file.Content,
			TokenCount: p.estimateTokens(file.Content),
			Score:      score,
			Breakdown:  breakdown,
		})
	}

	return scored
}

// calculateScore calculates a file's relevance score.
func (p *Pruner) calculateScore(file FileInfo, query string, queryTerms []string) (float64, ScoreBreakdown) {
	breakdown := ScoreBreakdown{}

	// 1. Symbol name matching (uses knowledge store)
	breakdown.SymbolScore = p.scoreSymbols(file, query, queryTerms)

	// 2. Content similarity
	breakdown.ContentScore = p.scoreContent(file, query, queryTerms)

	// 3. Path relevance
	breakdown.PathScore = p.scorePath(file, query, queryTerms)

	// 4. Recency (uses memory manager)
	breakdown.RecencyScore = p.scoreRecency(file)

	// Calculate weighted total
	total := breakdown.SymbolScore*p.config.SymbolMatchWeight +
		breakdown.ContentScore*p.config.ContentMatchWeight +
		breakdown.PathScore*p.config.PathMatchWeight +
		breakdown.RecencyScore*p.config.RecencyWeight

	return total, breakdown
}

// scoreSymbols scores based on symbol name matches.
func (p *Pruner) scoreSymbols(file FileInfo, query string, queryTerms []string) float64 {
	if p.knowledgeStore == nil {
		return 0
	}

	score := 0.0

	// Get symbols for this file from knowledge store
	symbols, err := p.knowledgeStore.ListSymbols()
	if err != nil {
		return 0
	}

	for _, symbol := range symbols {
		if symbol.FilePath != file.Path {
			continue
		}

		symbolName := strings.ToLower(symbol.Name)

		// Exact match
		if strings.Contains(symbolName, strings.ToLower(query)) {
			score += 10.0
			continue
		}

		// Term match
		for _, term := range queryTerms {
			if strings.Contains(symbolName, term) {
				score += 5.0
			}
		}
	}

	return score
}

// scoreContent scores based on content similarity.
func (p *Pruner) scoreContent(file FileInfo, query string, queryTerms []string) float64 {
	content := strings.ToLower(file.Content)
	score := 0.0

	// Exact query match
	if strings.Contains(content, strings.ToLower(query)) {
		score += 10.0
	}

	// Term frequency
	for _, term := range queryTerms {
		count := strings.Count(content, term)
		score += float64(count) * 2.0
	}

	// TF-IDF-like scoring (simplified)
	words := strings.Fields(content)
	if len(words) > 0 {
		termMatches := 0
		for _, term := range queryTerms {
			for _, word := range words {
				if word == term {
					termMatches++
				}
			}
		}
		density := float64(termMatches) / float64(len(words))
		score += density * 100.0
	}

	return score
}

// scorePath scores based on path relevance.
func (p *Pruner) scorePath(file FileInfo, query string, queryTerms []string) float64 {
	path := strings.ToLower(file.Path)
	score := 0.0

	// Path contains query terms
	for _, term := range queryTerms {
		if strings.Contains(path, term) {
			score += 5.0
		}
	}

	// File name matches
	fileName := strings.ToLower(filepath.Base(file.Path))
	for _, term := range queryTerms {
		if strings.Contains(fileName, term) {
			score += 10.0
		}
	}

	return score
}

// scoreRecency scores based on recent access.
func (p *Pruner) scoreRecency(file FileInfo) float64 {
	if p.memoryManager == nil {
		return 0
	}

	// Check if file is in recent memory
	entries, err := p.memoryManager.Search(file.Path)
	if err != nil || len(entries) == 0 {
		return 0
	}

	// Score based on how recently accessed
	for _, entry := range entries {
		if time.Since(entry.ExtractedAt) < 1*time.Hour {
			return 10.0
		} else if time.Since(entry.ExtractedAt) < 24*time.Hour {
			return 5.0
		} else if time.Since(entry.ExtractedAt) < 7*24*time.Hour {
			return 2.0
		}
	}

	return 0
}

// extractTerms extracts searchable terms from a query.
func (p *Pruner) extractTerms(query string) []string {
	// Normalize
	query = strings.ToLower(query)

	// Remove punctuation
	query = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(query, " ")

	// Split into words
	words := strings.Fields(query)

	// Filter out stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true, "can": true,
		"need": true, "dare": true, "ought": true, "used": true, "to": true,
		"of": true, "in": true, "for": true, "on": true, "with": true,
		"at": true, "by": true, "from": true, "as": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "between": true, "under": true,
		"and": true, "but": true, "or": true, "yet": true, "so": true,
		"if": true, "because": true, "although": true, "though": true,
		"while": true, "where": true, "when": true, "that": true, "which": true,
		"who": true, "whom": true, "whose": true, "what": true, "this": true,
		"these": true, "those": true, "i": true, "me": true, "my": true,
		"myself": true, "we": true, "our": true, "you": true, "your": true,
		"he": true, "him": true, "his": true, "she": true, "her": true,
		"it": true, "its": true, "they": true, "them": true, "their": true,
		"fix": true, "bug": true, "issue": true, "error": true, "problem": true,
	}

	var terms []string
	for _, word := range words {
		if !stopWords[word] && len(word) > 2 {
			terms = append(terms, word)
		}
	}

	return terms
}

// estimateTokens estimates the token count for content.
// Rough approximation: ~4 characters per token for code.
func (p *Pruner) estimateTokens(content string) int {
	// Count non-whitespace characters
	chars := 0
	for _, r := range content {
		if !unicode.IsSpace(r) {
			chars++
		}
	}
	return chars / 4
}

// getCachedResult retrieves a cached pruning result.
func (p *Pruner) getCachedResult(query string) (*PruneResult, error) {
	queryHash := p.hashQuery(query)

	var resultJSON string
	var createdAt time.Time

	query := `SELECT result, created_at FROM prune_cache WHERE query_hash = ?`
	err := p.db.QueryRow(query, queryHash).Scan(&resultJSON, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Check TTL
	if time.Since(createdAt) > p.config.CacheTTL {
		return nil, nil
	}

	// Update access stats
	_, _ = p.db.Exec(
		`UPDATE prune_cache SET access_count = access_count + 1, last_accessed = CURRENT_TIMESTAMP WHERE query_hash = ?`,
		queryHash,
	)

	// Parse result (simplified - would use JSON in production)
	// For now, return nil to indicate cache miss
	return nil, nil
}

// cacheResult caches a pruning result.
func (p *Pruner) cacheResult(query string, result *PruneResult) error {
	queryHash := p.hashQuery(query)

	// In production, serialize result to JSON
	// For now, just store placeholder
	_, err := p.db.Exec(
		`INSERT OR REPLACE INTO prune_cache (query_hash, query, result) VALUES (?, ?, ?)`,
		queryHash,
		query,
		"cached",
	)

	return err
}

// hashQuery creates a hash of the query for caching.
func (p *Pruner) hashQuery(query string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(query)))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes
}

// Cleanup removes old cache entries.
func (p *Pruner) Cleanup(olderThan time.Time) error {
	_, err := p.db.Exec(`DELETE FROM prune_cache WHERE created_at < ?`, olderThan)
	return err
}

// Close closes the pruner database connection.
func (p *Pruner) Close() error {
	return p.db.Close()
}

// FileInfo represents information about a file for pruning.
type FileInfo struct {
	Path    string
	Content string
}
