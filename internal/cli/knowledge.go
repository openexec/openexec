package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/repository"
	statepkg "github.com/openexec/openexec/pkg/db/state"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Manage and inspect the Deterministic Knowledge Base",
}

var (
	graphDirectory           string
	graphJSON                bool
	graphFile                string
	graphKind                string
	graphRead                bool
	graphReverse             bool
	graphDepth               int
	graphCallDirection       string
	graphCallDepth           int
	graphImpactDepth         int
	graphImpactFiles         []string
	graphConsoleURL          string
	graphConsoleProject      string
	graphConsoleTokenFile    string
	graphSymbols             []string
	graphTaskID              string
	graphRunID               string
	graphPlanID              string
	validationFiles          []string
	validationSymbolIDs      []string
	validationPatchHash      string
	validationDepth          int
	validationMode           string
	validationPhase          string
	validationStatus         string
	validationEvidenceStatus string
	validationIteration      int
	validationItemID         string
	validationStepID         string
	validationArtifact       string
	validationFinalize       bool
	validationRequiredItems  []string
	validationOptionalItems  []string
	validationRejectedItems  []string
)

var knowledgeGraphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Inspect the versioned repository pointer graph",
}

var knowledgeGraphValidationCmd = &cobra.Command{
	Use:   "validation",
	Short: "Manage graph-derived validation authority",
}

func openValidationState(directory string) (*statepkg.Store, error) {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	return statepkg.NewStore(filepath.Join(abs, ".openexec", "openexec.db"))
}

func ensurePlanMatchesDirectory(ctx context.Context, canonical *statepkg.Store, plan statepkg.ValidationPlanRevision, identity knowledge.RepositoryIdentity) error {
	var checkoutID string
	if err := canonical.GetDB().QueryRowContext(ctx, `SELECT checkout_id FROM graph_generations WHERE id = ?`, plan.GenerationID).Scan(&checkoutID); err != nil {
		return err
	}
	if checkoutID != identity.CheckoutID {
		return fmt.Errorf("validation plan belongs to a different checkout")
	}
	return nil
}

var knowledgeGraphValidationProposeCmd = &cobra.Command{
	Use:   "propose",
	Short: "Create an immutable validation proposal from changed files and symbols",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(graphTaskID) == "" {
			return fmt.Errorf("--task is required")
		}
		graph, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		impactRequest := knowledge.ChangedImpactRequest{Files: validationFiles, SymbolIDs: validationSymbolIDs, MaxDepth: validationDepth}
		impact, err := graph.ChangedImpactAnalysis(cmd.Context(), identity, impactRequest, knowledge.DefaultChangedImpactLimits())
		if closeErr := graph.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if len(impact.ChangedSymbols) == 0 {
			return fmt.Errorf("validation proposal requires at least one resolved changed symbol")
		}
		canonical, err := openValidationState(graphDirectory)
		if err != nil {
			return err
		}
		defer canonical.Close()
		plan, err := knowledge.ProposeValidationPlanFromChangedImpact(cmd.Context(), canonical, strings.TrimSpace(graphTaskID), strings.TrimSpace(graphRunID), strings.TrimSpace(validationPatchHash), impactRequest, impact)
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, plan)
		}
		cmd.Printf("Proposed validation plan %s revision %d for graph %s\n", plan.ID, plan.Revision, plan.GenerationID)
		return nil
	},
}

var knowledgeGraphValidationShowCmd = &cobra.Command{
	Use:   "show PLAN_ID",
	Short: "Read one immutable validation-plan revision",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		graph, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		if err := graph.Close(); err != nil {
			return err
		}
		canonical, err := openValidationState(graphDirectory)
		if err != nil {
			return err
		}
		defer canonical.Close()
		plan, err := canonical.GetValidationPlanRevision(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := ensurePlanMatchesDirectory(cmd.Context(), canonical, plan, identity); err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, plan)
		}
		cmd.Printf("Validation plan %s: %s revision %d; %d changed symbols, %d items, truncated=%t\n", plan.ID, plan.Status, plan.Revision, len(plan.ImpactSummary.ChangedSymbolIDs), len(plan.Items), plan.ImpactSummary.Truncated)
		return nil
	},
}

var knowledgeGraphValidationAcceptCmd = &cobra.Command{
	Use:   "accept PLAN_ID",
	Short: "Create an accepted revision from a current proposal",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		graph, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		defer graph.Close()
		canonical, err := openValidationState(graphDirectory)
		if err != nil {
			return err
		}
		defer canonical.Close()
		plan, err := canonical.GetValidationPlanRevision(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := ensurePlanMatchesDirectory(cmd.Context(), canonical, plan, identity); err != nil {
			return err
		}
		if len(plan.ImpactSummary.ChangedSymbolIDs) == 0 {
			return fmt.Errorf("validation plan has no resolved changed symbol")
		}
		if existing, err := canonical.AcceptedValidationPlanRevisionForProposal(cmd.Context(), plan.ID); err == nil {
			if graphJSON {
				return writeCommandJSON(cmd, existing)
			}
			cmd.Printf("Accepted validation plan %s revision %d\n", existing.ID, existing.Revision)
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		current, err := graph.CurrentRepositoryState(cmd.Context(), identity)
		if err != nil {
			return err
		}
		if current.GraphVersion != plan.GenerationID || current.WorktreeStateHash != plan.WorktreeStateHash {
			return statepkg.ErrValidationProposalStale
		}
		decisions := make([]statepkg.ValidationItemDecision, 0, len(validationRequiredItems)+len(validationOptionalItems)+len(validationRejectedItems))
		for _, id := range validationRequiredItems {
			decisions = append(decisions, statepkg.ValidationItemDecision{ID: id, Disposition: "accepted", Requirement: "required"})
		}
		for _, id := range validationOptionalItems {
			decisions = append(decisions, statepkg.ValidationItemDecision{ID: id, Disposition: "accepted", Requirement: "optional"})
		}
		for _, id := range validationRejectedItems {
			decisions = append(decisions, statepkg.ValidationItemDecision{ID: id, Disposition: "rejected", Requirement: "optional"})
		}
		accepted, err := canonical.AcceptValidationPlanRevision(cmd.Context(), plan.ID, strings.TrimSpace(graphRunID), decisions)
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, accepted)
		}
		cmd.Printf("Accepted validation plan %s revision %d\n", accepted.ID, accepted.Revision)
		return nil
	},
}

var knowledgeGraphValidationRunCmd = &cobra.Command{
	Use:   "run RUN_ID",
	Short: "Register an external validation run in this checkout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if validationMode != "read-only" && validationMode != "workspace-write" {
			return fmt.Errorf("--mode must be read-only or workspace-write")
		}
		canonical, err := openValidationState(graphDirectory)
		if err != nil {
			return err
		}
		defer canonical.Close()
		root, err := filepath.Abs(graphDirectory)
		if err != nil {
			return err
		}
		if existing, err := canonical.GetRun(cmd.Context(), args[0]); err != nil {
			return err
		} else if existing != nil {
			if existing.ProjectPath != root || existing.Mode != validationMode {
				return fmt.Errorf("run id already belongs to a different execution context")
			}
			if graphJSON {
				return writeCommandJSON(cmd, existing)
			}
			cmd.Printf("Validation run %s already registered\n", existing.ID)
			return nil
		}
		if err := canonical.CreateRun(cmd.Context(), args[0], "", "", root, validationMode); err != nil {
			return err
		}
		created, err := canonical.GetRun(cmd.Context(), args[0])
		if err != nil || created == nil || created.ProjectPath != root || created.Mode != validationMode {
			return fmt.Errorf("run id was claimed by a different execution context")
		}
		if graphJSON {
			return writeCommandJSON(cmd, created)
		}
		cmd.Printf("Registered validation run %s\n", args[0])
		return nil
	},
}

var knowledgeGraphValidationStepCmd = &cobra.Command{
	Use:   "step RUN_ID STEP_ID",
	Short: "Register a structured terminal step for an external run",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		validPhase := map[string]bool{"plan": true, "execute": true, "verify": true, "finalize": true}
		validStatus := map[string]bool{"completed": true, "failed": true, "inconclusive": true, "unavailable": true}
		if !validPhase[validationPhase] || !validStatus[validationStatus] || validationIteration < 1 {
			return fmt.Errorf("step requires a valid phase, terminal status, and positive iteration")
		}
		canonical, err := openValidationState(graphDirectory)
		if err != nil {
			return err
		}
		defer canonical.Close()
		root, _ := filepath.Abs(graphDirectory)
		run, err := canonical.GetRun(cmd.Context(), args[0])
		if err != nil || run == nil || run.ProjectPath != root {
			return fmt.Errorf("validation run not found in this checkout")
		}
		if existing, err := canonical.GetRunStep(cmd.Context(), args[1]); err != nil {
			return err
		} else if existing != nil {
			if existing.RunID != run.ID || existing.Phase != validationPhase || existing.Iteration != validationIteration || existing.Status != validationStatus {
				return fmt.Errorf("run step id already belongs to different evidence")
			}
			cmd.Printf("Validation step %s already registered\n", existing.ID)
			return nil
		}
		if err := canonical.AddRunStep(cmd.Context(), args[1], run.ID, "", validationPhase, validationIteration, validationStatus); err != nil {
			return err
		}
		created, err := canonical.GetRunStep(cmd.Context(), args[1])
		if err != nil || created == nil || created.RunID != run.ID || created.Phase != validationPhase || created.Iteration != validationIteration || created.Status != validationStatus {
			return fmt.Errorf("run step id was claimed by different evidence")
		}
		if graphJSON {
			return writeCommandJSON(cmd, created)
		}
		cmd.Printf("Registered validation step %s\n", args[1])
		return nil
	},
}

var knowledgeGraphValidationEvidenceCmd = &cobra.Command{
	Use:   "evidence PLAN_ID",
	Short: "Link one structured run step to an accepted validation item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		graph, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		if err := graph.Close(); err != nil {
			return err
		}
		canonical, err := openValidationState(graphDirectory)
		if err != nil {
			return err
		}
		defer canonical.Close()
		plan, err := canonical.GetValidationPlanRevision(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := ensurePlanMatchesDirectory(cmd.Context(), canonical, plan, identity); err != nil {
			return err
		}
		root, _ := filepath.Abs(graphDirectory)
		run, err := canonical.GetRun(cmd.Context(), graphRunID)
		if err != nil || run == nil || run.ProjectPath != root {
			return fmt.Errorf("validation run not found in this checkout")
		}
		step, err := canonical.GetRunStep(cmd.Context(), validationStepID)
		if err != nil || step == nil || step.RunID != run.ID {
			return fmt.Errorf("validation run step not found")
		}
		if !statepkg.EvidenceStatusMatchesRunStep(validationEvidenceStatus, step.Status) {
			return fmt.Errorf("evidence status does not match the structured run step")
		}
		itemBelongs := false
		for _, item := range plan.Items {
			itemBelongs = itemBelongs || item.ID == validationItemID
		}
		if !itemBelongs {
			return fmt.Errorf("validation item does not belong to this plan revision")
		}
		link := statepkg.ValidationEvidenceLink{ValidationItemID: validationItemID, RunID: run.ID, RunStepID: step.ID, ArtifactHash: validationArtifact, WorktreeStateHash: plan.WorktreeStateHash, PatchHash: plan.PatchHash, Status: validationEvidenceStatus}
		if err := canonical.LinkValidationEvidence(cmd.Context(), link); err != nil {
			return err
		}
		cmd.Printf("Linked %s evidence to validation item %s\n", validationEvidenceStatus, validationItemID)
		return nil
	},
}

var knowledgeGraphValidationCompletionCmd = &cobra.Command{
	Use:   "completion PLAN_ID",
	Short: "Read or finalize an immutable completion report",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		graph, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		if err := graph.Close(); err != nil {
			return err
		}
		canonical, err := openValidationState(graphDirectory)
		if err != nil {
			return err
		}
		defer canonical.Close()
		plan, err := canonical.GetValidationPlanRevision(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err := ensurePlanMatchesDirectory(cmd.Context(), canonical, plan, identity); err != nil {
			return err
		}
		var report statepkg.CompletionReport
		if validationFinalize {
			report, err = canonical.FinalizeEvidenceCoverage(cmd.Context(), args[0])
		} else {
			report, err = canonical.ReadCompletionReport(cmd.Context(), args[0])
		}
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, report)
		}
		cmd.Printf("Completion report %s: can_complete=%t; verified=%d; not_verified=%d\n", report.ID, report.CanComplete, len(report.Verified), len(report.NotVerified))
		return nil
	},
}

var knowledgeGraphScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Build and atomically publish a repository graph generation",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := knowledge.NewStore(graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		result, err := store.ScanRepository(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, result)
		}
		cmd.Printf("Graph %s: %s (%d files, %d symbols, %d edges)\n", result.Generation.ID, result.Generation.Status, result.Files, result.Symbols, result.Edges)
		for _, limitation := range result.Limitations {
			cmd.Printf("  limitation: %s\n", limitation)
		}
		return nil
	},
}

var knowledgeGraphRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh changed graph inputs and fall back to a full scan when required",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := knowledge.NewStore(graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		result, err := store.RefreshRepository(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, result)
		}
		cmd.Printf("Graph %s: %s; changed=%t; invalidation=%s/%s; full_scan=%t\n", result.Generation.ID, result.Generation.Status, result.Changed, result.Invalidation.Scope, result.Invalidation.Cause, result.Invalidation.FullScan)
		return nil
	},
}

var knowledgeGraphSymbolCmd = &cobra.Command{
	Use:   "symbol NAME",
	Short: "Resolve a symbol without silently selecting ambiguous candidates",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		store, identity, err := openGraphStore(ctx, graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		resolved, err := store.ResolveGraphSymbol(ctx, identity, args[0], graphFile, graphKind, 20)
		if err != nil {
			return err
		}
		if graphRead && resolved.Result.Candidate != nil {
			reader, err := repository.NewRootedReader(graphDirectory, identity.RepositoryID, identity.WorktreeID, knowledge.DefaultGraphLimits().MaxBytes)
			if err != nil {
				return err
			}
			returnValue, err := store.ReadGraphSymbol(ctx, identity, resolved.Result.Candidate.Symbol.ID, reader)
			if err != nil {
				return err
			}
			if graphJSON {
				return writeCommandJSON(cmd, returnValue)
			}
			cmd.Printf("%s (%s) %s:%d-%d\n%s\n", returnValue.Result.Symbol.DisplayName, returnValue.Result.Symbol.Kind, returnValue.Result.Occurrence.FilePath, returnValue.Result.Occurrence.StartLine, returnValue.Result.Occurrence.EndLine, returnValue.Result.Source.Content)
			return nil
		}
		if graphJSON {
			return writeCommandJSON(cmd, resolved)
		}
		if resolved.Result.Candidate != nil {
			candidate := resolved.Result.Candidate
			cmd.Printf("%s %s (%s:%d-%d) [%s]\n", candidate.Symbol.Kind, candidate.Symbol.QualifiedName, candidate.Occurrence.FilePath, candidate.Occurrence.StartLine, candidate.Occurrence.EndLine, candidate.Symbol.ID)
			return nil
		}
		if len(resolved.Result.Candidates) > 0 {
			cmd.Println("Ambiguous symbol; specify --file or --kind:")
			for _, candidate := range resolved.Result.Candidates {
				cmd.Printf("  %s %s (%s:%d) [%s]\n", candidate.Symbol.Kind, candidate.Symbol.QualifiedName, candidate.Occurrence.FilePath, candidate.Occurrence.StartLine, candidate.Symbol.ID)
			}
			return nil
		}
		return fmt.Errorf("symbol %q was not resolved", args[0])
	},
}

var knowledgeGraphDependenciesCmd = &cobra.Command{
	Use:   "dependencies MODULE",
	Short: "Show bounded module dependencies or dependants",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		result, err := store.FindModuleDependencies(cmd.Context(), identity, args[0], graphReverse, graphDepth, knowledge.DefaultGraphLimits())
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, result)
		}
		cmd.Printf("%s [%s]\n", result.Result.Root.QualifiedName, result.Generation.Freshness)
		for _, node := range result.Result.Nodes {
			cmd.Printf("  %s %s\n", node.NodeType, node.QualifiedName)
		}
		if result.Truncated {
			cmd.Println("  result truncated by graph limits")
		}
		return nil
	},
}

var knowledgeGraphImpactCmd = &cobra.Command{
	Use:   "impact [SYMBOL_ID...]",
	Short: "Explain a bounded affected set and validation recommendations",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && len(graphImpactFiles) == 0 {
			return fmt.Errorf("impact requires symbol ids or --files")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		store, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		if len(graphImpactFiles) > 0 {
			result, err := store.ChangedImpactAnalysis(cmd.Context(), identity, knowledge.ChangedImpactRequest{
				Files: graphImpactFiles, SymbolIDs: args, MaxDepth: graphImpactDepth,
			}, knowledge.DefaultChangedImpactLimits())
			if err != nil {
				return err
			}
			if graphJSON {
				return writeCommandJSON(cmd, result)
			}
			cmd.Printf("Changed impact: %s [%s]\n", result.Resolution.Status, result.Generation.Freshness)
			for _, symbol := range result.ChangedSymbols {
				cmd.Printf("  changed: %s\n", symbol.QualifiedName)
			}
			for _, caller := range result.Propagation.DirectCallers {
				cmd.Printf("  caller: %s\n", caller.Node.QualifiedName)
			}
			for _, caller := range result.Propagation.AffectedCallers {
				cmd.Printf("  affected caller: %s\n", caller.Node.QualifiedName)
			}
			for _, dependant := range result.Propagation.ModuleDependants {
				cmd.Printf("  module dependant: %s\n", dependant.Node.QualifiedName)
			}
			for _, effect := range result.Propagation.OperationalEffects {
				cmd.Printf("  operational effect: %s\n", effect.Node.QualifiedName)
			}
			for _, test := range result.Propagation.RelatedTests {
				cmd.Printf("  test candidate: %s (%s)\n", test.FilePath, test.Reason)
			}
			for _, file := range result.UnresolvedFiles {
				cmd.Printf("  unresolved file: %s\n", file)
			}
			printed := map[string]bool{}
			for _, limitation := range result.Unresolved {
				cmd.Printf("  unresolved: %s\n", limitation)
				printed[limitation] = true
			}
			for _, limitation := range result.Limitations {
				if !printed[limitation] {
					cmd.Printf("  limitation: %s\n", limitation)
				}
			}
			if result.Truncated {
				cmd.Println("  result truncated by graph limits")
			}
			return nil
		}
		result, err := store.ImpactAnalysis(cmd.Context(), identity, args, graphImpactDepth, knowledge.DefaultGraphLimits())
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, result)
		}
		cmd.Printf("Impact: %s [%s]\n", result.Resolution.Status, result.Generation.Freshness)
		for _, caller := range result.Result.DirectCallers {
			cmd.Printf("  caller: %s\n", caller.Node.QualifiedName)
		}
		for _, test := range result.Result.RelatedTests {
			cmd.Printf("  test candidate: %s (%s)\n", test.FilePath, test.Reason)
		}
		for _, limitation := range result.Result.Unresolved {
			cmd.Printf("  unresolved: %s\n", limitation)
		}
		return nil
	},
}

var knowledgeGraphCallsCmd = &cobra.Command{
	Use:   "calls SYMBOL_ID",
	Short: "Show bounded incoming or outgoing symbol call relationships",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if graphCallDirection != "incoming" && graphCallDirection != "outgoing" {
			return fmt.Errorf("direction must be incoming or outgoing")
		}
		store, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		result, err := store.FindSymbolRelationships(cmd.Context(), identity, args[0], graphCallDirection == "incoming", graphCallDepth, []string{"calls"}, knowledge.DefaultGraphLimits())
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, result)
		}
		cmd.Printf("%s calls for %s [%s]\n", graphCallDirection, result.Result.Root.QualifiedName, result.Generation.Freshness)
		for _, node := range result.Result.Nodes {
			cmd.Printf("  %s %s\n", node.NodeType, node.QualifiedName)
		}
		if result.Truncated {
			cmd.Println("  result truncated by graph limits")
		}
		return nil
	},
}

var knowledgeGraphStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the active graph generation and freshness",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		state, err := store.CurrentRepositoryState(cmd.Context(), identity)
		if err != nil {
			return err
		}
		if graphJSON {
			return writeCommandJSON(cmd, state)
		}
		cmd.Printf("Graph: %s\nFreshness: %s\nRepository: %s\nWorktree: %s\nState: %s\n", state.GraphVersion, state.Freshness, state.RepositoryID, state.WorktreeID, state.WorktreeStateHash)
		return nil
	},
}

var knowledgeGraphPublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Refresh the graph and publish its lossy repository read model to Agent Console",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
		token := strings.TrimSpace(os.Getenv("AGENT_CONSOLE_TOKEN"))
		if graphConsoleTokenFile != "" {
			contents, readErr := os.ReadFile(graphConsoleTokenFile)
			if readErr != nil {
				return fmt.Errorf("read Agent Console token file: %w", readErr)
			}
			if len(contents) > 4096 {
				return fmt.Errorf("Agent Console token file is unexpectedly large")
			}
			token = strings.TrimSpace(string(contents))
		}
		projection, err := refreshAndPublishRepositoryContext(cmd.Context(), store, identity, graphConsoleURL, graphConsoleProject, token, graphSymbols, graphTaskID, graphRunID, graphPlanID, knowledge.RepositoryContextPublisher{})
		if err != nil {
			return err
		}
		cmd.Printf("Published graph %s (%s) to Agent Console project %s\n", projection.GraphVersion, projection.Freshness, graphConsoleProject)
		return nil
	},
}

type repositoryContextPublisher interface {
	Publish(context.Context, string, string, string, knowledge.RepositoryContextProjection) error
}

func refreshAndPublishRepositoryContext(ctx context.Context, store *knowledge.Store, identity knowledge.RepositoryIdentity, consoleURL, consoleProject, token string, symbols []string, taskID, runID, planID string, publisher repositoryContextPublisher) (knowledge.RepositoryContextProjection, error) {
	if _, err := store.RefreshRepository(ctx, identity.RootPath); err != nil {
		return knowledge.RepositoryContextProjection{}, fmt.Errorf("refresh repository graph: %w", err)
	}
	report, err := repositoryContextCompletionReport(ctx, identity, planID)
	if err != nil {
		return knowledge.RepositoryContextProjection{}, err
	}
	projection, err := store.BuildRepositoryContext(ctx, identity, symbols, taskID, runID, planID, report)
	if err != nil {
		return knowledge.RepositoryContextProjection{}, err
	}
	if err := publisher.Publish(ctx, consoleURL, consoleProject, token, projection); err != nil {
		return knowledge.RepositoryContextProjection{}, err
	}
	return projection, nil
}

func repositoryContextCompletionReport(ctx context.Context, identity knowledge.RepositoryIdentity, planID string) (*statepkg.CompletionReport, error) {
	if strings.TrimSpace(planID) == "" {
		return nil, nil
	}
	canonical, err := openValidationState(identity.RootPath)
	if err != nil {
		return nil, err
	}
	defer canonical.Close()
	plan, err := canonical.GetValidationPlanRevision(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("load repository context validation plan: %w", err)
	}
	if err := ensurePlanMatchesDirectory(ctx, canonical, plan, identity); err != nil {
		return nil, fmt.Errorf("validate repository context plan: %w", err)
	}
	report, err := canonical.ReadCompletionReport(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("load repository context completion report: %w", err)
	}
	return &report, nil
}

func openGraphStore(ctx context.Context, directory string) (*knowledge.Store, knowledge.RepositoryIdentity, error) {
	store, err := knowledge.NewStore(directory)
	if err != nil {
		return nil, knowledge.RepositoryIdentity{}, err
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, directory, "")
	if err != nil {
		store.Close()
		return nil, knowledge.RepositoryIdentity{}, err
	}
	_, _ = store.MigrateLegacySymbols(ctx, identity)
	return store, identity, nil
}

func writeCommandJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

var knowledgeLsCmd = &cobra.Command{
	Use:   "ls [directory]",
	Short: "List all projects with an initialized knowledge base",
	RunE: func(cmd *cobra.Command, args []string) error {
		searchDir := "."
		if len(args) > 0 {
			searchDir = args[0]
		}

		cmd.Printf("Searching for knowledge bases in %s...\n", searchDir)

		found := 0
		err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.Name() == "knowledge.db" && strings.Contains(path, ".openexec") {
				// Path is something like path/to/project/.openexec/knowledge.db
				projectPath := filepath.Dir(filepath.Dir(path))
				cmd.Printf("  - %s\n", projectPath)
				found++
			}
			return nil
		})

		if err != nil {
			return err
		}
		if found == 0 {
			cmd.Println("No knowledge bases found.")
		} else {
			cmd.Printf("\nFound %d knowledge base(s).\n", found)
		}
		return nil
	},
}

var knowledgeShowCmd = &cobra.Command{
	Use:   "show [symbols|envs|api|prd]",
	Short: "Show records in the current project's knowledge base",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kType := args[0]
		store, err := knowledge.NewStore(".")
		if err != nil {
			return err
		}
		defer store.Close()

		switch kType {
		case "symbols":
			list, _ := store.ListSymbols()
			cmd.Println("Surgical Pointers (Symbols):")
			cmd.Println("---------------------------")
			if len(list) == 0 {
				cmd.Println("No symbols indexed. Run 'openexec knowledge index'")
			}
			for _, s := range list {
				cmd.Printf("[%s] %s (%s:%d-%d)\n", s.Kind, s.Name, filepath.Base(s.FilePath), s.StartLine, s.EndLine)
				if s.Purpose != "" {
					cmd.Printf("    Purpose: %s\n", s.Purpose)
				}
			}
		case "envs":
			list, _ := store.ListEnvironments()
			cmd.Println("Environment Topologies:")
			cmd.Println("-----------------------")
			if len(list) == 0 {
				cmd.Println("No environments recorded.")
			}
			for _, e := range list {
				cmd.Printf("Env: %s [%s]\n", e.Env, e.RuntimeType)
				cmd.Printf("    Topology: %s\n", e.Topology)
			}
		case "api":
			list, _ := store.ListAPIDocs()
			cmd.Println("API Contracts:")
			cmd.Println("--------------")
			if len(list) == 0 {
				cmd.Println("No API contracts detected.")
			}
			for _, a := range list {
				cmd.Printf("%s %s: %s\n", a.Method, a.Path, a.Description)
			}
		case "prd":
			// We'll show all sections for simplicity
			sections := []string{"personas", "user_journeys", "functional", "non_functional"}
			cmd.Println("Product Requirements (PRD):")
			cmd.Println("--------------------------")
			found := false
			for _, sec := range sections {
				list, _ := store.ListPRDRecords(sec)
				if len(list) > 0 {
					found = true
					cmd.Printf("\nSection: %s\n", strings.ToUpper(sec))
					for _, r := range list {
						cmd.Printf("  - %s: %s\n", r.Key, r.Content)
					}
				}
			}
			if !found {
				cmd.Println("No PRD records found. Use the Wizard to generate them.")
			}
		default:
			return fmt.Errorf("unknown type: %s. Use symbols, envs, api, or prd", kType)
		}
		return nil
	},
}

var knowledgeInitCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize an empty knowledge base at a path",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		_, err := knowledge.NewStore(path)
		if err != nil {
			return err
		}
		cmd.Printf("✓ Initialized empty knowledge base at %s/.openexec/knowledge.db\n", path)
		return nil
	},
}

var knowledgeIndexCmd = &cobra.Command{
	Use:   "index [directory]",
	Short: "Automatically indexes project source code into deterministic Pointer Records",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := "."
		if len(args) > 0 {
			projectDir = args[0]
		}

		cmd.Printf("🔍 Indexing project at %s...\n", projectDir)

		kStore, err := knowledge.NewStore(projectDir)
		if err != nil {
			return err
		}
		defer kStore.Close()

		indexer := knowledge.NewIndexer(kStore)
		if err := indexer.IndexProject(projectDir); err != nil {
			return err
		}

		stats := indexer.GetStats()
		cmd.Printf("✓ Indexing complete. %d symbols indexed across %d files.\n", stats.SymbolsExtracted, stats.FilesProcessed)
		if stats.ErrorCount > 0 {
			cmd.Printf("  %d file(s) had errors during indexing.\n", stats.ErrorCount)
		}
		for lang, count := range stats.ByLanguage {
			cmd.Printf("  %s: %d symbols\n", lang, count)
		}
		graph, err := kStore.ScanRepository(cmd.Context(), projectDir)
		if err != nil {
			return fmt.Errorf("legacy pointers were indexed but graph publication failed: %w", err)
		}
		cmd.Printf("✓ Graph %s published: %d modules, %d symbols, %d edges.\n", graph.Generation.ID, graph.Files, graph.Symbols, graph.Edges)
		return nil
	},
}

var knowledgeSymbolsCmd = &cobra.Command{
	Use:   "symbols",
	Short: "List indexed symbols (functions, structs, interfaces)",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := knowledge.NewStore(".")
		if err != nil {
			return fmt.Errorf("failed to open knowledge store: %w", err)
		}
		defer store.Close()

		symbols, err := store.ListSymbols()
		if err != nil {
			return fmt.Errorf("failed to list symbols: %w", err)
		}

		if len(symbols) == 0 {
			cmd.Println("No symbols indexed. Run 'openexec knowledge index' first.")
			return nil
		}

		// Print table header
		cmd.Printf("%-40s %-12s %-30s %s\n", "Name", "Kind", "File", "Lines")
		cmd.Printf("%-40s %-12s %-30s %s\n",
			strings.Repeat("-", 40),
			strings.Repeat("-", 12),
			strings.Repeat("-", 30),
			strings.Repeat("-", 10))

		for _, s := range symbols {
			file := s.FilePath
			if len(file) > 30 {
				file = "..." + file[len(file)-27:]
			}
			cmd.Printf("%-40s %-12s %-30s %d-%d\n", s.Name, s.Kind, file, s.StartLine, s.EndLine)
		}

		cmd.Printf("\nTotal: %d symbols\n", len(symbols))
		return nil
	},
}

var knowledgeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show knowledge base statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := "."
		absDir, _ := filepath.Abs(projectDir)

		store, err := knowledge.NewStore(projectDir)
		if err != nil {
			return fmt.Errorf("failed to open knowledge store: %w", err)
		}
		defer store.Close()

		symbols, err := store.ListSymbols()
		if err != nil {
			return fmt.Errorf("failed to query symbols: %w", err)
		}

		envs, _ := store.ListEnvironments()
		apis, _ := store.ListAPIDocs()
		policies, _ := store.ListPolicies()

		cmd.Println("Knowledge Base Status")
		cmd.Println("---------------------")
		cmd.Printf("  Project:      %s\n", absDir)

		// Check db file size
		dbPath := filepath.Join(projectDir, ".openexec", "openexec.db")
		if info, err := os.Stat(dbPath); err == nil {
			cmd.Printf("  DB file:      %s (%s)\n", dbPath, formatBytes(info.Size()))
		} else {
			cmd.Printf("  DB file:      %s\n", dbPath)
		}

		cmd.Printf("  Symbols:      %d\n", len(symbols))
		cmd.Printf("  Environments: %d\n", len(envs))
		cmd.Printf("  API docs:     %d\n", len(apis))
		cmd.Printf("  Policies:     %d\n", len(policies))

		return nil
	},
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func init() {
	for _, command := range []*cobra.Command{knowledgeGraphScanCmd, knowledgeGraphRefreshCmd, knowledgeGraphSymbolCmd, knowledgeGraphDependenciesCmd, knowledgeGraphCallsCmd, knowledgeGraphImpactCmd, knowledgeGraphStatusCmd, knowledgeGraphPublishCmd} {
		command.Flags().StringVarP(&graphDirectory, "directory", "d", ".", "repository directory")
		command.Flags().BoolVar(&graphJSON, "json", false, "emit machine-readable JSON")
	}
	knowledgeGraphSymbolCmd.Flags().StringVar(&graphFile, "file", "", "repository-relative file filter")
	knowledgeGraphSymbolCmd.Flags().StringVar(&graphKind, "kind", "", "symbol kind filter")
	knowledgeGraphSymbolCmd.Flags().BoolVar(&graphRead, "read", false, "read the hash-validated source range")
	knowledgeGraphDependenciesCmd.Flags().BoolVar(&graphReverse, "reverse", false, "show module dependants")
	knowledgeGraphDependenciesCmd.Flags().IntVar(&graphDepth, "depth", 1, "bounded traversal depth")
	knowledgeGraphCallsCmd.Flags().StringVar(&graphCallDirection, "direction", "outgoing", "call direction: outgoing or incoming")
	knowledgeGraphCallsCmd.Flags().IntVar(&graphCallDepth, "depth", 1, "bounded traversal depth")
	knowledgeGraphImpactCmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "depth" {
			return pflag.NormalizedName("max-depth")
		}
		return pflag.NormalizedName(name)
	})
	knowledgeGraphImpactCmd.Flags().IntVar(&graphImpactDepth, "max-depth", 2, "bounded traversal depth")
	knowledgeGraphImpactCmd.Flags().StringSliceVar(&graphImpactFiles, "files", nil, "repository-relative changed files (comma-separated or repeated)")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphConsoleURL, "console-url", "", "Agent Console base URL")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphConsoleProject, "console-project", "", "Agent Console checkout ID")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphConsoleTokenFile, "console-token-file", "", "file containing the Agent Console bearer token (or export AGENT_CONSOLE_TOKEN)")
	knowledgeGraphPublishCmd.Flags().StringSliceVar(&graphSymbols, "symbol", nil, "symbol name to include (repeatable)")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphTaskID, "task", "", "OpenExec task reference")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphRunID, "run", "", "OpenExec run reference")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphPlanID, "plan", "", "OpenExec validation-plan reference")
	_ = knowledgeGraphPublishCmd.MarkFlagRequired("console-url")
	_ = knowledgeGraphPublishCmd.MarkFlagRequired("console-project")
	validationCommands := []*cobra.Command{knowledgeGraphValidationProposeCmd, knowledgeGraphValidationShowCmd, knowledgeGraphValidationAcceptCmd, knowledgeGraphValidationRunCmd, knowledgeGraphValidationStepCmd, knowledgeGraphValidationEvidenceCmd, knowledgeGraphValidationCompletionCmd}
	for _, command := range validationCommands {
		command.Flags().StringVarP(&graphDirectory, "directory", "d", ".", "repository directory")
		command.Flags().BoolVar(&graphJSON, "json", false, "emit machine-readable JSON")
		_ = command.MarkFlagRequired("directory")
	}
	knowledgeGraphValidationProposeCmd.Flags().StringVar(&graphTaskID, "task", "", "external task identity")
	knowledgeGraphValidationProposeCmd.Flags().StringVar(&graphRunID, "run", "", "external planning run identity")
	knowledgeGraphValidationProposeCmd.Flags().StringVar(&validationPatchHash, "patch-hash", "", "patch identity for post-change validation")
	knowledgeGraphValidationProposeCmd.Flags().StringSliceVar(&validationFiles, "files", nil, "repository-relative changed files (comma-separated or repeated)")
	knowledgeGraphValidationProposeCmd.Flags().StringSliceVar(&validationSymbolIDs, "symbol-id", nil, "stable changed symbol id (repeatable)")
	knowledgeGraphValidationProposeCmd.Flags().IntVar(&validationDepth, "max-depth", 2, "bounded traversal depth")
	knowledgeGraphValidationAcceptCmd.Flags().StringVar(&graphRunID, "run", "", "external run identity to bind")
	knowledgeGraphValidationAcceptCmd.Flags().StringSliceVar(&validationRequiredItems, "require-item", nil, "proposal item id to accept as required (repeatable)")
	knowledgeGraphValidationAcceptCmd.Flags().StringSliceVar(&validationOptionalItems, "optional-item", nil, "proposal item id to accept as optional (repeatable)")
	knowledgeGraphValidationAcceptCmd.Flags().StringSliceVar(&validationRejectedItems, "reject-item", nil, "proposal item id to reject (repeatable)")
	knowledgeGraphValidationRunCmd.Flags().StringVar(&validationMode, "mode", "read-only", "run access mode")
	knowledgeGraphValidationStepCmd.Flags().StringVar(&validationPhase, "phase", "verify", "structured phase")
	knowledgeGraphValidationStepCmd.Flags().StringVar(&validationStatus, "status", "completed", "terminal step status")
	knowledgeGraphValidationStepCmd.Flags().IntVar(&validationIteration, "iteration", 1, "step iteration")
	knowledgeGraphValidationEvidenceCmd.Flags().StringVar(&validationItemID, "item", "", "accepted validation item identity")
	knowledgeGraphValidationEvidenceCmd.Flags().StringVar(&graphRunID, "run", "", "registered validation run identity")
	knowledgeGraphValidationEvidenceCmd.Flags().StringVar(&validationStepID, "step", "", "registered run step identity")
	knowledgeGraphValidationEvidenceCmd.Flags().StringVar(&validationArtifact, "artifact", "", "optional registered artifact hash")
	knowledgeGraphValidationEvidenceCmd.Flags().StringVar(&validationEvidenceStatus, "status", "passed", "evidence status")
	knowledgeGraphValidationCompletionCmd.Flags().BoolVar(&validationFinalize, "finalize", false, "freeze the current evidence coverage")
	_ = knowledgeGraphValidationProposeCmd.MarkFlagRequired("task")
	_ = knowledgeGraphValidationEvidenceCmd.MarkFlagRequired("item")
	_ = knowledgeGraphValidationEvidenceCmd.MarkFlagRequired("run")
	_ = knowledgeGraphValidationEvidenceCmd.MarkFlagRequired("step")
	knowledgeGraphValidationCmd.AddCommand(validationCommands...)
	knowledgeGraphCmd.AddCommand(knowledgeGraphScanCmd, knowledgeGraphRefreshCmd, knowledgeGraphSymbolCmd, knowledgeGraphDependenciesCmd, knowledgeGraphCallsCmd, knowledgeGraphImpactCmd, knowledgeGraphStatusCmd, knowledgeGraphPublishCmd)
	knowledgeGraphCmd.AddCommand(knowledgeGraphValidationCmd)
	knowledgeCmd.AddCommand(knowledgeLsCmd)
	knowledgeCmd.AddCommand(knowledgeShowCmd)
	knowledgeCmd.AddCommand(knowledgeInitCmd)
	knowledgeCmd.AddCommand(knowledgeIndexCmd)
	knowledgeCmd.AddCommand(knowledgeSymbolsCmd)
	knowledgeCmd.AddCommand(knowledgeStatusCmd)
	knowledgeCmd.AddCommand(knowledgeGraphCmd)
	rootCmd.AddCommand(knowledgeCmd)
}
