package cli

// Operator-session identity binding. An OPENEXEC_OPERATOR_SESSION=1 env var is
// spoofable by any process that can run the CLI, so it is not a defensible
// identity boundary. When an operator allowlist is configured, an operator
// session instead requires the current `gh`-authenticated GitHub login (backed
// by gh's OAuth token) to be listed. The env var survives only as a documented
// dev/pilot shortcut used when no allowlist exists.

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openexec/openexec/internal/governance/connectors/github"
)

// operatorConfig is the operator-owned allowlist of GitHub logins permitted to
// run approval/merge/risk-accept operations. It lives in ~/.openexec (outside
// any agent-writable workspace) so an agent cannot add itself.
type operatorConfig struct {
	Operators []string `yaml:"operators"`
}

// loadOperators reads ~/.openexec/operators.yaml. A missing file yields nil
// (no allowlist configured → env-var dev fallback applies).
func loadOperators() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".openexec", "operators.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg operatorConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg.Operators, nil
}

// resolveOperatorSession decides whether this invocation is a human operator
// session and records how it was established. With an operator allowlist present
// the session is bound to the verifiable `gh` login (a spoofable env var is
// ignored); without one it falls back to OPENEXEC_OPERATOR_SESSION=1 as a
// dev/pilot shortcut. The returned identity string is stamped into the audit
// trail on approval/merge/risk-accept.
func resolveOperatorSession(ctx context.Context) (isOperator bool, identity string) {
	operators, err := loadOperators()
	if err == nil && len(operators) > 0 {
		login := currentGitHubLogin(ctx)
		if login == "" {
			return false, "no gh identity (run `gh auth login`); operator allowlist is configured"
		}
		for _, o := range operators {
			if strings.EqualFold(strings.TrimSpace(o), login) {
				return true, "github:" + login
			}
		}
		return false, "github:" + login + " (not in operator allowlist)"
	}
	if os.Getenv("OPENEXEC_OPERATOR_SESSION") == "1" {
		return true, "dev:env-var"
	}
	return false, ""
}

// currentGitHubLogin returns the login of the `gh`-authenticated user, or "" if
// gh is unavailable / unauthenticated.
func currentGitHubLogin(ctx context.Context) string {
	out, err := (&github.ExecRunner{}).Run(ctx, "api", "user", "--jq", ".login")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
