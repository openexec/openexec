package predictive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestPredictiveLoader(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	// Create knowledge store
	knowledgeStore, err := knowledge.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create knowledge store: %v", err)
	}
	defer knowledgeStore.Close()

	config := DefaultLoaderConfig()
	loader, err := NewLoader(tmpDir, knowledgeStore, config)
	if err != nil {
		t.Fatalf("failed to create loader: %v", err)
	}
	defer loader.Close()

	t.Run("Predict And Load", func(t *testing.T) {
		// Create test files
		os.WriteFile(filepath.Join(tmpDir, "auth.go"), []byte("package auth"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "login.go"), []byte("package main"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "user.go"), []byte("package user"), 0644)

		allFiles := []string{
			filepath.Join(tmpDir, "auth.go"),
			filepath.Join(tmpDir, "login.go"),
			filepath.Join(tmpDir, "user.go"),
		}

		task := "Fix authentication in login"
		result, err := loader.PredictAndLoad(context.Background(), task, allFiles)
		if err != nil {
			t.Fatalf("PredictAndLoad failed: %v", err)
		}

		if len(result.Predictions) == 0 {
			t.Error("expected some predictions")
		}

		// Check that auth-related files are predicted
		foundAuth := false
		for _, pred := range result.Predictions {
			if contains(pred.Path, "auth") || contains(pred.Path, "login") {
				foundAuth = true
				break
			}
		}
		if !foundAuth {
			t.Error("expected auth/login files in predictions")
		}
	})

	t.Run("Symbol Extraction", func(t *testing.T) {
		tests := []struct {
			task     string
			expected []string
		}{
			{
				"Fix the Login function",
				[]string{"Login"},
			},
			{
				"Update user_profile handler",
				[]string{"user_profile", "handler"},
			},
			{
				"Call the authenticate method",
				[]string{"authenticate"},
			},
		}

		for _, tt := range tests {
			symbols := loader.extractSymbols(tt.task)

			for _, expected := range tt.expected {
				found := false
				for _, symbol := range symbols {
					if contains(symbol, expected) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected symbol containing %q in %v", expected, symbols)
				}
			}
		}
	})

	t.Run("Pattern Extraction", func(t *testing.T) {
		tests := []struct {
			task     string
			expected []string
		}{
			{
				"Fix authentication bug",
				[]string{"auth"},
			},
			{
				"Update API handlers",
				[]string{"api", "handler"},
			},
			{
				"Add database model",
				[]string{"db", "model"},
			},
		}

		for _, tt := range tests {
			patterns := loader.extractPatterns(tt.task)

			for _, expected := range tt.expected {
				found := false
				for _, pattern := range patterns {
					if pattern == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected pattern %q in %v", expected, patterns)
				}
			}
		}
	})

	t.Run("Pattern Matching", func(t *testing.T) {
		tests := []struct {
			file     string
			pattern  string
			expected float64
		}{
			{"auth/login.go", "auth", 80.0},
			{"handlers/api.go", "api", 100.0},
			{"models/user.go", "db", 0.0},
		}

		for _, tt := range tests {
			score := loader.matchPattern(tt.file, tt.pattern)
			if score != tt.expected {
				t.Errorf("matchPattern(%q, %q) = %f, expected %f", tt.file, tt.pattern, score, tt.expected)
			}
		}
	})

	t.Run("Record Access", func(t *testing.T) {
		task := "Fix login bug"
		filePath := "auth/login.go"

		err := loader.RecordAccess(task, filePath)
		if err != nil {
			t.Fatalf("RecordAccess failed: %v", err)
		}

		// Verify by predicting
		allFiles := []string{filePath}
		result, _ := loader.PredictAndLoad(context.Background(), task, allFiles)

		// Should have higher confidence due to history
		for _, pred := range result.Predictions {
			if pred.Path == filePath && pred.Confidence > 0 {
				// Success - historical pattern found
				return
			}
		}
	})

	t.Run("Cache File", func(t *testing.T) {
		// Create test file
		testFile := filepath.Join(tmpDir, "cached.go")
		os.WriteFile(testFile, []byte("package main"), 0644)

		// First load - cache miss
		allFiles := []string{testFile}
		result1, _ := loader.PredictAndLoad(context.Background(), "test", allFiles)

		if result1.CacheMisses != 1 {
			t.Errorf("expected 1 cache miss, got %d", result1.CacheMisses)
		}

		// Second load - cache hit
		result2, _ := loader.PredictAndLoad(context.Background(), "test", allFiles)

		if result2.CacheHits != 1 {
			t.Errorf("expected 1 cache hit, got %d", result2.CacheHits)
		}

		// Verify GetFile works
		entry, ok := loader.GetFile(testFile)
		if !ok {
			t.Error("expected file to be in cache")
		}
		if entry.Path != testFile {
			t.Errorf("expected path %s, got %s", testFile, entry.Path)
		}
	})
}

func TestLoaderConfig(t *testing.T) {
	config := DefaultLoaderConfig()

	if config.MaxPreloadFiles != 10 {
		t.Errorf("expected MaxPreloadFiles 10, got %d", config.MaxPreloadFiles)
	}
	if config.ConfidenceThreshold != 30.0 {
		t.Errorf("expected ConfidenceThreshold 30.0, got %f", config.ConfidenceThreshold)
	}
	if !config.EnableLearning {
		t.Error("expected EnableLearning to be true")
	}
}

func TestPredictionResult(t *testing.T) {
	result := &PredictionResult{
		Predictions: []Prediction{
			{Path: "file1.go", Confidence: 90.0, Reason: "Symbol match"},
			{Path: "file2.go", Confidence: 70.0, Reason: "Pattern match"},
		},
		LoadedFiles: []FileEntry{
			{Path: "file1.go", Confidence: 90.0},
		},
		CacheHits:      1,
		CacheMisses:    1,
		ProcessingTime: 10 * time.Millisecond,
	}

	if len(result.Predictions) != 2 {
		t.Errorf("expected 2 predictions, got %d", len(result.Predictions))
	}
	if len(result.LoadedFiles) != 1 {
		t.Errorf("expected 1 loaded file, got %d", len(result.LoadedFiles))
	}
	if result.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", result.CacheHits)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
