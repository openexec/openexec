// Package memory provides a layered memory system for OpenExec.
// It implements a hierarchy of memory files similar to CLAUDE.md but with
// better organization and auto-extraction capabilities.
package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MemoryLayer represents a layer in the memory hierarchy.
// Later layers override earlier ones.
type MemoryLayer int

const (
	// LayerManaged is organization-wide memory (/etc/openexec/)
	LayerManaged MemoryLayer = iota
	// LayerUser is user-specific memory (~/.openexec/)
	LayerUser
	// LayerProject is project-specific memory (./.openexec/)
	LayerProject
	// LayerLocal is private local memory (OPENEXEC.local.md)
	LayerLocal
)

func (l MemoryLayer) String() string {
	switch l {
	case LayerManaged:
		return "managed"
	case LayerUser:
		return "user"
	case LayerProject:
		return "project"
	case LayerLocal:
		return "local"
	default:
		return "unknown"
	}
}

// MemoryFile represents a single memory file at a specific layer.
type MemoryFile struct {
	Layer    MemoryLayer `json:"layer"`
	Path     string      `json:"path"`
	Content  string      `json:"content"`
	Modified time.Time   `json:"modified"`
}

// MemoryEntry represents a single memory entry extracted from files.
type MemoryEntry struct {
	Category    string    `json:"category"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Source      string    `json:"source"`
	Layer       string    `json:"layer"`
	ExtractedAt time.Time `json:"extracted_at"`
}

// MemorySystem manages the layered memory hierarchy.
type MemorySystem struct {
	projectDir string
	userDir    string
	managedDir string
}

// NewMemorySystem creates a new memory system for a project.
func NewMemorySystem(projectDir string) *MemorySystem {
	homeDir, _ := os.UserHomeDir()
	return &MemorySystem{
		projectDir: projectDir,
		userDir:    filepath.Join(homeDir, ".openexec"),
		managedDir: "/etc/openexec",
	}
}

// Load loads all memory files in priority order (later overrides earlier).
func (ms *MemorySystem) Load() ([]*MemoryFile, error) {
	var files []*MemoryFile

	// Load in priority order
	loaders := []struct {
		layer MemoryLayer
		path  string
	}{
		{LayerManaged, filepath.Join(ms.managedDir, "MEMORY.md")},
		{LayerUser, filepath.Join(ms.userDir, "MEMORY.md")},
		{LayerProject, filepath.Join(ms.projectDir, ".openexec", "MEMORY.md")},
		{LayerLocal, filepath.Join(ms.projectDir, "OPENEXEC.local.md")},
	}

	for _, loader := range loaders {
		content, modified, err := ms.readFile(loader.path)
		if err != nil {
			// File doesn't exist is OK
			continue
		}

		files = append(files, &MemoryFile{
			Layer:    loader.layer,
			Path:     loader.path,
			Content:  content,
			Modified: modified,
		})
	}

	return files, nil
}

// LoadMerged loads and merges all memory files into a single content string.
// Later layers override/append to earlier layers.
func (ms *MemorySystem) LoadMerged() (string, error) {
	files, err := ms.Load()
	if err != nil {
		return "", err
	}

	var sections []string
	for _, file := range files {
		sections = append(sections, ms.formatSection(file))
	}

	return strings.Join(sections, "\n\n---\n\n"), nil
}

// GetLayer retrieves memory from a specific layer.
func (ms *MemorySystem) GetLayer(layer MemoryLayer) (*MemoryFile, error) {
	var path string
	switch layer {
	case LayerManaged:
		path = filepath.Join(ms.managedDir, "MEMORY.md")
	case LayerUser:
		path = filepath.Join(ms.userDir, "MEMORY.md")
	case LayerProject:
		path = filepath.Join(ms.projectDir, ".openexec", "MEMORY.md")
	case LayerLocal:
		path = filepath.Join(ms.projectDir, "OPENEXEC.local.md")
	default:
		return nil, fmt.Errorf("unknown memory layer: %v", layer)
	}

	content, modified, err := ms.readFile(path)
	if err != nil {
		return nil, err
	}

	return &MemoryFile{
		Layer:    layer,
		Path:     path,
		Content:  content,
		Modified: modified,
	}, nil
}

// Write writes memory to a specific layer.
func (ms *MemorySystem) Write(layer MemoryLayer, content string) error {
	var path string
	switch layer {
	case LayerManaged:
		path = filepath.Join(ms.managedDir, "MEMORY.md")
	case LayerUser:
		path = filepath.Join(ms.userDir, "MEMORY.md")
	case LayerProject:
		path = filepath.Join(ms.projectDir, ".openexec", "MEMORY.md")
	case LayerLocal:
		path = filepath.Join(ms.projectDir, "OPENEXEC.local.md")
	default:
		return fmt.Errorf("unknown memory layer: %v", layer)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write memory file %s: %w", path, err)
	}

	return nil
}

// Append appends content to a specific layer.
func (ms *MemorySystem) Append(layer MemoryLayer, content string) error {
	existing, err := ms.GetLayer(layer)
	if err != nil {
		// File doesn't exist, just write
		return ms.Write(layer, content)
	}

	newContent := existing.Content + "\n\n" + content
	return ms.Write(layer, newContent)
}

// ExtractEntries extracts structured memory entries from all layers.
func (ms *MemorySystem) ExtractEntries() ([]*MemoryEntry, error) {
	files, err := ms.Load()
	if err != nil {
		return nil, err
	}

	var entries []*MemoryEntry
	for _, file := range files {
		fileEntries := ms.parseEntries(file)
		entries = append(entries, fileEntries...)
	}

	return entries, nil
}

// ExtractByCategory extracts entries filtered by category.
func (ms *MemorySystem) ExtractByCategory(category string) ([]*MemoryEntry, error) {
	entries, err := ms.ExtractEntries()
	if err != nil {
		return nil, err
	}

	var filtered []*MemoryEntry
	for _, entry := range entries {
		if strings.EqualFold(entry.Category, category) {
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// ExtractPatterns extracts common patterns from memory.
func (ms *MemorySystem) ExtractPatterns() ([]*MemoryEntry, error) {
	return ms.ExtractByCategory("pattern")
}

// ExtractDecisions extracts architectural decisions from memory.
func (ms *MemorySystem) ExtractDecisions() ([]*MemoryEntry, error) {
	return ms.ExtractByCategory("decision")
}

// ExtractPreferences extracts user preferences from memory.
func (ms *MemorySystem) ExtractPreferences() ([]*MemoryEntry, error) {
	return ms.ExtractByCategory("preference")
}

// AutoExtract analyzes a session and extracts memories automatically.
func (ms *MemorySystem) AutoExtract(session Session) ([]*MemoryEntry, error) {
	var entries []*MemoryEntry

	// Extract decisions
	for _, decision := range session.Decisions {
		entries = append(entries, &MemoryEntry{
			Category:    "decision",
			Key:         decision.Topic,
			Value:       decision.Rationale,
			Source:      "auto-extract",
			Layer:       LayerProject.String(),
			ExtractedAt: time.Now().UTC(),
		})
	}

	// Extract patterns
	for _, pattern := range session.Patterns {
		entries = append(entries, &MemoryEntry{
			Category:    "pattern",
			Key:         pattern.Name,
			Value:       pattern.Description,
			Source:      "auto-extract",
			Layer:       LayerProject.String(),
			ExtractedAt: time.Now().UTC(),
		})
	}

	// Extract preferences
	for _, pref := range session.Preferences {
		entries = append(entries, &MemoryEntry{
			Category:    "preference",
			Key:         pref.Key,
			Value:       pref.Value,
			Source:      "auto-extract",
			Layer:       LayerUser.String(),
			ExtractedAt: time.Now().UTC(),
		})
	}

	return entries, nil
}

// SaveEntries saves extracted entries to the appropriate layer.
func (ms *MemorySystem) SaveEntries(entries []*MemoryEntry, layer MemoryLayer) error {
	var sections []string

	// Group by category
	byCategory := make(map[string][]*MemoryEntry)
	for _, entry := range entries {
		byCategory[entry.Category] = append(byCategory[entry.Category], entry)
	}

	// Build content
	for category, catEntries := range byCategory {
		sections = append(sections, fmt.Sprintf("## %s\n", strings.Title(category)))
		for _, entry := range catEntries {
			sections = append(sections, fmt.Sprintf("### %s\n%s\n", entry.Key, entry.Value))
		}
	}

	content := strings.Join(sections, "\n")
	return ms.Append(layer, content)
}

// Search searches all memory layers for a query.
func (ms *MemorySystem) Search(query string) ([]*MemoryEntry, error) {
	entries, err := ms.ExtractEntries()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var matches []*MemoryEntry

	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Key), query) ||
			strings.Contains(strings.ToLower(entry.Value), query) ||
			strings.Contains(strings.ToLower(entry.Category), query) {
			matches = append(matches, entry)
		}
	}

	return matches, nil
}

// readFile reads a file and returns its content and modification time.
func (ms *MemorySystem) readFile(path string) (string, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, err
	}

	return string(content), info.ModTime(), nil
}

// formatSection formats a memory file as a section.
func (ms *MemorySystem) formatSection(file *MemoryFile) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<!-- Layer: %s -->\n", file.Layer))
	sb.WriteString(fmt.Sprintf("<!-- Source: %s -->\n", file.Path))
	sb.WriteString(fmt.Sprintf("<!-- Modified: %s -->\n", file.Modified.Format(time.RFC3339)))
	sb.WriteString(file.Content)
	return sb.String()
}

// parseEntries parses memory entries from a file.
func (ms *MemorySystem) parseEntries(file *MemoryFile) []*MemoryEntry {
	var entries []*MemoryEntry
	
	// Simple parsing: look for ## Category and ### Key patterns
	categoryPattern := regexp.MustCompile(`^##\s+(.+)$`)
	keyPattern := regexp.MustCompile(`^###\s+(.+)$`)
	
	var currentCategory string
	var currentKey string
	var currentValue []string
	
	scanner := bufio.NewScanner(strings.NewReader(file.Content))
	for scanner.Scan() {
		line := scanner.Text()
		
		// Check for category
		if matches := categoryPattern.FindStringSubmatch(line); matches != nil {
			// Save previous entry if exists
			if currentKey != "" && len(currentValue) > 0 {
				entries = append(entries, &MemoryEntry{
					Category:    strings.ToLower(strings.TrimSpace(currentCategory)),
					Key:         strings.TrimSpace(currentKey),
					Value:       strings.TrimSpace(strings.Join(currentValue, "\n")),
					Source:      file.Path,
					Layer:       file.Layer.String(),
					ExtractedAt: time.Now().UTC(),
				})
			}
			currentCategory = matches[1]
			currentKey = ""
			currentValue = nil
			continue
		}
		
		// Check for key
		if matches := keyPattern.FindStringSubmatch(line); matches != nil {
			// Save previous entry if exists
			if currentKey != "" && len(currentValue) > 0 {
				entries = append(entries, &MemoryEntry{
					Category:    strings.ToLower(strings.TrimSpace(currentCategory)),
					Key:         strings.TrimSpace(currentKey),
					Value:       strings.TrimSpace(strings.Join(currentValue, "\n")),
					Source:      file.Path,
					Layer:       file.Layer.String(),
					ExtractedAt: time.Now().UTC(),
				})
			}
			currentKey = matches[1]
			currentValue = nil
			continue
		}
		
		// Accumulate value
		if currentKey != "" {
			currentValue = append(currentValue, line)
		}
	}
	
	// Save last entry
	if currentKey != "" && len(currentValue) > 0 {
		entries = append(entries, &MemoryEntry{
			Category:    strings.ToLower(strings.TrimSpace(currentCategory)),
			Key:         strings.TrimSpace(currentKey),
			Value:       strings.TrimSpace(strings.Join(currentValue, "\n")),
			Source:      file.Path,
			Layer:       file.Layer.String(),
			ExtractedAt: time.Now().UTC(),
		})
	}
	
	return entries
}

// Session represents a conversation/session for auto-extraction.
type Session struct {
	ID          string
	Decisions   []Decision
	Patterns    []Pattern
	Preferences []Preference
	StartTime   time.Time
	EndTime     time.Time
}

// Decision represents an architectural decision.
type Decision struct {
	Topic     string
	Decision  string
	Rationale string
	Timestamp time.Time
}

// Pattern represents a learned pattern.
type Pattern struct {
	Name        string
	Description string
	Examples    []string
}

// Preference represents a user preference.
type Preference struct {
	Key   string
	Value string
}
