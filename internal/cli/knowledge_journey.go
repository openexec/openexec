package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/graphjourney"
	"github.com/spf13/cobra"
)

var (
	journeyBaseURL   string
	journeyTokenFile string
	journeyQuery     string
	journeyEvidence  string
	journeyRequery   string
	journeyCommit    string
	journeyPrepare   bool
	journeyPrepareIn time.Duration
)

// knowledgeGraphJourneyCmd is the reviewed graph deploy's live acceptance
// check, not an owner command: it walks a running daemon and writes the
// evidence file the deploy keeps.
var knowledgeGraphJourneyCmd = &cobra.Command{
	Use:    "journey",
	Short:  "Walk the Explore journey against a running repository-graph daemon",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := readGraphBearer(journeyTokenFile)
		if err != nil {
			return err
		}
		cfg := graphjourney.Config{BaseURL: journeyBaseURL, Token: token, Query: journeyQuery, WantCommit: journeyCommit}
		var evidence graphjourney.Evidence
		var walkErr error
		if journeyPrepare && journeyRequery != "" {
			return fmt.Errorf("--prepare and --requery are separate deploy steps")
		}
		if journeyPrepare {
			evidence, walkErr = graphjourney.Prepare(cmd.Context(), cfg, journeyPrepareIn)
		} else if journeyRequery != "" {
			raw, err := os.ReadFile(journeyRequery)
			if err != nil {
				return err
			}
			var prior graphjourney.Evidence
			if err := json.Unmarshal(raw, &prior); err != nil {
				return fmt.Errorf("prior journey evidence %s: %w", journeyRequery, err)
			}
			evidence, walkErr = graphjourney.Requery(cmd.Context(), cfg, prior)
		} else {
			evidence, walkErr = graphjourney.Walk(cmd.Context(), cfg)
		}
		encoded, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			return err
		}
		if journeyEvidence != "" {
			if err := os.WriteFile(journeyEvidence, append(encoded, '\n'), 0o600); err != nil {
				return err
			}
		} else {
			cmd.Println(string(encoded))
		}
		return walkErr
	},
}

// readGraphBearer accepts either a bare token file or the daemon's own
// EnvironmentFile, so the check authenticates exactly as the service does.
// An empty path falls back to the variable the daemon itself reads.
func readGraphBearer(path string) (string, error) {
	const name = "OPENEXEC_REPOSITORY_GRAPH_TOKEN"
	if path == "" {
		if token := strings.TrimSpace(os.Getenv(name)); token != "" {
			return token, nil
		}
		return "", fmt.Errorf("graph bearer required: pass --token-file or set %s", name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(string(raw))
	if !strings.Contains(content, "=") {
		if content == "" {
			return "", fmt.Errorf("%s holds no graph bearer", path)
		}
		return content, nil
	}
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(strings.TrimPrefix(key, "export ")) == name {
			if token := strings.Trim(strings.TrimSpace(value), `"'`); token != "" {
				return token, nil
			}
		}
	}
	return "", fmt.Errorf("%s does not set %s", path, name)
}

func init() {
	knowledgeGraphJourneyCmd.Flags().StringVar(&journeyBaseURL, "base-url", "http://127.0.0.1:8765", "running daemon to walk")
	knowledgeGraphJourneyCmd.Flags().StringVar(&journeyTokenFile, "token-file", "", "bare graph bearer file or the daemon's EnvironmentFile")
	knowledgeGraphJourneyCmd.Flags().StringVar(&journeyQuery, "query", "Execute", "symbol search the journey starts from")
	knowledgeGraphJourneyCmd.Flags().StringVar(&journeyEvidence, "evidence", "", "write journey evidence JSON here instead of stdout")
	knowledgeGraphJourneyCmd.Flags().StringVar(&journeyRequery, "requery", "", "re-query the identity held in this earlier evidence file")
	knowledgeGraphJourneyCmd.Flags().BoolVar(&journeyPrepare, "prepare", false, "only bring the daemon's graph current before the journey is walked")
	knowledgeGraphJourneyCmd.Flags().DurationVar(&journeyPrepareIn, "prepare-within", 5*time.Minute, "how long --prepare waits for a refresh the daemon is still running")
	knowledgeGraphJourneyCmd.Flags().StringVar(&journeyCommit, "want-commit", "", "refuse a daemon that does not report this build commit")
	knowledgeGraphCmd.AddCommand(knowledgeGraphJourneyCmd)
}
