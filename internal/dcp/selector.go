package dcp

import (
	"context"
	"sort"
	"strings"

	"github.com/openexec/openexec/internal/tools"
)

// ToolScore represents a tool's relevance score for a given intent.
type ToolScore struct {
	Tool       tools.Tool
	Score      float64 // 0.0 to 1.0
	Reason     string  // Why this tool was ranked this way
	Categories []string // Categories this tool belongs to
}

// Selector provides tool ranking and selection based on intent.
// Unlike the Coordinator which routes to a single tool, the Selector
// returns a ranked list of potentially relevant tools for MCP to present.
type Selector struct {
	tools      map[string]tools.Tool
	categories map[string][]string // tool name -> categories
}

// SelectorConfig configures the Selector.
type SelectorConfig struct {
	// MaxResults limits the number of tools returned.
	MaxResults int
	// MinScore filters tools below this threshold.
	MinScore float64
}

// DefaultSelectorConfig returns sensible defaults.
func DefaultSelectorConfig() SelectorConfig {
	return SelectorConfig{
		MaxResults: 10,
		MinScore:   0.1,
	}
}

// NewSelector creates a new tool selector.
func NewSelector() *Selector {
	return &Selector{
		tools:      make(map[string]tools.Tool),
		categories: make(map[string][]string),
	}
}

// RegisterTool adds a tool to the selector with optional categories.
func (s *Selector) RegisterTool(tool tools.Tool, categories ...string) {
	s.tools[tool.Name()] = tool
	s.categories[tool.Name()] = categories
}

// RegisterToolsFrom imports all tools from a coordinator.
func (s *Selector) RegisterToolsFrom(c *Coordinator) {
	for name, tool := range c.tools {
		s.RegisterTool(tool)
		_ = name
	}
}

// RankTools returns tools ranked by relevance to the given intent.
func (s *Selector) RankTools(ctx context.Context, intent string, cfg SelectorConfig) []ToolScore {
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 10
	}

	scores := make([]ToolScore, 0, len(s.tools))
	intentLower := strings.ToLower(intent)
	intentWords := strings.Fields(intentLower)

	for _, tool := range s.tools {
		score, reason := s.scoreToolForIntent(tool, intentLower, intentWords)
		if score >= cfg.MinScore {
			scores = append(scores, ToolScore{
				Tool:       tool,
				Score:      score,
				Reason:     reason,
				Categories: s.categories[tool.Name()],
			})
		}
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// Limit results
	if len(scores) > cfg.MaxResults {
		scores = scores[:cfg.MaxResults]
	}

	return scores
}

// scoreToolForIntent calculates relevance score between a tool and intent.
func (s *Selector) scoreToolForIntent(tool tools.Tool, intentLower string, intentWords []string) (float64, string) {
	nameLower := strings.ToLower(tool.Name())
	descLower := strings.ToLower(tool.Description())

	var score float64
	var reasons []string

	// 1. Exact name match (highest signal)
	if strings.Contains(intentLower, nameLower) {
		score += 0.5
		reasons = append(reasons, "name match")
	}

	// 2. Word overlap with description
	descWords := strings.Fields(descLower)
	matchedWords := 0
	for _, iw := range intentWords {
		if len(iw) < 3 {
			continue // Skip short words
		}
		for _, dw := range descWords {
			if strings.Contains(dw, iw) || strings.Contains(iw, dw) {
				matchedWords++
				break
			}
		}
	}
	if matchedWords > 0 && len(intentWords) > 0 {
		wordScore := float64(matchedWords) / float64(len(intentWords)) * 0.3
		score += wordScore
		reasons = append(reasons, "description overlap")
	}

	// 3. Category bonus (if categories match common intent patterns)
	categories := s.categories[tool.Name()]
	for _, cat := range categories {
		catLower := strings.ToLower(cat)
		if strings.Contains(intentLower, catLower) {
			score += 0.15
			reasons = append(reasons, "category match: "+cat)
		}
	}

	// 4. Common action verb detection
	actionVerbs := map[string][]string{
		"read":   {"read", "show", "display", "get", "fetch", "view", "list"},
		"write":  {"write", "create", "add", "insert", "save", "store"},
		"edit":   {"edit", "modify", "update", "change", "fix", "patch"},
		"delete": {"delete", "remove", "drop", "clear"},
		"search": {"search", "find", "grep", "locate", "look"},
		"run":    {"run", "execute", "start", "launch", "test"},
	}

	for actionType, verbs := range actionVerbs {
		intentHasAction := false
		for _, v := range verbs {
			if strings.Contains(intentLower, v) {
				intentHasAction = true
				break
			}
		}
		if !intentHasAction {
			continue
		}

		// Check if tool matches this action type
		for _, v := range verbs {
			if strings.Contains(nameLower, v) || strings.Contains(descLower, v) {
				score += 0.1
				reasons = append(reasons, "action match: "+actionType)
				break
			}
		}
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	reason := "no match"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, ", ")
	}

	return score, reason
}

// FilterByCategory returns only tools in the specified categories.
func (s *Selector) FilterByCategory(category string) []tools.Tool {
	var result []tools.Tool
	categoryLower := strings.ToLower(category)

	for name, tool := range s.tools {
		categories := s.categories[name]
		for _, cat := range categories {
			if strings.ToLower(cat) == categoryLower {
				result = append(result, tool)
				break
			}
		}
	}

	return result
}

// SuggestForPhase returns tools appropriate for a given pipeline phase.
func (s *Selector) SuggestForPhase(phase string) []tools.Tool {
	phaseTools := map[string][]string{
		"intake":   {"read", "search", "glob", "grep"},
		"planning": {"read", "search", "knowledge"},
		"execute":  {"write", "edit", "bash", "run"},
		"review":   {"read", "diff", "test"},
		"finalize": {"git", "commit", "format"},
	}

	prefixes := phaseTools[strings.ToLower(phase)]
	if prefixes == nil {
		return nil
	}

	var result []tools.Tool
	for _, tool := range s.tools {
		nameLower := strings.ToLower(tool.Name())
		for _, prefix := range prefixes {
			if strings.Contains(nameLower, prefix) {
				result = append(result, tool)
				break
			}
		}
	}

	return result
}
