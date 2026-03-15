package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Indexer handles automatic extraction of knowledge from source code.
// Now supports polyglot indexing via the ProviderRegistry.
type Indexer struct {
	store    *Store
	registry *ProviderRegistry
	stats    IndexStats
}

// IndexStats tracks indexing statistics.
type IndexStats struct {
	FilesProcessed  int
	SymbolsExtracted int
	ErrorCount      int
	ByLanguage      map[string]int
}

// Syncer defines the interface for triggering file re-indexing
type Syncer interface {
	SyncFile(filePath string) error
}

// NewIndexer creates an indexer with default provider registry.
func NewIndexer(store *Store) *Indexer {
	return &Indexer{
		store:    store,
		registry: NewProviderRegistry(),
		stats:    IndexStats{ByLanguage: make(map[string]int)},
	}
}

// NewIndexerWithRegistry creates an indexer with a custom provider registry.
func NewIndexerWithRegistry(store *Store, registry *ProviderRegistry) *Indexer {
	return &Indexer{
		store:    store,
		registry: registry,
		stats:    IndexStats{ByLanguage: make(map[string]int)},
	}
}

// GetRegistry returns the provider registry for configuration.
func (idx *Indexer) GetRegistry() *ProviderRegistry {
	return idx.registry
}

// GetStats returns indexing statistics.
func (idx *Indexer) GetStats() IndexStats {
	return idx.stats
}

// ResetStats clears indexing statistics.
func (idx *Indexer) ResetStats() {
	idx.stats = IndexStats{ByLanguage: make(map[string]int)}
}

// IndexProject scans the directory and populates symbol and API records.
// Now uses the polyglot provider registry for multi-language support.
func (idx *Indexer) IndexProject(projectDir string) error {
	idx.ResetStats()

	return filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories, node_modules, vendor, etc.
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") ||
			   name == "node_modules" ||
			   name == "vendor" ||
			   name == "__pycache__" ||
			   name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Try to find a provider for this file
		provider := idx.registry.GetProvider(path)
		if provider == nil {
			return nil // No provider for this file type
		}

		// Index the file using the appropriate provider
		if err := idx.IndexFileWithProvider(path, provider); err != nil {
			idx.stats.ErrorCount++
			// Log but don't fail the entire walk
			return nil
		}

		return nil
	})
}

// IndexFile indexes a single file using the appropriate provider.
func (idx *Indexer) IndexFile(path string) error {
	provider := idx.registry.GetProvider(path)
	if provider == nil {
		return fmt.Errorf("no provider available for file type: %s", filepath.Ext(path))
	}
	return idx.IndexFileWithProvider(path, provider)
}

// IndexFileWithProvider indexes a file using a specific provider.
func (idx *Indexer) IndexFileWithProvider(path string, provider LanguageProvider) error {
	// 1. Extract symbols using the provider
	symbols, err := provider.ExtractSymbols(path)
	if err != nil {
		return fmt.Errorf("failed to extract symbols from %s: %w", path, err)
	}

	// 2. Clear old records for this file to handle Line Drift
	if err := idx.store.DeleteSymbolsByFile(path); err != nil {
		return fmt.Errorf("failed to clear old symbols for %s: %w", path, err)
	}

	// 3. Store the new symbols
	for _, sym := range symbols {
		record := &SymbolRecord{
			Name:      sym.Name,
			Kind:      string(sym.Kind),
			FilePath:  sym.FilePath,
			StartLine: sym.StartLine,
			EndLine:   sym.EndLine,
			Signature: sym.Signature,
			Purpose:   sym.DocComment,
		}
		if err := idx.store.SetSymbol(record); err != nil {
			return fmt.Errorf("failed to store symbol %s: %w", sym.Name, err)
		}
		idx.stats.SymbolsExtracted++
		idx.stats.ByLanguage[sym.Language]++
	}

	idx.stats.FilesProcessed++
	return nil
}

// SupportedLanguages returns the list of languages the indexer can process.
func (idx *Indexer) SupportedLanguages() []string {
	return idx.registry.SupportedLanguages()
}

// EnableLanguage enables indexing for a specific language.
func (idx *Indexer) EnableLanguage(name string) {
	idx.registry.EnableLanguage(name)
}

// DisableLanguage disables indexing for a specific language.
func (idx *Indexer) DisableLanguage(name string) {
	idx.registry.DisableLanguage(name)
}
