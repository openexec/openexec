package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryManager(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	manager, err := NewMemoryManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create memory manager: %v", err)
	}
	defer manager.Close()

	t.Run("Store and Get Entry", func(t *testing.T) {
		entry := &MemoryEntry{
			Category:    "decision",
			Key:         "Use Go",
			Value:       "Go provides excellent performance",
			Source:      "architecture-review",
			Layer:       "project",
			ExtractedAt: time.Now().UTC(),
		}

		err := manager.StoreEntry(entry)
		if err != nil {
			t.Fatalf("StoreEntry failed: %v", err)
		}

		retrieved, err := manager.GetEntry("decision", "Use Go")
		if err != nil {
			t.Fatalf("GetEntry failed: %v", err)
		}

		if retrieved == nil {
			t.Fatal("expected entry, got nil")
		}
		if retrieved.Value != entry.Value {
			t.Errorf("expected value %s, got %s", entry.Value, retrieved.Value)
		}
	})

	t.Run("Store Multiple Entries", func(t *testing.T) {
		entries := []*MemoryEntry{
			{
				Category:    "pattern",
				Key:         "Error Wrapping",
				Value:       "Always wrap errors",
				Layer:       "project",
				ExtractedAt: time.Now().UTC(),
			},
			{
				Category:    "pattern",
				Key:         "Context Passing",
				Value:       "Pass context as first param",
				Layer:       "project",
				ExtractedAt: time.Now().UTC(),
			},
		}

		err := manager.StoreEntries(entries)
		if err != nil {
			t.Fatalf("StoreEntries failed: %v", err)
		}

		// Verify both stored
		list, err := manager.ListEntries("pattern")
		if err != nil {
			t.Fatalf("ListEntries failed: %v", err)
		}

		if len(list) < 2 {
			t.Errorf("expected at least 2 patterns, got %d", len(list))
		}
	})

	t.Run("List Entries By Category", func(t *testing.T) {
		// Store entries in different categories
		manager.StoreEntry(&MemoryEntry{
			Category:    "decision",
			Key:         "Decision 1",
			Value:       "Value 1",
			Layer:       "project",
			ExtractedAt: time.Now().UTC(),
		})
		manager.StoreEntry(&MemoryEntry{
			Category:    "preference",
			Key:         "Preference 1",
			Value:       "Value 2",
			Layer:       "user",
			ExtractedAt: time.Now().UTC(),
		})

		// List only decisions
		decisions, err := manager.ListEntries("decision")
		if err != nil {
			t.Fatalf("ListEntries failed: %v", err)
		}

		for _, entry := range decisions {
			if entry.Category != "decision" {
				t.Errorf("expected only decision entries, got %s", entry.Category)
			}
		}
	})

	t.Run("Search Entries", func(t *testing.T) {
		// Store searchable entries
		manager.StoreEntry(&MemoryEntry{
			Category:    "decision",
			Key:         "Database Choice",
			Value:       "We chose PostgreSQL for production",
			Layer:       "project",
			ExtractedAt: time.Now().UTC(),
		})
		manager.StoreEntry(&MemoryEntry{
			Category:    "decision",
			Key:         "Cache Choice",
			Value:       "We chose Redis for caching",
			Layer:       "project",
			ExtractedAt: time.Now().UTC(),
		})

		// Search for PostgreSQL
		results, err := manager.Search("PostgreSQL")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		found := false
		for _, entry := range results {
			if entry.Key == "Database Choice" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find 'Database Choice' in search results")
		}
	})

	t.Run("Delete Entry", func(t *testing.T) {
		// Store entry
		manager.StoreEntry(&MemoryEntry{
			Category:    "temp",
			Key:         "To Be Deleted",
			Value:       "Delete me",
			Layer:       "project",
			ExtractedAt: time.Now().UTC(),
		})

		// Verify exists
		entry, _ := manager.GetEntry("temp", "To Be Deleted")
		if entry == nil {
			t.Fatal("entry should exist before deletion")
		}

		// Delete
		err := manager.DeleteEntry("temp", "To Be Deleted")
		if err != nil {
			t.Fatalf("DeleteEntry failed: %v", err)
		}

		// Verify deleted
		entry, _ = manager.GetEntry("temp", "To Be Deleted")
		if entry != nil {
			t.Error("entry should be deleted")
		}
	})

	t.Run("Record and Extract Session", func(t *testing.T) {
		session := &Session{
			ID:        "session-1",
			StartTime: time.Now().UTC().Add(-1 * time.Hour),
			EndTime:   time.Now().UTC(),
			Decisions: []Decision{
				{
					Topic:     "Architecture",
					Decision:  "Use microservices",
					Rationale: "Better scalability",
					Timestamp: time.Now().UTC(),
				},
			},
			Patterns: []Pattern{
				{
					Name:        "Circuit Breaker",
					Description: "Prevent cascade failures",
				},
			},
			Preferences: []Preference{
				{
					Key:   "logging",
					Value: "structured",
				},
			},
		}

		// Record session
		err := manager.RecordSession(session)
		if err != nil {
			t.Fatalf("RecordSession failed: %v", err)
		}

		// Extract from session
		entries, err := manager.ExtractFromSession(session.ID)
		if err != nil {
			t.Fatalf("ExtractFromSession failed: %v", err)
		}

		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}

		// Verify decision extracted
		foundDecision := false
		for _, entry := range entries {
			if entry.Category == "decision" && entry.Key == "Architecture" {
				foundDecision = true
				break
			}
		}
		if !foundDecision {
			t.Error("expected to find architecture decision")
		}
	})

	t.Run("Get Stats", func(t *testing.T) {
		// Store some entries
		for i := 0; i < 5; i++ {
			manager.StoreEntry(&MemoryEntry{
				Category:    "test",
				Key:         string(rune('A' + i)),
				Value:       "value",
				Layer:       "project",
				ExtractedAt: time.Now().UTC(),
			})
		}

		stats, err := manager.GetStats()
		if err != nil {
			t.Fatalf("GetStats failed: %v", err)
		}

		if stats.TotalEntries < 5 {
			t.Errorf("expected at least 5 entries, got %d", stats.TotalEntries)
		}
		if stats.ByCategory["test"] < 5 {
			t.Errorf("expected at least 5 test entries, got %d", stats.ByCategory["test"])
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		// Store old entry
		oldEntry := &MemoryEntry{
			Category:    "old",
			Key:         "Old Entry",
			Value:       "Should be cleaned up",
			Layer:       "project",
			ExtractedAt: time.Now().UTC().Add(-48 * time.Hour),
		}
		manager.StoreEntry(oldEntry)

		// Store recent entry
		recentEntry := &MemoryEntry{
			Category:    "recent",
			Key:         "Recent Entry",
			Value:       "Should remain",
			Layer:       "project",
			ExtractedAt: time.Now().UTC(),
		}
		manager.StoreEntry(recentEntry)

		// Cleanup entries older than 24 hours
		cutoff := time.Now().UTC().Add(-24 * time.Hour)
		err := manager.Cleanup(cutoff)
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		// Verify old entry is gone
		old, _ := manager.GetEntry("old", "Old Entry")
		if old != nil {
			t.Error("old entry should be cleaned up")
		}

		// Verify recent entry remains
		recent, _ := manager.GetEntry("recent", "Recent Entry")
		if recent == nil {
			t.Error("recent entry should remain")
		}
	})

	t.Run("Load Context", func(t *testing.T) {
		// Write to memory file
		system := NewMemorySystem(tmpDir)
		system.Write(LayerProject, "# Project Context\n\nImportant info.")

		// Load context through manager
		context, err := manager.LoadContext()
		if err != nil {
			t.Fatalf("LoadContext failed: %v", err)
		}

		if context == "" {
			t.Error("expected non-empty context")
		}
	})
}

func TestMemoryStats(t *testing.T) {
	stats := &MemoryStats{
		TotalEntries:  10,
		ByCategory:    map[string]int{"decision": 5, "pattern": 5},
		ByLayer:       map[string]int{"project": 8, "user": 2},
		TotalSessions: 3,
	}

	if stats.TotalEntries != 10 {
		t.Errorf("expected 10 total entries, got %d", stats.TotalEntries)
	}
	if stats.ByCategory["decision"] != 5 {
		t.Errorf("expected 5 decisions, got %d", stats.ByCategory["decision"])
	}
	if stats.ByLayer["project"] != 8 {
		t.Errorf("expected 8 project entries, got %d", stats.ByLayer["project"])
	}
	if stats.TotalSessions != 3 {
		t.Errorf("expected 3 sessions, got %d", stats.TotalSessions)
	}
}
