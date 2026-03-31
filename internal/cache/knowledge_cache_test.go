package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKnowledgeCache(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755); err != nil {
		t.Fatalf("failed to create .openexec dir: %v", err)
	}

	cache, err := NewKnowledgeCache(tmpDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to create knowledge cache: %v", err)
	}
	defer cache.Close()

	t.Run("Set and Get", func(t *testing.T) {
		projectPath := "/project"
		filePath := "src/main.go"
		fileHash := ComputeFileHashString("package main\nfunc main() {}")
		symbols := []byte(`[{"name": "main", "kind": "func"}]`)

		// Act - Set
		err := cache.Set(projectPath, filePath, fileHash, symbols)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Act - Get
		result, err := cache.Get(projectPath, filePath, fileHash)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		// Assert
		if result == nil {
			t.Fatal("expected cache hit, got nil")
		}
		if string(result) != string(symbols) {
			t.Errorf("expected %s, got %s", string(symbols), string(result))
		}
	})

	t.Run("Cache Miss - Different Hash", func(t *testing.T) {
		projectPath := "/project"
		filePath := "src/changed.go"
		oldHash := ComputeFileHashString("old content")
		newHash := ComputeFileHashString("new content")
		symbols := []byte(`[{"name": "old", "kind": "func"}]`)

		// Set with old hash
		err := cache.Set(projectPath, filePath, oldHash, symbols)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Get with new hash (file changed)
		result, err := cache.Get(projectPath, filePath, newHash)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		// Assert - should be nil (cache miss due to hash mismatch)
		if result != nil {
			t.Error("expected cache miss due to hash mismatch, got result")
		}
	})

	t.Run("Cache Miss - Non-existent Entry", func(t *testing.T) {
		result, err := cache.Get("/nonexistent", "file.go", "somehash")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if result != nil {
			t.Error("expected nil for non-existent entry")
		}
	})

	t.Run("Cache Expiration", func(t *testing.T) {
		// Create cache with very short TTL
		shortCache, err := NewKnowledgeCache(tmpDir, 1*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to create short cache: %v", err)
		}
		defer shortCache.Close()

		projectPath := "/project"
		filePath := "src/expiring.go"
		fileHash := ComputeFileHashString("content")
		symbols := []byte(`[{"name": "test"}]`)

		// Set
		err = shortCache.Set(projectPath, filePath, fileHash, symbols)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		// Get - should be expired
		result, err := shortCache.Get(projectPath, filePath, fileHash)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if result != nil {
			t.Error("expected nil for expired entry")
		}
	})

	t.Run("Invalidate", func(t *testing.T) {
		projectPath := "/project"
		filePath := "src/invalidate.go"
		fileHash := ComputeFileHashString("invalidate me")
		symbols := []byte(`[{"name": "invalidate"}]`)

		// Set
		err := cache.Set(projectPath, filePath, fileHash, symbols)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Invalidate
		err = cache.Invalidate(projectPath, filePath)
		if err != nil {
			t.Fatalf("Invalidate failed: %v", err)
		}

		// Get - should be nil
		result, err := cache.Get(projectPath, filePath, fileHash)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if result != nil {
			t.Error("expected nil after invalidate")
		}
	})

	t.Run("Invalidate Project", func(t *testing.T) {
		projectPath := "/invalidate-project"
		
		// Set multiple files
		for i := 0; i < 3; i++ {
			filePath := filepath.Join("src", string(rune('a'+i))+".go")
			fileHash := ComputeFileHashString("content" + string(rune('0'+i)))
			symbols := []byte(`[{"name": "test"}]`)
			
			err := cache.Set(projectPath, filePath, fileHash, symbols)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		// Invalidate entire project
		err := cache.InvalidateProject(projectPath)
		if err != nil {
			t.Fatalf("InvalidateProject failed: %v", err)
		}

		// Verify all files are gone
		for i := 0; i < 3; i++ {
			filePath := filepath.Join("src", string(rune('a'+i))+".go")
			fileHash := ComputeFileHashString("content" + string(rune('0'+i)))
			
			result, err := cache.Get(projectPath, filePath, fileHash)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if result != nil {
				t.Errorf("expected nil for %s after project invalidate", filePath)
			}
		}
	})

	t.Run("Stats", func(t *testing.T) {
		// Add some entries
		for i := 0; i < 5; i++ {
			filePath := filepath.Join("src", string(rune('a'+i))+".go")
			fileHash := ComputeFileHashString("content" + string(rune('0'+i)))
			symbols := []byte(`[{"name": "test"}]`)
			
			err := cache.Set("/stats-project", filePath, fileHash, symbols)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		total, expired, err := cache.Stats()
		if err != nil {
			t.Fatalf("Stats failed: %v", err)
		}

		if total < 5 {
			t.Errorf("expected at least 5 entries, got %d", total)
		}
		if expired != 0 {
			t.Errorf("expected 0 expired, got %d", expired)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		// Create cache with short TTL
		cleanupCache, err := NewKnowledgeCache(tmpDir, 1*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to create cleanup cache: %v", err)
		}
		defer cleanupCache.Close()

		// Add entries
		for i := 0; i < 3; i++ {
			filePath := filepath.Join("cleanup", string(rune('a'+i))+".go")
			fileHash := ComputeFileHashString("content" + string(rune('0'+i)))
			symbols := []byte(`[{"name": "test"}]`)
			
			err := cleanupCache.Set("/cleanup-project", filePath, fileHash, symbols)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		// Cleanup
		err = cleanupCache.Cleanup()
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		// Verify entries are gone
		total, _, err := cleanupCache.Stats()
		if err != nil {
			t.Fatalf("Stats failed: %v", err)
		}

		if total != 0 {
			t.Errorf("expected 0 entries after cleanup, got %d", total)
		}
	})
}

func TestComputeFileHash(t *testing.T) {
	content := []byte("package main\n\nfunc main() {\n\tprintln(\"Hello\")\n}")
	hash1 := ComputeFileHash(content)
	hash2 := ComputeFileHash(content)

	if hash1 != hash2 {
		t.Error("same content should produce same hash")
	}

	differentContent := []byte("different content")
	hash3 := ComputeFileHash(differentContent)

	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}

	if len(hash1) != 64 { // SHA-256 hex is 64 chars
		t.Errorf("expected 64 char hash, got %d", len(hash1))
	}
}

func TestSerializeSymbols(t *testing.T) {
	type Symbol struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}

	symbols := []Symbol{
		{Name: "main", Kind: "func"},
		{Name: "Helper", Kind: "func"},
	}

	// Serialize
	data, err := SerializeSymbols(symbols)
	if err != nil {
		t.Fatalf("SerializeSymbols failed: %v", err)
	}

	// Deserialize
	var result []Symbol
	err = DeserializeSymbols(data, &result)
	if err != nil {
		t.Fatalf("DeserializeSymbols failed: %v", err)
	}

	// Verify
	if len(result) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(result))
	}
	if result[0].Name != "main" {
		t.Errorf("expected 'main', got %s", result[0].Name)
	}
}
