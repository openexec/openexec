package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemorySystem(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	system := NewMemorySystem(tmpDir)

	t.Run("Write and Get Layer", func(t *testing.T) {
		content := "# Project Memory\n\nThis is project-specific context."
		
		err := system.Write(LayerProject, content)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		file, err := system.GetLayer(LayerProject)
		if err != nil {
			t.Fatalf("GetLayer failed: %v", err)
		}

		if !strings.Contains(file.Content, "Project Memory") {
			t.Error("expected content to contain 'Project Memory'")
		}
		if file.Layer != LayerProject {
			t.Errorf("expected layer %v, got %v", LayerProject, file.Layer)
		}
	})

	t.Run("Append to Layer", func(t *testing.T) {
		// First write
		err := system.Write(LayerProject, "# Initial")
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Append
		err = system.Append(LayerProject, "\n\n## New Section\nMore content.")
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}

		// Verify
		file, err := system.GetLayer(LayerProject)
		if err != nil {
			t.Fatalf("GetLayer failed: %v", err)
		}

		if !strings.Contains(file.Content, "Initial") {
			t.Error("expected content to contain 'Initial'")
		}
		if !strings.Contains(file.Content, "New Section") {
			t.Error("expected content to contain 'New Section'")
		}
	})

	t.Run("Load All Layers", func(t *testing.T) {
		// Write to multiple layers
		system.Write(LayerProject, "# Project")
		system.Write(LayerLocal, "# Local")

		// Load all
		files, err := system.Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if len(files) < 2 {
			t.Errorf("expected at least 2 files, got %d", len(files))
		}
	})

	t.Run("Load Merged", func(t *testing.T) {
		// Write to multiple layers
		system.Write(LayerProject, "# Project Content")
		system.Write(LayerLocal, "# Local Content")

		// Load merged
		merged, err := system.LoadMerged()
		if err != nil {
			t.Fatalf("LoadMerged failed: %v", err)
		}

		if !strings.Contains(merged, "Project Content") {
			t.Error("expected merged content to contain 'Project Content'")
		}
		if !strings.Contains(merged, "Local Content") {
			t.Error("expected merged content to contain 'Local Content'")
		}
	})

	t.Run("Parse Entries", func(t *testing.T) {
		content := `# Memory

## Decisions

### Use SQLite
We chose SQLite for simplicity.

### Use Go
Go provides great performance.

## Patterns

### Error Handling
Always wrap errors with context.
`

		system.Write(LayerProject, content)
		file, _ := system.GetLayer(LayerProject)
		entries := system.parseEntries(file)

		if len(entries) < 2 {
			t.Errorf("expected at least 2 entries, got %d", len(entries))
		}

		// Check for decision entry
		foundDecision := false
		for _, entry := range entries {
			if entry.Category == "decisions" && entry.Key == "Use SQLite" {
				foundDecision = true
				break
			}
		}
		if !foundDecision {
			t.Error("expected to find 'Use SQLite' decision entry")
		}
	})

	t.Run("Extract By Category", func(t *testing.T) {
		content := `## Decisions

### Decision 1
Content 1

## Patterns

### Pattern 1
Content 2
`

		system.Write(LayerProject, content)

		decisions, err := system.ExtractByCategory("decisions")
		if err != nil {
			t.Fatalf("ExtractByCategory failed: %v", err)
		}

		if len(decisions) != 1 {
			t.Errorf("expected 1 decision, got %d", len(decisions))
		}
	})

	t.Run("Search", func(t *testing.T) {
		content := `## Decisions

### Use PostgreSQL
We chose PostgreSQL for production.

### Use SQLite
We chose SQLite for local development.
`

		system.Write(LayerProject, content)

		results, err := system.Search("SQLite")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		found := false
		for _, entry := range results {
			if strings.Contains(entry.Key, "SQLite") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find SQLite in search results")
		}
	})

	t.Run("Auto Extract", func(t *testing.T) {
		session := Session{
			ID:        "test-session",
			StartTime: time.Now().UTC(),
			Decisions: []Decision{
				{
					Topic:     "Database",
					Decision:  "Use SQLite",
					Rationale: "Simple and fast",
					Timestamp: time.Now().UTC(),
				},
			},
			Patterns: []Pattern{
				{
					Name:        "Error Wrapping",
					Description: "Always wrap errors",
				},
			},
			Preferences: []Preference{
				{
					Key:   "style",
					Value: "compact",
				},
			},
		}

		entries, err := system.AutoExtract(session)
		if err != nil {
			t.Fatalf("AutoExtract failed: %v", err)
		}

		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}

		// Verify decision
		foundDecision := false
		for _, entry := range entries {
			if entry.Category == "decision" && entry.Key == "Database" {
				foundDecision = true
				break
			}
		}
		if !foundDecision {
			t.Error("expected to find decision entry")
		}
	})
}

func TestMemoryLayers(t *testing.T) {
	tests := []struct {
		layer    MemoryLayer
		expected string
	}{
		{LayerManaged, "managed"},
		{LayerUser, "user"},
		{LayerProject, "project"},
		{LayerLocal, "local"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.layer.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.layer.String())
			}
		})
	}
}

func TestMemoryEntry(t *testing.T) {
	entry := &MemoryEntry{
		Category:    "decision",
		Key:         "Database Choice",
		Value:       "We use SQLite",
		Source:      "meeting-notes.md",
		Layer:       "project",
		ExtractedAt: time.Now().UTC(),
	}

	if entry.Category != "decision" {
		t.Errorf("expected category 'decision', got %s", entry.Category)
	}
	if entry.Key != "Database Choice" {
		t.Errorf("expected key 'Database Choice', got %s", entry.Key)
	}
}

func TestSessionTypes(t *testing.T) {
	t.Run("Decision", func(t *testing.T) {
		d := Decision{
			Topic:     "Architecture",
			Decision:  "Use microservices",
			Rationale: "Scalability",
			Timestamp: time.Now().UTC(),
		}

		if d.Topic != "Architecture" {
			t.Errorf("expected topic 'Architecture', got %s", d.Topic)
		}
	})

	t.Run("Pattern", func(t *testing.T) {
		p := Pattern{
			Name:        "Repository Pattern",
			Description: "Abstract data access",
			Examples:    []string{"user_repo.go"},
		}

		if p.Name != "Repository Pattern" {
			t.Errorf("expected name 'Repository Pattern', got %s", p.Name)
		}
	})

	t.Run("Preference", func(t *testing.T) {
		p := Preference{
			Key:   "theme",
			Value: "dark",
		}

		if p.Key != "theme" {
			t.Errorf("expected key 'theme', got %s", p.Key)
		}
	})
}
