package knowledge

import (
	"context"
	"fmt"

	statepkg "github.com/openexec/openexec/pkg/db/state"
)

// SuggestedValidationItems converts advisory graph recommendations into
// suggested, optional items. It deliberately does not accept or execute them.
func SuggestedValidationItems(impact QueryEnvelope[ImpactResult]) []statepkg.ValidationItem {
	items := make([]statepkg.ValidationItem, 0, len(impact.Result.ValidationRecommendations))
	for _, recommendation := range impact.Result.ValidationRecommendations {
		paths := make([]string, 0, len(recommendation.GraphPaths))
		for _, path := range recommendation.GraphPaths {
			paths = append(paths, fmt.Sprintf("%s --%s--> %s [%s]", path.FromNodeID, path.EdgeType, path.ToNodeID, path.EdgeID))
		}
		items = append(items, statepkg.ValidationItem{
			Source: "graph", Disposition: "suggested", Requirement: "optional",
			Criterion: recommendation.Criterion, Scope: recommendation.Scope,
			GraphPaths: paths, Limitations: recommendation.Limitations,
		})
	}
	return items
}

// ProposeValidationPlanFromImpact records a proposal against the exact graph
// state. A later authorized decision must create an accepted revision.
func ProposeValidationPlanFromImpact(ctx context.Context, canonical *statepkg.Store, taskID, runID, patchHash string, impact QueryEnvelope[ImpactResult]) (statepkg.ValidationPlanRevision, error) {
	if impact.Generation.Freshness != FreshnessCurrent {
		return statepkg.ValidationPlanRevision{}, fmt.Errorf("cannot propose impact validation from %s graph", impact.Generation.Freshness)
	}
	return canonical.CreateValidationPlanRevision(ctx, statepkg.ValidationPlanRevision{
		TaskID: taskID, RunID: runID, GenerationID: impact.Generation.GraphVersion,
		WorktreeStateHash: impact.Generation.WorktreeStateHash, PatchHash: patchHash,
		Status: "proposed", Items: SuggestedValidationItems(impact),
	})
}
