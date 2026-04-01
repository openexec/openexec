package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/spf13/cobra"
)

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Manage and inspect the Deterministic Knowledge Base",
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
	knowledgeCmd.AddCommand(knowledgeLsCmd)
	knowledgeCmd.AddCommand(knowledgeShowCmd)
	knowledgeCmd.AddCommand(knowledgeInitCmd)
	knowledgeCmd.AddCommand(knowledgeIndexCmd)
	knowledgeCmd.AddCommand(knowledgeSymbolsCmd)
	knowledgeCmd.AddCommand(knowledgeStatusCmd)
	rootCmd.AddCommand(knowledgeCmd)
}
