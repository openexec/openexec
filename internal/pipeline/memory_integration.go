package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/memory"
)

// extractMemory extracts learning patterns from a completed stage result
// and stores them in the memory system for future sessions.
func (p *Pipeline) extractMemory(run *blueprint.Run, result *blueprint.StageResult) {
	if p.memoryManager == nil || result == nil {
		return
	}

	// Extract patterns from stage output
	entry := &memory.MemoryEntry{
		Category:    "stage_pattern",
		Key:         fmt.Sprintf("%s:%s", run.BlueprintID, result.StageName),
		Value:       result.Output,
		Source:      fmt.Sprintf("run:%s", run.ID),
		Layer:       memory.LayerProject.String(),
		ExtractedAt: time.Now().UTC(),
	}

	_ = p.memoryManager.StoreEntry(entry)
}

// loadMemoryContext loads relevant memories for a task and formats them for briefing injection.
func (p *Pipeline) loadMemoryContext(taskDescription string) string {
	if p.memoryManager == nil || taskDescription == "" {
		return ""
	}

	entries, err := p.memoryManager.Search(taskDescription)
	if err != nil || len(entries) == 0 {
		return ""
	}

	// Limit to 5 most relevant entries
	limit := 5
	if len(entries) < limit {
		limit = len(entries)
	}
	entries = entries[:limit]

	var sb strings.Builder
	sb.WriteString("\n\n--- Prior Learning Context ---\n")
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Key, e.Value))
	}
	return sb.String()
}
