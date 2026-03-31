// Package predictive provides predictive file loading for OpenExec.
// It analyzes tasks and pre-loads files likely to be needed, eliminating round-trips.
package predictive

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
	_ "github.com/mattn/go-sqlite3"
)

// Loader predicts and pre-loads files based on task analysis.
type Loader struct {
	knowledgeStore *knowledge.Store
	db             *sql.DB
	config         *LoaderConfig
	fileCache      map[string]*FileEntry
}

// LoaderConfig contains configuration for the predictive loader.
type LoaderConfig struct {
	// MaxPreloadFiles is the maximum number of files to preload.
	MaxPreloadFiles int

	// ConfidenceThreshold is the minimum confidence score to preload (0-100).
	ConfidenceThreshold float64

	// EnableLearning enables learning from actual file usage.
	EnableLearning bool

	// SymbolMatchWeight is the weight for symbol name matches.
	SymbolMatchWeight float64

	// PatternMatchWeight is the weight for pattern matches.
	PatternMatchWeight float64

	// HistoryMatchWeight is the weight for historical access patterns.
	HistoryMatchWeight float64
}

// DefaultLoaderConfig returns default loader configuration.
func DefaultLoaderConfig() *LoaderConfig {
	return &LoaderConfig{
		MaxPreloadFiles:     10,
		ConfidenceThreshold: 30.0,
		EnableLearning:      true,
		SymbolMatchWeight:   10.0,
		PatternMatchWeight:  5.0,
		HistoryMatchWeight:  3.0,
	}
}

// FileEntry represents a preloaded file.
type FileEntry struct {
	Path        string
	Content     string
	Confidence  float64
	LoadedAt    time.Time
	AccessCount int
}

// Prediction represents a file prediction with confidence.
type Prediction struct {
	Path       string
	Confidence float64
	Reason     string
}

// PredictionResult contains the result of a prediction.
type PredictionResult struct {
	Predictions    []Prediction
	LoadedFiles    []FileEntry
	CacheHits      int
	CacheMisses    int
	ProcessingTime time.Duration
}

// NewLoader creates a new predictive loader.
func NewLoader(projectDir string, knowledgeStore *knowledge.Store, config *LoaderConfig) (*Loader, error) {
	if config == nil {
		config = DefaultLoaderConfig()
	}

	dbPath := filepath.Join(projectDir, ".openexec", "predictive.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open predictive db: %w", err)
	}

	loader := &Loader{
		knowledgeStore: knowledgeStore,
		db:             db,
		config:         config,
		fileCache:      make(map[string]*FileEntry),
	}

	if err := loader.migrate(); err != nil {
		return nil, err
	}

	return loader, nil
}

func (l *Loader) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS file_access_patterns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_pattern TEXT NOT NULL,
			file_path TEXT NOT NULL,
			access_count INTEGER DEFAULT 1,
			last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(task_pattern, file_path)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_patterns_task 
			ON file_access_patterns(task_pattern);`,
		`CREATE INDEX IF NOT EXISTS idx_patterns_file 
			ON file_access_patterns(file_path);`,
		`CREATE TABLE IF NOT EXISTS symbol_predictions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol_name TEXT NOT NULL,
			file_path TEXT NOT NULL,
			prediction_count INTEGER DEFAULT 1,
			success_count INTEGER DEFAULT 0,
			UNIQUE(symbol_name, file_path)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_symbols_name 
			ON symbol_predictions(symbol_name);`,
	}

	for _, q := range queries {
		if _, err := l.db.Exec(q); err != nil {
			return fmt.Errorf("predictive migration failed: %w", err)
		}
	}
	return nil
}

// PredictAndLoad analyzes a task and pre-loads likely files.
func (l *Loader) PredictAndLoad(ctx context.Context, task string, allFiles []string) (*PredictionResult, error) {
	start := time.Now()

	// Extract symbols from task
	symbols := l.extractSymbols(task)

	// Make predictions
	predictions := l.predictFiles(task, symbols, allFiles)

	// Sort by confidence
	l.sortPredictions(predictions)

	// Load top predictions
	var loaded []FileEntry
	cacheHits := 0
	cacheMisses := 0

	for _, pred := range predictions {
		if len(loaded) >= l.config.MaxPreloadFiles {
			break
		}

		if pred.Confidence < l.config.ConfidenceThreshold {
			continue
		}

		// Check if already cached
		if entry, ok := l.fileCache[pred.Path]; ok {
			loaded = append(loaded, *entry)
			cacheHits++
			continue
		}

		// Load file
		content, err := l.loadFile(pred.Path)
		if err != nil {
			continue
		}

		entry := &FileEntry{
			Path:       pred.Path,
			Content:    content,
			Confidence: pred.Confidence,
			LoadedAt:   time.Now(),
		}

		l.fileCache[pred.Path] = entry
		loaded = append(loaded, *entry)
		cacheMisses++
	}

	result := &PredictionResult{
		Predictions:    predictions,
		LoadedFiles:    loaded,
		CacheHits:      cacheHits,
		CacheMisses:    cacheMisses,
		ProcessingTime: time.Since(start),
	}

	return result, nil
}

// predictFiles generates file predictions.
func (l *Loader) predictFiles(task string, symbols []string, allFiles []string) []Prediction {
	predictions := make(map[string]*Prediction)

	// Predict based on symbols
	for _, symbol := range symbols {
		files := l.predictFromSymbol(symbol, allFiles)
		for _, pred := range files {
			if existing, ok := predictions[pred.Path]; ok {
				existing.Confidence += pred.Confidence * l.config.SymbolMatchWeight
			} else {
				pred.Confidence *= l.config.SymbolMatchWeight
				predictions[pred.Path] = &pred
			}
		}
	}

	// Predict based on patterns
	patternPreds := l.predictFromPatterns(task, allFiles)
	for _, pred := range patternPreds {
		if existing, ok := predictions[pred.Path]; ok {
			existing.Confidence += pred.Confidence * l.config.PatternMatchWeight
		} else {
			pred.Confidence *= l.config.PatternMatchWeight
			predictions[pred.Path] = &pred
		}
	}

	// Predict based on history
	historyPreds := l.predictFromHistory(task, allFiles)
	for _, pred := range historyPreds {
		if existing, ok := predictions[pred.Path]; ok {
			existing.Confidence += pred.Confidence * l.config.HistoryMatchWeight
		} else {
			pred.Confidence *= l.config.HistoryMatchWeight
			predictions[pred.Path] = &pred
		}
	}

	// Convert to slice
	result := make([]Prediction, 0, len(predictions))
	for _, pred := range predictions {
		result = append(result, *pred)
	}

	return result
}

// predictFromSymbol predicts files based on symbol names.
func (l *Loader) predictFromSymbol(symbol string, allFiles []string) []Prediction {
	var predictions []Prediction

	if l.knowledgeStore == nil {
		return predictions
	}

	// Look up symbol in knowledge store
	symbolRecord, err := l.knowledgeStore.GetSymbol(symbol)
	if err == nil && symbolRecord != nil {
		predictions = append(predictions, Prediction{
			Path:       symbolRecord.FilePath,
			Confidence: 100.0,
			Reason:     fmt.Sprintf("Symbol '%s' defined here", symbol),
		})
	}

	// Check symbol prediction history
	rows, err := l.db.Query(
		`SELECT file_path, prediction_count, success_count 
		 FROM symbol_predictions 
		 WHERE symbol_name = ? 
		 ORDER BY success_count DESC`,
		symbol,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var path string
			var predCount, successCount int
			if err := rows.Scan(&path, &predCount, &successCount); err == nil {
				confidence := float64(successCount) / float64(predCount) * 100.0
				predictions = append(predictions, Prediction{
					Path:       path,
					Confidence: confidence,
					Reason:     fmt.Sprintf("Historical match for '%s'", symbol),
				})
			}
		}
	}

	return predictions
}

// predictFromPatterns predicts files based on common patterns.
func (l *Loader) predictFromPatterns(task string, allFiles []string) []Prediction {
	var predictions []Prediction

	// Extract patterns from task
	patterns := l.extractPatterns(task)

	for _, pattern := range patterns {
		for _, file := range allFiles {
			score := l.matchPattern(file, pattern)
			if score > 0 {
				predictions = append(predictions, Prediction{
					Path:       file,
					Confidence: score,
					Reason:     fmt.Sprintf("Matches pattern '%s'", pattern),
				})
			}
		}
	}

	return predictions
}

// predictFromHistory predicts files based on historical access patterns.
func (l *Loader) predictFromHistory(task string, allFiles []string) []Prediction {
	var predictions []Prediction

	// Extract task pattern
	taskPattern := l.simplifyTask(task)

	// Query historical patterns
	rows, err := l.db.Query(
		`SELECT file_path, access_count 
		 FROM file_access_patterns 
		 WHERE task_pattern LIKE ? 
		 ORDER BY access_count DESC 
		 LIMIT 10`,
		"%"+taskPattern+"%",
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var path string
			var count int
			if err := rows.Scan(&path, &count); err == nil {
				confidence := min(float64(count)*10.0, 100.0)
				predictions = append(predictions, Prediction{
					Path:       path,
					Confidence: confidence,
					Reason:     "Historical access pattern",
				})
			}
		}
	}

	return predictions
}

// extractSymbols extracts potential symbol names from a task.
func (l *Loader) extractSymbols(task string) []string {
	var symbols []string

	// Common patterns for symbol names
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\b([A-Z][a-zA-Z0-9]+)\b`),                         // CamelCase
		regexp.MustCompile(`\b([a-z]+(?:_[a-z]+)+)\b`),                        // snake_case
		regexp.MustCompile(`(?i)(?:function|method|class|struct|type)\s+(\w+)`), // Definitions
		regexp.MustCompile(`(?i)(?:call|use|import)\s+(\w+)`),                  // Usage
		regexp.MustCompile(`\b([a-z]{5,})\b`),                                  // Meaningful lowercase words
	}

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(task, -1)
		for _, match := range matches {
			if len(match) > 1 {
				symbol := match[1]
				if len(symbol) > 2 && !l.isStopWord(symbol) {
					symbols = append(symbols, symbol)
				}
			}
		}
	}

	return l.deduplicate(symbols)
}

// extractPatterns extracts patterns from a task.
func (l *Loader) extractPatterns(task string) []string {
	var patterns []string
	lower := strings.ToLower(task)

	// Common file type patterns
	if strings.Contains(lower, "auth") || strings.Contains(lower, "login") {
		patterns = append(patterns, "auth", "login", "middleware")
	}
	if strings.Contains(lower, "test") {
		patterns = append(patterns, "_test", "spec")
	}
	if strings.Contains(lower, "api") || strings.Contains(lower, "handler") {
		patterns = append(patterns, "api", "handler", "route")
	}
	if strings.Contains(lower, "db") || strings.Contains(lower, "model") {
		patterns = append(patterns, "db", "model", "store")
	}

	return patterns
}

// matchPattern checks if a file matches a pattern.
func (l *Loader) matchPattern(file string, pattern string) float64 {
	lowerFile := strings.ToLower(file)
	lowerPattern := strings.ToLower(pattern)

	// Filename match (higher priority)
	filename := filepath.Base(lowerFile)
	if strings.Contains(filename, lowerPattern) {
		return 100.0
	}

	// Direct match in path
	if strings.Contains(lowerFile, lowerPattern) {
		return 80.0
	}

	return 0.0
}

// simplifyTask creates a simplified pattern for historical matching.
func (l *Loader) simplifyTask(task string) string {
	// Remove common words and normalize
	stopWords := []string{"the", "a", "an", "is", "are", "was", "were", "be", "been",
		"have", "has", "had", "do", "does", "did", "will", "would", "could", "should"}

	lower := strings.ToLower(task)
	for _, word := range stopWords {
		lower = strings.ReplaceAll(lower, " "+word+" ", " ")
	}

	return strings.TrimSpace(lower)
}

// loadFile loads a file's content.
func (l *Loader) loadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// GetFile retrieves a preloaded file from cache.
func (l *Loader) GetFile(path string) (*FileEntry, bool) {
	entry, ok := l.fileCache[path]
	return entry, ok
}

// RecordAccess records that a file was actually accessed.
func (l *Loader) RecordAccess(task string, filePath string) error {
	if !l.config.EnableLearning {
		return nil
	}

	taskPattern := l.simplifyTask(task)

	// Update access pattern
	_, err := l.db.Exec(
		`INSERT INTO file_access_patterns (task_pattern, file_path, access_count) 
		 VALUES (?, ?, 1)
		 ON CONFLICT(task_pattern, file_path) 
		 DO UPDATE SET access_count = access_count + 1, last_accessed = CURRENT_TIMESTAMP`,
		taskPattern, filePath,
	)

	return err
}

// RecordSuccess records that a prediction was successful.
func (l *Loader) RecordSuccess(symbol string, filePath string) error {
	if !l.config.EnableLearning {
		return nil
	}

	_, err := l.db.Exec(
		`INSERT INTO symbol_predictions (symbol_name, file_path, prediction_count, success_count) 
		 VALUES (?, ?, 1, 1)
		 ON CONFLICT(symbol_name, file_path) 
		 DO UPDATE SET 
		   prediction_count = prediction_count + 1,
		   success_count = success_count + 1`,
		symbol, filePath,
	)

	return err
}

// sortPredictions sorts predictions by confidence (descending).
func (l *Loader) sortPredictions(predictions []Prediction) {
	// Simple bubble sort for small lists
	for i := 0; i < len(predictions); i++ {
		for j := i + 1; j < len(predictions); j++ {
			if predictions[j].Confidence > predictions[i].Confidence {
				predictions[i], predictions[j] = predictions[j], predictions[i]
			}
		}
	}
}

// isStopWord checks if a word is a stop word.
func (l *Loader) isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"day": true, "get": true, "has": true, "him": true, "his": true,
		"how": true, "its": true, "may": true, "new": true, "now": true,
		"old": true, "see": true, "two": true, "who": true, "boy": true,
		"did": true, "use": true,
		"many": true, "oil": true, "sit": true, "set": true, "run": true,
		"eat": true, "far": true, "sea": true, "eye": true, "ago": true,
		"off": true, "too": true, "any": true, "say": true, "man": true,
		"try": true, "ask": true, "end": true, "why": true, "let": true,
		"put": true,
	}
	return stopWords[strings.ToLower(word)]
}

// deduplicate removes duplicates from a slice.
func (l *Loader) deduplicate(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// min returns the minimum of two floats
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Cleanup removes old entries.
func (l *Loader) Cleanup(olderThan time.Time) error {
	_, err := l.db.Exec(
		`DELETE FROM file_access_patterns WHERE last_accessed < ?`,
		olderThan,
	)
	return err
}

// Close closes the loader.
func (l *Loader) Close() error {
	return l.db.Close()
}


