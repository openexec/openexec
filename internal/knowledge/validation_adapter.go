package knowledge

import (
	"context"
	"fmt"

	statepkg "github.com/openexec/openexec/pkg/db/state"
)

// SuggestedValidationItems converts advisory graph recommendations into
// suggested, optional items. It deliberately does not accept or execute them.
func SuggestedValidationItems(impact QueryEnvelope[ImpactResult]) []statepkg.ValidationItem {
	return suggestedValidationItems(impact.Result.ValidationRecommendations)
}

func suggestedValidationItems(recommendations []ValidationRecommendation) []statepkg.ValidationItem {
	items := make([]statepkg.ValidationItem, 0, len(recommendations))
	for _, recommendation := range recommendations {
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

func ProposeValidationPlanFromChangedImpact(ctx context.Context, canonical *statepkg.Store, taskID, runID, patchHash string, request ChangedImpactRequest, impact ChangedImpactResponse) (statepkg.ValidationPlanRevision, error) {
	if impact.Generation.Freshness != FreshnessCurrent {
		return statepkg.ValidationPlanRevision{}, fmt.Errorf("cannot propose impact validation from %s graph", impact.Generation.Freshness)
	}
	files, err := normalizeChangedImpactFiles(request.Files)
	if err != nil {
		return statepkg.ValidationPlanRevision{}, err
	}
	depth := request.MaxDepth
	if depth <= 0 {
		depth = 2
	}
	changed := make([]string, 0, len(impact.ChangedSymbols))
	affected := make([]string, 0, len(impact.Propagation.DirectCallers)+len(impact.Propagation.AffectedCallers)+len(impact.Propagation.ModuleDependants)+len(impact.Propagation.OperationalEffects))
	tests := make([]string, 0, len(impact.Propagation.RelatedTests))
	for _, symbol := range impact.ChangedSymbols {
		changed = append(changed, symbol.ID)
	}
	for _, node := range impact.Propagation.DirectCallers {
		affected = append(affected, node.Node.ID)
	}
	for _, node := range impact.Propagation.AffectedCallers {
		affected = append(affected, node.Node.ID)
	}
	for _, node := range impact.Propagation.ModuleDependants {
		affected = append(affected, node.Node.ID)
	}
	for _, node := range impact.Propagation.OperationalEffects {
		affected = append(affected, node.Node.ID)
	}
	for _, test := range impact.Propagation.RelatedTests {
		tests = append(tests, test.FilePath)
	}
	return canonical.CreateValidationPlanRevision(ctx, statepkg.ValidationPlanRevision{
		TaskID: taskID, RunID: runID, GenerationID: impact.Generation.GraphVersion,
		WorktreeStateHash: impact.Generation.WorktreeStateHash, PatchHash: patchHash,
		ImpactQuery: statepkg.ValidationImpactQuery{Files: files, SymbolIDs: uniqueNonEmpty(request.SymbolIDs), MaxDepth: depth},
		ImpactSummary: statepkg.ValidationImpactSummary{
			ChangedSymbolIDs: uniqueNonEmpty(changed), AffectedNodeIDs: uniqueNonEmpty(affected), RelatedTestFiles: uniqueNonEmpty(tests),
			UnresolvedFiles: append([]string{}, impact.UnresolvedFiles...), Unresolved: append([]string{}, impact.Unresolved...),
			Limitations: append([]string{}, impact.Limitations...), Truncated: impact.Truncated,
		},
		Status: "proposed", Items: suggestedValidationItems(impact.ValidationRecommendations),
	})
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
