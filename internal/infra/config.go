// Package infra implements the config-driven SRE command registry described
// in docs/SRE_ORCHESTRATION_ROADMAP.md (Phase 1).
//
// Safety model: the registry is deny-by-default. Only commands explicitly
// enumerated in the operator-curated config file can ever be resolved, every
// caller-supplied parameter is validated against an enum or a strict regex
// (validate-and-reject — hostile input is refused, never sanitized), and
// resolution produces argv arrays for exec.CommandContext so no shell
// interpreter is ever involved. Destructive verbs (terraform destroy, raw
// salt cmd.run, arbitrary ssh commands) simply do not exist in the action
// space.
package infra

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFileName is the operator-curated allowlist, relative to the
// workspace root. It lives in its own file — NOT in the init-generated
// openexec.yaml — so that `openexec init` can never overwrite a security
// allowlist and so reviewers can audit it in isolation.
const ConfigFileName = ".openexec/infra.yaml"

// Config is the root of the infra allowlist file.
type Config struct {
	// Enabled gates the whole feature; false (or a missing file) means no
	// infra tools are ever advertised or resolvable.
	Enabled bool `yaml:"enabled"`

	// Environments maps an environment name (e.g. "staging", "production")
	// to its allowlist. The environment is a first-class parameter of every
	// infra tool call.
	Environments map[string]*EnvConfig `yaml:"environments"`
}

// EnvConfig is the per-environment allowlist. Engines left nil are not
// available in that environment (deny-by-default).
type EnvConfig struct {
	// RiskProfile is "low" or "high". Phase 1 records it for audit output;
	// Phase 3 uses it to tier approval policy per environment.
	RiskProfile string `yaml:"risk_profile"`

	Ansible   *AnsibleConfig   `yaml:"ansible,omitempty"`
	Salt      *SaltConfig      `yaml:"salt,omitempty"`
	SSH       *SSHConfig       `yaml:"ssh,omitempty"`
	Terraform *TerraformConfig `yaml:"terraform,omitempty"`
}

// AnsibleConfig allowlists playbooks for ansible-playbook.
type AnsibleConfig struct {
	// PlaybookDir is the directory playbooks are resolved from. It SHOULD
	// point outside the agent-writable workspace (e.g. /etc/openexec/playbooks
	// or a read-only checkout) — see "Indirect Payload Generation" in the
	// roadmap: if agents can edit an allowlisted playbook, the allowlist
	// only constrains the filename, not the payload.
	PlaybookDir string `yaml:"playbook_dir"`

	// Inventory is the fixed inventory path passed as -i. Operator-set,
	// never caller-supplied.
	Inventory string `yaml:"inventory"`

	// Playbooks are allowed playbook file basenames (no path separators).
	Playbooks []string `yaml:"playbooks"`
}

// SaltConfig allowlists salt states and targets.
type SaltConfig struct {
	// Targets are the exact target strings callers may select (a caller
	// must pick one verbatim; the strings themselves may use salt globs
	// at the operator's discretion).
	Targets []string `yaml:"targets"`

	// States are allowed state names for state.apply.
	States []string `yaml:"states"`
}

// SSHConfig allowlists read-only diagnostic queries over ssh.
type SSHConfig struct {
	// AllowedHosts are glob patterns (path.Match syntax) a caller-supplied
	// host must match, e.g. "10.0.1.*" or "*.staging.internal".
	AllowedHosts []string `yaml:"allowed_hosts"`

	// Queries maps a query name to the remote argv it runs, e.g.
	// check_disk: ["df", "-h"]. The argv is operator-trusted config;
	// callers can only select a name.
	Queries map[string][]string `yaml:"queries"`
}

// TerraformConfig allowlists terraform plan inputs.
type TerraformConfig struct {
	// WorkingDir is passed via -chdir. Operator-set, never caller-supplied.
	WorkingDir string `yaml:"working_dir"`

	// AllowedVariables are -var names callers may set.
	AllowedVariables []string `yaml:"allowed_variables"`
}

// Validation patterns. All caller-facing identifiers must start with an
// alphanumeric so no value can ever be parsed as a command-line option
// (e.g. an ssh "host" of -oProxyCommand=... is an option-injection attack).
var (
	envNameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	basenameRe  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	saltStateRe = regexp.MustCompile(`^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)*$`)
	queryNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	// hostRe covers IPs and hostnames; the leading alphanumeric blocks
	// option injection, and there is no slash or whitespace.
	hostRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	// limitRe covers ansible --limit patterns (host globs, groups, unions,
	// exclusions) without shell metacharacters. Argv isolation already
	// prevents shell injection; the regex narrows the surface anyway.
	limitRe     = regexp.MustCompile(`^[a-zA-Z0-9_.:*,!&-]+$`)
	tfVarNameRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	// tfVarValueRe rejects control characters (incl. newlines). The value
	// rides inside a single "-var=name=value" argv element, so it cannot
	// become a separate argument, but it must not corrupt logs or files.
	tfVarValueRe = regexp.MustCompile(`^[^\x00-\x1f\x7f]*$`)
)

// Load reads and validates the infra config for a workspace. It returns
// (nil, nil) when the file does not exist or the feature is disabled —
// callers treat nil as "no infra tools". Any malformed or invalid file is
// an error: a security allowlist must fail loud, never half-load.
func Load(workspaceDir string) (*Config, error) {
	path := filepath.Join(workspaceDir, filepath.FromSlash(ConfigFileName))
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ConfigFileName, err)
	}

	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// Unknown keys in a security allowlist are almost certainly typos
	// (e.g. "playboks:") that would silently widen or narrow the policy.
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ConfigFileName, err)
	}
	if !cfg.Enabled {
		return nil, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", ConfigFileName, err)
	}
	return &cfg, nil
}

// Validate checks every enumerated value against the strict patterns above.
func (c *Config) Validate() error {
	if len(c.Environments) == 0 {
		return fmt.Errorf("enabled but no environments defined")
	}
	for env, ec := range c.Environments {
		if !envNameRe.MatchString(env) {
			return fmt.Errorf("environment name %q: must match %s", env, envNameRe)
		}
		if ec == nil {
			return fmt.Errorf("environment %q: empty definition", env)
		}
		if ec.RiskProfile != "low" && ec.RiskProfile != "high" {
			return fmt.Errorf("environment %q: risk_profile must be \"low\" or \"high\", got %q", env, ec.RiskProfile)
		}
		if a := ec.Ansible; a != nil {
			if a.PlaybookDir == "" {
				return fmt.Errorf("environment %q: ansible.playbook_dir is required", env)
			}
			if a.Inventory == "" {
				return fmt.Errorf("environment %q: ansible.inventory is required", env)
			}
			for _, p := range a.Playbooks {
				if !basenameRe.MatchString(p) || strings.Contains(p, "/") {
					return fmt.Errorf("environment %q: playbook %q must be a plain file basename", env, p)
				}
			}
		}
		if s := ec.Salt; s != nil {
			for _, st := range s.States {
				if !saltStateRe.MatchString(st) {
					return fmt.Errorf("environment %q: salt state %q: must match %s", env, st, saltStateRe)
				}
			}
			for _, tgt := range s.Targets {
				if tgt == "" || strings.HasPrefix(tgt, "-") {
					return fmt.Errorf("environment %q: salt target %q: must be non-empty and not start with '-'", env, tgt)
				}
			}
		}
		if sh := ec.SSH; sh != nil {
			if len(sh.AllowedHosts) == 0 && len(sh.Queries) > 0 {
				return fmt.Errorf("environment %q: ssh.allowed_hosts is required when queries are defined", env)
			}
			for name, argv := range sh.Queries {
				if !queryNameRe.MatchString(name) {
					return fmt.Errorf("environment %q: ssh query name %q: must match %s", env, name, queryNameRe)
				}
				if len(argv) == 0 {
					return fmt.Errorf("environment %q: ssh query %q: empty command", env, name)
				}
			}
		}
		if tf := ec.Terraform; tf != nil {
			if tf.WorkingDir == "" {
				return fmt.Errorf("environment %q: terraform.working_dir is required", env)
			}
			for _, v := range tf.AllowedVariables {
				if !tfVarNameRe.MatchString(v) {
					return fmt.Errorf("environment %q: terraform variable %q: must match %s", env, v, tfVarNameRe)
				}
			}
		}
	}
	return nil
}
