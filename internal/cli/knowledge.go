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
			if err != nil { return nil }
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
	Use:   "show [symbols|envs|api]",
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
		default:
			return fmt.Errorf("unknown type: %s. Use symbols, envs, or api", kType)
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

		cmd.Println("✓ Indexing complete. Surgical pointers recorded in knowledge.db")
		return nil
	},
}

func init() {
	knowledgeCmd.AddCommand(knowledgeLsCmd)
	knowledgeCmd.AddCommand(knowledgeShowCmd)
	knowledgeCmd.AddCommand(knowledgeInitCmd)
	knowledgeCmd.AddCommand(knowledgeIndexCmd)
	rootCmd.AddCommand(knowledgeCmd)
}
