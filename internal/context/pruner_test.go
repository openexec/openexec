package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/memory"
)

func TestPruner(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	// Create knowledge store
	knowledgeStore, err := knowledge.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create knowledge store: %v", err)
	}
	defer knowledgeStore.Close()

	// Create memory manager
	memoryManager, err := memory.NewMemoryManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create memory manager: %v", err)
	}
	defer memoryManager.Close()

	config := DefaultPrunerConfig()
	pruner, err := NewPruner(tmpDir, knowledgeStore, memoryManager, config)
	if err != nil {
		t.Fatalf("failed to create pruner: %v", err)
	}
	defer pruner.Close()

	t.Run("Prune Files", func(t *testing.T) {
		files := []FileInfo{
			{Path: "auth/login.go", Content: "package auth\n\nfunc Login() {}"},
			{Path: "auth/middleware.go", Content: "package auth\n\nfunc AuthMiddleware() {}"},
			{Path: "user/profile.go", Content: "package user\n\nfunc GetProfile() {}"},
			{Path: "api/handlers.go", Content: "package api\n\nfunc HandleRequest() {}"},
			{Path: "db/models.go", Content: "package db\n\ntype User struct{}"},
		}

		query := "auth login middleware"
		result, err := pruner.Prune(files, query)
		if err != nil {
			t.Fatalf("Prune failed: %v", err)
		}

		if result.TotalFiles == 0 {
			t.Error("expected some files to be selected")
		}

		// Check that auth files are ranked higher
		foundAuth := false
		for _, file := range result.Files {
			if strings.Contains(file.Path, "auth") {
				foundAuth = true
				break
			}
		}
		if !foundAuth {
			t.Error("expected auth files to be selected")
		}
	})

	t.Run("Respects Token Budget", func(t *testing.T) {
		// Create files with known token counts
		files := []FileInfo{
			{Path: "file1.go", Content: strings.Repeat("a", 400)},  // ~100 tokens
			{Path: "file2.go", Content: strings.Repeat("b", 400)},  // ~100 tokens
			{Path: "file3.go", Content: strings.Repeat("c", 400)},  // ~100 tokens
			{Path: "file4.go", Content: strings.Repeat("d", 400)},  // ~100 tokens
		}

		config := &PrunerConfig{
			MaxTokens:         250, // Should fit ~2 files
			MaxFiles:          10,
			MinRelevanceScore: 0,
		}

		pruner, _ := NewPruner(tmpDir, nil, nil, config)
		defer pruner.Close()

		result, err := pruner.Prune(files, "test query")
		if err != nil {
			t.Fatalf("Prune failed: %v", err)
		}

		if result.TotalTokens > config.MaxTokens {
			t.Errorf("exceeded token budget: %d > %d", result.TotalTokens, config.MaxTokens)
		}
	})

	t.Run("Respects Max Files", func(t *testing.T) {
		files := []FileInfo{
			{Path: "file1.go", Content: "content1"},
			{Path: "file2.go", Content: "content2"},
			{Path: "file3.go", Content: "content3"},
			{Path: "file4.go", Content: "content4"},
			{Path: "file5.go", Content: "content5"},
		}

		config := &PrunerConfig{
			MaxTokens:         100000,
			MaxFiles:          3,
			MinRelevanceScore: 0,
		}

		pruner, _ := NewPruner(tmpDir, nil, nil, config)
		defer pruner.Close()

		result, err := pruner.Prune(files, "test")
		if err != nil {
			t.Fatalf("Prune failed: %v", err)
		}

		if result.TotalFiles > config.MaxFiles {
			t.Errorf("exceeded max files: %d > %d", result.TotalFiles, config.MaxFiles)
		}
	})

	t.Run("Min Relevance Score", func(t *testing.T) {
		files := []FileInfo{
			{Path: "high.go", Content: "auth login authentication"},
			{Path: "low.go", Content: "random unrelated content"},
		}

		config := &PrunerConfig{
			MaxTokens:         100000,
			MaxFiles:          10,
			MinRelevanceScore: 5.0,
		}

		pruner, _ := NewPruner(tmpDir, nil, nil, config)
		defer pruner.Close()

		result, err := pruner.Prune(files, "auth")
		if err != nil {
			t.Fatalf("Prune failed: %v", err)
		}

		// Should only include high.go
		for _, file := range result.Files {
			if file.Path == "low.go" {
				t.Error("low.go should be filtered out by relevance score")
			}
		}
	})
}

func TestScoreCalculation(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	pruner, _ := NewPruner(tmpDir, nil, nil, DefaultPrunerConfig())
	defer pruner.Close()

	t.Run("Content Scoring", func(t *testing.T) {
		file := FileInfo{
			Path:    "auth.go",
			Content: "package auth\n\nfunc Login() {}\nfunc Logout() {}",
		}
		query := "login auth"
		terms := pruner.extractTerms(query)

		score := pruner.scoreContent(file, query, terms)

		if score <= 0 {
			t.Error("expected positive content score")
		}
	})

	t.Run("Path Scoring", func(t *testing.T) {
		file := FileInfo{
			Path:    "auth/login.go",
			Content: "content",
		}
		query := "auth login"
		terms := pruner.extractTerms(query)

		score := pruner.scorePath(file, query, terms)

		if score <= 0 {
			t.Error("expected positive path score")
		}
	})

	t.Run("Score Breakdown", func(t *testing.T) {
		file := FileInfo{
			Path:    "auth/login.go",
			Content: "package auth\n\nfunc Login() {}",
		}
		query := "login"
		terms := pruner.extractTerms(query)

		_, breakdown := pruner.calculateScore(file, query, terms)

		// Without knowledge store, symbol score should be 0
		if breakdown.SymbolScore != 0 {
			t.Error("expected 0 symbol score without knowledge store")
		}

		// Content and path should have scores
		if breakdown.ContentScore <= 0 {
			t.Error("expected positive content score")
		}
		if breakdown.PathScore <= 0 {
			t.Error("expected positive path score")
		}
	})
}

func TestTermExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	pruner, _ := NewPruner(tmpDir, nil, nil, DefaultPrunerConfig())
	defer pruner.Close()

	tests := []struct {
		query    string
		expected []string
	}{
		{
			"Fix the login bug",
			[]string{"login"},
		},
		{
			"Implement authentication middleware",
			[]string{"implement", "authentication", "middleware"},
		},
		{
			"The quick brown fox",
			[]string{"quick", "brown", "fox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			terms := pruner.extractTerms(tt.query)

			for _, expected := range tt.expected {
				found := false
				for _, term := range terms {
					if term == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected term %q not found in %v", expected, terms)
				}
			}
		})
	}
}

func TestTokenEstimation(t *testing.T) {
	tmpDir := t.TempDir()
	pruner, _ := NewPruner(tmpDir, nil, nil, DefaultPrunerConfig())
	defer pruner.Close()

	tests := []struct {
		content  string
		expected int
	}{
		{"", 0},
		{"abcd", 1},           // 4 chars / 4 = 1
		{"abcdefghij", 2},     // 10 chars / 4 = 2
		{"a b c d", 1},        // 4 non-space chars / 4 = 1
		{"    ", 0},           // Only spaces
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			tokens := pruner.estimateTokens(tt.content)
			if tokens != tt.expected {
				t.Errorf("expected %d tokens, got %d", tt.expected, tokens)
			}
		})
	}
}

func TestPrunerConfig(t *testing.T) {
	config := DefaultPrunerConfig()

	if config.MaxTokens != 100000 {
		t.Errorf("expected MaxTokens 100000, got %d", config.MaxTokens)
	}
	if config.MaxFiles != 20 {
		t.Errorf("expected MaxFiles 20, got %d", config.MaxFiles)
	}
	if config.MinRelevanceScore != 10.0 {
		t.Errorf("expected MinRelevanceScore 10.0, got %f", config.MinRelevanceScore)
	}
	if !config.EnableCaching {
		t.Error("expected EnableCaching to be true")
	}
}

func TestPruneResult(t *testing.T) {
	result := &PruneResult{
		Files: []FileScore{
			{Path: "file1.go", Score: 100, TokenCount: 100},
			{Path: "file2.go", Score: 80, TokenCount: 150},
		},
		TotalTokens:    250,
		TotalFiles:     2,
		OriginalFiles:  10,
		Query:          "test query",
		CacheHit:       false,
		ProcessingTime: 10 * time.Millisecond,
	}

	if result.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", result.TotalFiles)
	}
	if result.OriginalFiles != 10 {
		t.Errorf("expected 10 original files, got %d", result.OriginalFiles)
	}
	if result.TotalTokens != 250 {
		t.Errorf("expected 250 tokens, got %d", result.TotalTokens)
	}
}

func TestFileScore(t *testing.T) {
	score := FileScore{
		Path:       "auth/login.go",
		Content:    "package auth",
		TokenCount: 25,
		Score:      85.5,
		Breakdown: ScoreBreakdown{
			SymbolScore:  10.0,
			ContentScore: 5.0,
			PathScore:    3.0,
			RecencyScore: 2.0,
		},
	}

	if score.Path != "auth/login.go" {
		t.Errorf("expected path 'auth/login.go', got %s", score.Path)
	}
	if score.Score != 85.5 {
		t.Errorf("expected score 85.5, got %f", score.Score)
	}
}
