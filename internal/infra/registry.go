package infra

import (
	"fmt"
	"github.com/openexec/openexec/internal/infracontract"
	"path"
	"path/filepath"
	"sort"
)

// Command is a fully-resolved, validated infra command: an argv array for
// exec.CommandContext. There is deliberately no way to construct one except
// through Registry resolution.
// Command is the resolved, argv-bounded infra command. It is the core
// infracontract.Command, aliased here so *Registry's resolvers satisfy the
// core infracontract.Registry seam that the MCP server depends on (mcp must not
// import this module directly).
type Command = infracontract.Command

// Registry resolves tool calls into Commands, deny-by-default: anything not
// explicitly enumerated in the validated Config is refused.
type Registry struct {
	cfg *Config
}

// Registry satisfies the core infracontract.Registry seam.
var _ infracontract.Registry = (*Registry)(nil)

// DestructiveChanges parses a terraform plan JSON and returns the resources it
// would delete/replace. A thin method over the package function so *Registry
// satisfies infracontract.Registry.
func (r *Registry) DestructiveChanges(planJSON []byte) ([]string, error) {
	return DestructiveChanges(planJSON)
}

// NewRegistry wraps a validated config. Returns an error for nil/disabled
// config so a Registry always means "infra tools are live".
func NewRegistry(cfg *Config) (*Registry, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, fmt.Errorf("infra config is nil or disabled")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Registry{cfg: cfg}, nil
}

// LoadRegistry loads the workspace config and wraps it. Returns (nil, nil)
// when the feature is absent or disabled.
func LoadRegistry(workspaceDir string) (*Registry, error) {
	cfg, err := Load(workspaceDir)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	return NewRegistry(cfg)
}

func (r *Registry) env(name string) (*EnvConfig, error) {
	ec, ok := r.cfg.Environments[name]
	if !ok {
		return nil, fmt.Errorf("environment %q is not in the allowlist (allowed: %v)", name, r.Environments())
	}
	return ec, nil
}

// Environments returns the sorted environment names (for schema enums).
func (r *Registry) Environments() []string {
	out := make([]string, 0, len(r.cfg.Environments))
	for name := range r.cfg.Environments {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// union collects sorted distinct values across environments.
func union(values func(*EnvConfig) []string, envs map[string]*EnvConfig) []string {
	seen := map[string]bool{}
	for _, ec := range envs {
		for _, v := range values(ec) {
			seen[v] = true
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Playbooks returns all allowlisted playbooks across environments (schema
// enum; per-environment membership is still enforced at resolution).
func (r *Registry) Playbooks() []string {
	return union(func(ec *EnvConfig) []string {
		if ec.Ansible == nil {
			return nil
		}
		return ec.Ansible.Playbooks
	}, r.cfg.Environments)
}

// States returns all allowlisted salt states across environments.
func (r *Registry) States() []string {
	return union(func(ec *EnvConfig) []string {
		if ec.Salt == nil {
			return nil
		}
		return ec.Salt.States
	}, r.cfg.Environments)
}

// Targets returns all allowlisted salt targets across environments.
func (r *Registry) Targets() []string {
	return union(func(ec *EnvConfig) []string {
		if ec.Salt == nil {
			return nil
		}
		return ec.Salt.Targets
	}, r.cfg.Environments)
}

// Queries returns all allowlisted ssh query names across environments.
func (r *Registry) Queries() []string {
	return union(func(ec *EnvConfig) []string {
		if ec.SSH == nil {
			return nil
		}
		names := make([]string, 0, len(ec.SSH.Queries))
		for n := range ec.SSH.Queries {
			names = append(names, n)
		}
		return names
	}, r.cfg.Environments)
}

// TerraformVariables returns all allowlisted -var names across environments.
func (r *Registry) TerraformVariables() []string {
	return union(func(ec *EnvConfig) []string {
		if ec.Terraform == nil {
			return nil
		}
		return ec.Terraform.AllowedVariables
	}, r.cfg.Environments)
}

// HasEngine reports whether any environment configures the given engine
// ("ansible", "salt", "ssh", "terraform") — used to decide which MCP tools
// to advertise.
func (r *Registry) HasEngine(engine string) bool {
	for _, ec := range r.cfg.Environments {
		switch engine {
		case "ansible":
			if ec.Ansible != nil {
				return true
			}
		case "salt":
			if ec.Salt != nil {
				return true
			}
		case "ssh":
			if ec.SSH != nil {
				return true
			}
		case "terraform":
			if ec.Terraform != nil {
				return true
			}
		}
	}
	return false
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ResolveAnsiblePlaybook validates and builds an ansible-playbook command.
// check=true runs --check (dry-run, not apply-class).
func (r *Registry) ResolveAnsiblePlaybook(env, playbook, limit string, check bool) (*Command, error) {
	ec, err := r.env(env)
	if err != nil {
		return nil, err
	}
	a := ec.Ansible
	if a == nil {
		return nil, fmt.Errorf("environment %q does not allow ansible", env)
	}
	if !contains(a.Playbooks, playbook) {
		return nil, fmt.Errorf("playbook %q is not in the %q allowlist (allowed: %v)", playbook, env, a.Playbooks)
	}
	// The playbook is a validated basename from the enum; it is joined
	// against the operator-configured dir only after the membership check,
	// so a caller can never address a path of their choosing.
	playbookPath := filepath.Join(a.PlaybookDir, playbook)

	args := []string{"-i", a.Inventory, playbookPath}
	if limit != "" {
		if !limitRe.MatchString(limit) {
			return nil, fmt.Errorf("limit %q does not match the allowed pattern %s", limit, limitRe)
		}
		args = append(args, "--limit", limit)
	}
	if check {
		args = append(args, "--check")
	}
	return &Command{
		Binary:      "ansible-playbook",
		Args:        args,
		ApplyClass:  !check,
		Environment: env,
		RiskProfile: ec.RiskProfile,
	}, nil
}

// ResolveSaltState validates and builds a salt state.apply command.
// test=true appends test=True (dry-run, not apply-class).
func (r *Registry) ResolveSaltState(env, state, target string, test bool) (*Command, error) {
	ec, err := r.env(env)
	if err != nil {
		return nil, err
	}
	s := ec.Salt
	if s == nil {
		return nil, fmt.Errorf("environment %q does not allow salt", env)
	}
	if !contains(s.States, state) {
		return nil, fmt.Errorf("salt state %q is not in the %q allowlist (allowed: %v)", state, env, s.States)
	}
	if !contains(s.Targets, target) {
		return nil, fmt.Errorf("salt target %q is not in the %q allowlist (allowed: %v)", target, env, s.Targets)
	}
	args := []string{target, "state.apply", state}
	if test {
		args = append(args, "test=True")
	}
	return &Command{
		Binary:      "salt",
		Args:        args,
		ApplyClass:  !test,
		Environment: env,
		RiskProfile: ec.RiskProfile,
	}, nil
}

// ResolveSSHQuery validates and builds an ssh diagnostic command. SSH
// queries are read-only by contract: the remote argv comes from operator
// config, the caller only selects a query name and a host.
func (r *Registry) ResolveSSHQuery(env, host, query string) (*Command, error) {
	ec, err := r.env(env)
	if err != nil {
		return nil, err
	}
	sh := ec.SSH
	if sh == nil {
		return nil, fmt.Errorf("environment %q does not allow ssh", env)
	}
	// Reject anything that could be parsed as an ssh option (leading '-')
	// or escape a hostname position — before glob matching.
	if !hostRe.MatchString(host) {
		return nil, fmt.Errorf("host %q does not match the allowed pattern %s", host, hostRe)
	}
	matched := false
	for _, pattern := range sh.AllowedHosts {
		if ok, _ := path.Match(pattern, host); ok {
			matched = true
			break
		}
	}
	if !matched {
		return nil, fmt.Errorf("host %q does not match any allowed host pattern in %q (%v)", host, env, sh.AllowedHosts)
	}
	argv, ok := sh.Queries[query]
	if !ok {
		names := make([]string, 0, len(sh.Queries))
		for n := range sh.Queries {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("ssh query %q is not in the %q allowlist (allowed: %v)", query, env, names)
	}
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", host}
	args = append(args, argv...)
	return &Command{
		Binary:      "ssh",
		Args:        args,
		ApplyClass:  false,
		Environment: env,
		RiskProfile: ec.RiskProfile,
	}, nil
}

// terraformPlanFile is the fixed, per-environment saved-plan filename
// (relative to the terraform working dir via -chdir). It is deliberately
// NOT caller-suppliable: the apply tool can only ever consume the plan the
// plan tool wrote, which is the TOCTOU guarantee of the saved-plan pipeline.
func terraformPlanFile(env string) string {
	return "openexec-" + env + ".tfplan"
}

// ResolveTerraformPlan validates and builds a terraform plan command.
// Plan is read-class: it refreshes and renders changes but mutates nothing
// (save=true additionally writes the local saved-plan file for
// ResolveTerraformApply to consume).
func (r *Registry) ResolveTerraformPlan(env string, vars map[string]string, save bool) (*Command, error) {
	ec, err := r.env(env)
	if err != nil {
		return nil, err
	}
	tf := ec.Terraform
	if tf == nil {
		return nil, fmt.Errorf("environment %q does not allow terraform", env)
	}
	args := []string{"-chdir=" + tf.WorkingDir, "plan", "-input=false", "-no-color", "-lock-timeout=30s"}
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic argv for tests and audit logs
	for _, name := range names {
		if !contains(tf.AllowedVariables, name) {
			return nil, fmt.Errorf("terraform variable %q is not in the %q allowlist (allowed: %v)", name, env, tf.AllowedVariables)
		}
		value := vars[name]
		if !tfVarValueRe.MatchString(value) {
			return nil, fmt.Errorf("terraform variable %q has a value with control characters", name)
		}
		// One argv element per -var: the value cannot become a separate
		// argument or option no matter what it contains.
		args = append(args, "-var="+name+"="+value)
	}
	if save {
		args = append(args, "-out="+terraformPlanFile(env))
	}
	return &Command{
		Binary:      "terraform",
		Args:        args,
		ApplyClass:  false,
		Environment: env,
		RiskProfile: ec.RiskProfile,
	}, nil
}

// ResolveTerraformShowPlan builds the `terraform show -json <planfile>`
// command used to deterministically inspect the saved plan before apply.
// Read-class.
func (r *Registry) ResolveTerraformShowPlan(env string) (*Command, error) {
	ec, err := r.env(env)
	if err != nil {
		return nil, err
	}
	tf := ec.Terraform
	if tf == nil {
		return nil, fmt.Errorf("environment %q does not allow terraform", env)
	}
	return &Command{
		Binary:      "terraform",
		Args:        []string{"-chdir=" + tf.WorkingDir, "show", "-json", terraformPlanFile(env)},
		ApplyClass:  false,
		Environment: env,
		RiskProfile: ec.RiskProfile,
	}, nil
}

// ResolveTerraformApply builds the saved-plan apply (Phase 2). It only
// ever applies the fixed per-environment plan file — there is no way to
// pass variables, targets, or a different plan path, and terraform itself
// refuses a stale plan if state drifted since planning (the TOCTOU fix).
// Apply-class: requires approval upstream.
func (r *Registry) ResolveTerraformApply(env string) (*Command, error) {
	ec, err := r.env(env)
	if err != nil {
		return nil, err
	}
	tf := ec.Terraform
	if tf == nil {
		return nil, fmt.Errorf("environment %q does not allow terraform", env)
	}
	return &Command{
		Binary:      "terraform",
		Args:        []string{"-chdir=" + tf.WorkingDir, "apply", "-input=false", "-no-color", "-lock-timeout=30s", terraformPlanFile(env)},
		ApplyClass:  true,
		Environment: env,
		RiskProfile: ec.RiskProfile,
	}, nil
}
