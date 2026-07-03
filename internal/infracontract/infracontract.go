// Package infracontract is the CORE seam between the MCP server and the SRE/infra
// module. It lets internal/mcp expose infra tools by depending on this interface
// instead of importing internal/infra directly — so the core/module dependency
// rule holds (mcp does not import a module). The composition root (internal/cli)
// injects the real *infra.Registry, which satisfies Registry.
//
// See docs/OPENEXEC_CORE_MODULE_STRATEGY.md — the SRE/MCP adapter decision.
package infracontract

import "context"

// Command is a resolved, argv-bounded infrastructure command. internal/infra
// aliases its own Command to this type, so *infra.Registry's resolver methods
// satisfy Registry without change.
type Command struct {
	// Binary is the executable name (resolved via PATH at execution time).
	Binary string
	// Args is the strict argument array. Never joined into a shell string.
	Args []string
	// ApplyClass is true for commands that mutate infrastructure (require
	// approval); dry-runs (--check, test=True, plan) are not apply-class.
	ApplyClass bool
	// Environment and RiskProfile are recorded for audit output.
	Environment string
	RiskProfile string
}

// Runner executes a resolved command's argv (no shell). infra.ExecRunner
// satisfies it structurally.
type Runner interface {
	Run(ctx context.Context, binary string, args []string) (output string, exitCode int, err error)
}

// Registry is the infra capability the MCP server needs, deny-by-default. The
// method set matches *infra.Registry (with the DestructiveChanges helper, which
// infra exposes as a thin method over its package function).
type Registry interface {
	HasEngine(engine string) bool
	Environments() []string
	Playbooks() []string
	States() []string
	Targets() []string
	Queries() []string
	ResolveAnsiblePlaybook(env, playbook, limit string, check bool) (*Command, error)
	ResolveSaltState(env, state, target string, test bool) (*Command, error)
	ResolveSSHQuery(env, host, query string) (*Command, error)
	ResolveTerraformPlan(env string, vars map[string]string, save bool) (*Command, error)
	ResolveTerraformShowPlan(env string) (*Command, error)
	ResolveTerraformApply(env string) (*Command, error)
	// DestructiveChanges parses a terraform plan JSON and returns the resources
	// it would delete/replace (deterministic; never an LLM judgment).
	DestructiveChanges(planJSON []byte) ([]string, error)
}
