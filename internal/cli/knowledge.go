package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/repository"
	"github.com/spf13/cobra"
)

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Manage and inspect the Deterministic Knowledge Base",
}

var (
	graphDirectory        string
	graphJSON             bool
	graphFile             string
	graphKind             string
	graphRead             bool
	graphReverse          bool
	graphDepth            int
	graphImpactDepth      int
	graphConsoleURL       string
	graphConsoleProject   string
	graphConsoleTokenFile string
	graphSymbols          []string
	graphTaskID           string
	graphRunID            string
	graphPlanID           string
)

var knowledgeGraphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Inspect the versioned repository pointer graph",
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
	Use:   "impact SYMBOL_ID...",
	Short: "Explain a bounded affected set and validation recommendations",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, identity, err := openGraphStore(cmd.Context(), graphDirectory)
		if err != nil {
			return err
		}
		defer store.Close()
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
	projection, err := store.BuildRepositoryContext(ctx, identity, symbols, taskID, runID, planID, nil)
	if err != nil {
		return knowledge.RepositoryContextProjection{}, err
	}
	if err := publisher.Publish(ctx, consoleURL, consoleProject, token, projection); err != nil {
		return knowledge.RepositoryContextProjection{}, err
	}
	return projection, nil
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
	for _, command := range []*cobra.Command{knowledgeGraphScanCmd, knowledgeGraphRefreshCmd, knowledgeGraphSymbolCmd, knowledgeGraphDependenciesCmd, knowledgeGraphImpactCmd, knowledgeGraphStatusCmd, knowledgeGraphPublishCmd} {
		command.Flags().StringVarP(&graphDirectory, "directory", "d", ".", "repository directory")
		command.Flags().BoolVar(&graphJSON, "json", false, "emit machine-readable JSON")
	}
	knowledgeGraphSymbolCmd.Flags().StringVar(&graphFile, "file", "", "repository-relative file filter")
	knowledgeGraphSymbolCmd.Flags().StringVar(&graphKind, "kind", "", "symbol kind filter")
	knowledgeGraphSymbolCmd.Flags().BoolVar(&graphRead, "read", false, "read the hash-validated source range")
	knowledgeGraphDependenciesCmd.Flags().BoolVar(&graphReverse, "reverse", false, "show module dependants")
	knowledgeGraphDependenciesCmd.Flags().IntVar(&graphDepth, "depth", 1, "bounded traversal depth")
	knowledgeGraphImpactCmd.Flags().IntVar(&graphImpactDepth, "depth", 2, "bounded traversal depth")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphConsoleURL, "console-url", "", "Agent Console base URL")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphConsoleProject, "console-project", "", "Agent Console checkout ID")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphConsoleTokenFile, "console-token-file", "", "file containing the Agent Console bearer token (or export AGENT_CONSOLE_TOKEN)")
	knowledgeGraphPublishCmd.Flags().StringSliceVar(&graphSymbols, "symbol", nil, "symbol name to include (repeatable)")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphTaskID, "task", "", "OpenExec task reference")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphRunID, "run", "", "OpenExec run reference")
	knowledgeGraphPublishCmd.Flags().StringVar(&graphPlanID, "plan", "", "OpenExec validation-plan reference")
	_ = knowledgeGraphPublishCmd.MarkFlagRequired("console-url")
	_ = knowledgeGraphPublishCmd.MarkFlagRequired("console-project")
	knowledgeGraphCmd.AddCommand(knowledgeGraphScanCmd, knowledgeGraphRefreshCmd, knowledgeGraphSymbolCmd, knowledgeGraphDependenciesCmd, knowledgeGraphImpactCmd, knowledgeGraphStatusCmd, knowledgeGraphPublishCmd)
	knowledgeCmd.AddCommand(knowledgeLsCmd)
	knowledgeCmd.AddCommand(knowledgeShowCmd)
	knowledgeCmd.AddCommand(knowledgeInitCmd)
	knowledgeCmd.AddCommand(knowledgeIndexCmd)
	knowledgeCmd.AddCommand(knowledgeSymbolsCmd)
	knowledgeCmd.AddCommand(knowledgeStatusCmd)
	knowledgeCmd.AddCommand(knowledgeGraphCmd)
	rootCmd.AddCommand(knowledgeCmd)
}
