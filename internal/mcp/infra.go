package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/infracontract"
)

// infraConfigFileName is the operator allowlist path, surfaced in "not enabled"
// errors. Mirrors infra.ConfigFileName without importing the module.
const infraConfigFileName = ".openexec/infra.yaml"

// Infra tools expose the deny-by-default SRE command registry
// (internal/infra, configured via .openexec/infra.yaml) as parameter-bounded
// MCP tools. See docs/SRE_ORCHESTRATION_ROADMAP.md Phase 1.
//
// Safety model recap:
//   - Tools exist only when the operator wrote an allowlist; every parameter
//     is validated against that allowlist (enum/regex, validate-and-reject).
//   - Execution is exec.CommandContext argv arrays — no shell, ever.
//   - Apply-class invocations (a real playbook run, a real state.apply)
//     additionally require human sign-off through the approval gate and FAIL
//     CLOSED when no gate is wired. Dry-runs (--check, test=True, plan) are
//     read-class and do not.
//   - Like run_shell_command, infra executions are deliberately NOT
//     idempotency-eligible: re-running on resume is safer than skipping over
//     partial side effects.
//
// Infra tools are a separate capability plane from the coding toolsets:
// the broker exempts them from toolset filtering (their own registry is a
// stricter boundary) but restricts them to danger-full-access mode.

// infraOutputLimit truncates command output returned to the model.
const infraOutputLimit = 16000

// isInfraTool reports whether a tool name belongs to the infra plane.
func isInfraTool(name string) bool {
	switch name {
	case "ansible_run_playbook", "salt_apply_state", "ssh_run_query", "terraform_plan", "terraform_apply":
		return true
	}
	return false
}

// SetInfraRegistry enables the infra tools, backed by the given registry.
// A nil runner gets the production ExecRunner.
func (s *Server) SetInfraRegistry(reg infracontract.Registry) {
	s.infraRegistry = reg
}

// SetInfraRunner overrides the command runner (tests).
func (s *Server) SetInfraRunner(r infracontract.Runner) {
	s.infraRunner = r
}

// --- Request structs (audited against schemas in schema_audit_test.go) ---

type AnsibleRunPlaybookRequest struct {
	Environment string `json:"environment"`
	Playbook    string `json:"playbook"`
	Limit       string `json:"limit,omitempty"`
	Check       bool   `json:"check,omitempty"`
}

type SaltApplyStateRequest struct {
	Environment string `json:"environment"`
	State       string `json:"state"`
	Target      string `json:"target"`
	Test        bool   `json:"test,omitempty"`
}

type SSHRunQueryRequest struct {
	Environment string `json:"environment"`
	Host        string `json:"host"`
	Query       string `json:"query"`
}

type TerraformPlanRequest struct {
	Environment string            `json:"environment"`
	Vars        map[string]string `json:"vars,omitempty"`
	Save        bool              `json:"save,omitempty"`
}

type TerraformApplyRequest struct {
	Environment string `json:"environment"`
}

// --- Tool definitions (enums compiled from the operator's allowlist) ---

func enumProp(desc string, values []string) map[string]interface{} {
	p := map[string]interface{}{
		"type":        "string",
		"description": desc,
	}
	if len(values) > 0 {
		p["enum"] = values
	}
	return p
}

func AnsibleRunPlaybookToolDef(reg infracontract.Registry) map[string]interface{} {
	return map[string]interface{}{
		"name":        "ansible_run_playbook",
		"description": "Run a pre-approved Ansible playbook from the operator allowlist against a configured environment. Apply-class: a real run requires human approval through the approval gate; set check=true for a --check dry-run that needs no approval. Playbooks outside the allowlist do not exist as far as this tool is concerned.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"environment": enumProp("Target environment from the allowlist; playbook availability is checked per environment.", reg.Environments()),
				"playbook":    enumProp("Playbook file basename from the allowlist; resolved against the operator-configured playbook directory.", reg.Playbooks()),
				"limit": map[string]interface{}{
					"type":        "string",
					"description": "Optional ansible --limit host pattern (letters, digits, and .:*,!&- only); rejected if it contains anything else.",
				},
				"check": map[string]interface{}{
					"type":        "boolean",
					"description": "Run with --check (dry-run). Dry-runs are read-class and skip the approval gate.",
				},
			},
			"required": []string{"environment", "playbook"},
		},
	}
}

func SaltApplyStateToolDef(reg infracontract.Registry) map[string]interface{} {
	return map[string]interface{}{
		"name":        "salt_apply_state",
		"description": "Apply a pre-approved Salt state to an allowlisted target in a configured environment. Apply-class: a real run requires human approval through the approval gate; set test=true for a test=True dry-run that needs no approval. Targets and states outside the allowlist are refused.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"environment": enumProp("Target environment from the allowlist.", reg.Environments()),
				"state":       enumProp("Salt state name from the allowlist, applied via state.apply.", reg.States()),
				"target":      enumProp("Salt target selector; must be one of the operator-allowlisted target strings verbatim.", reg.Targets()),
				"test": map[string]interface{}{
					"type":        "boolean",
					"description": "Run with test=True (dry-run). Dry-runs are read-class and skip the approval gate.",
				},
			},
			"required": []string{"environment", "state", "target"},
		},
	}
}

func SSHRunQueryToolDef(reg infracontract.Registry) map[string]interface{} {
	return map[string]interface{}{
		"name":        "ssh_run_query",
		"description": "Run a pre-approved read-only diagnostic query (e.g. disk or service status) over SSH on a host matching the environment's allowed host patterns. The remote command comes from operator config — only the query name and host are selectable, so this tool cannot execute arbitrary commands.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"environment": enumProp("Target environment from the allowlist.", reg.Environments()),
				"host": map[string]interface{}{
					"type":        "string",
					"description": "Hostname or IP to query; must match one of the environment's allowed host glob patterns.",
				},
				"query": enumProp("Diagnostic query name from the allowlist; maps to a fixed operator-configured command.", reg.Queries()),
			},
			"required": []string{"environment", "host", "query"},
		},
	}
}

func TerraformPlanToolDef(reg infracontract.Registry) map[string]interface{} {
	return map[string]interface{}{
		"name":        "terraform_plan",
		"description": "Run terraform plan in the environment's configured working directory with optional allowlisted variables. Read-class: plan renders pending changes without mutating infrastructure. Set save=true to write the per-environment saved plan file that terraform_apply consumes — apply can only ever run a saved plan.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"environment": enumProp("Target environment from the allowlist; determines the terraform working directory.", reg.Environments()),
				"vars": map[string]interface{}{
					"type":                 "object",
					"description":          "Optional terraform input variables as name/value string pairs; every name must be in the environment's allowed_variables list.",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"save": map[string]interface{}{
					"type":        "boolean",
					"description": "Also write the fixed per-environment plan file (-out) so terraform_apply can later apply exactly this plan.",
				},
			},
			"required": []string{"environment"},
		},
	}
}

func TerraformApplyToolDef(reg infracontract.Registry) map[string]interface{} {
	return map[string]interface{}{
		"name":        "terraform_apply",
		"description": "Apply the previously saved terraform plan for an environment (created by terraform_plan with save=true). TOCTOU-safe: only the fixed saved plan file can be applied — no variables, targets, or paths are accepted, and terraform refuses a stale plan if state drifted. The saved plan is first inspected via `terraform show -json` and destructive actions (delete/replace) are detected deterministically in Go and surfaced to the approver. Apply-class: requires human approval through the approval gate.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"environment": enumProp("Target environment from the allowlist whose saved plan should be applied.", reg.Environments()),
			},
			"required": []string{"environment"},
		},
	}
}

// InfraToolDefs returns the tool definitions for every engine the operator
// configured (engines with no config in any environment are not advertised).
func InfraToolDefs(reg infracontract.Registry) []interface{} {
	var defs []interface{}
	if reg.HasEngine("ansible") {
		defs = append(defs, AnsibleRunPlaybookToolDef(reg))
	}
	if reg.HasEngine("salt") {
		defs = append(defs, SaltApplyStateToolDef(reg))
	}
	if reg.HasEngine("ssh") {
		defs = append(defs, SSHRunQueryToolDef(reg))
	}
	if reg.HasEngine("terraform") {
		defs = append(defs, TerraformPlanToolDef(reg), TerraformApplyToolDef(reg))
	}
	return defs
}

// --- Handlers ---

// infraEnabled guards every infra handler: a client can always *name* a
// tool, so a server without a loaded allowlist must refuse explicitly
// rather than dereference a nil registry.
func (s *Server) infraEnabled(req Request) bool {
	if s.infraRegistry == nil || s.infraRunner == nil {
		s.writeToolError(req.ID, "infra tools are not enabled: no "+infraConfigFileName+" allowlist is loaded in this workspace")
		return false
	}
	return true
}

// runInfraCommand is the single execution path for all infra tools:
// approval for apply-class, then argv execution, then a structured result.
// destructive carries deterministically-detected destructive changes (the
// Phase-2 terraform plan gate) so the approver sees exactly what would be
// deleted or replaced.
func (s *Server) runInfraCommand(req Request, toolName string, args json.RawMessage, cmd *infracontract.Command, destructive []string) {
	// Phase-3 environment tiering: an environment the operator explicitly
	// marked risk_profile=low (e.g. staging) runs apply-class commands
	// autonomously — EXCEPT when the deterministic plan gate found
	// destructive changes, which always require a human regardless of tier.
	needsApproval := cmd.ApplyClass && (cmd.RiskProfile != "low" || len(destructive) > 0)

	if needsApproval {
		// FAIL CLOSED: an apply-class infra command without an operational
		// approval channel is refused, not waved through. (Contrast with
		// RequestToolApproval's permissive nil-gate default for coding
		// tools — infrastructure mutation gets the strict variant.)
		if GetApprovalGateForServer(s) == nil {
			msg := fmt.Sprintf(
				"%s: this is an apply-class infrastructure command and no approval gate is wired on this server — refusing (fail-closed). Run a dry-run instead (check/test), or use a session with an operator approval channel.", toolName)
			if len(destructive) > 0 {
				msg += "\nDestructive changes detected in the saved plan:\n  " + strings.Join(destructive, "\n  ")
			}
			s.writeToolError(req.ID, msg)
			return
		}
		var argsMap map[string]interface{}
		_ = json.Unmarshal(args, &argsMap)
		if argsMap == nil {
			argsMap = map[string]interface{}{}
		}
		// Surface the deterministic findings to the human approver.
		if len(destructive) > 0 {
			argsMap["destructive_changes"] = destructive
		}
		ctx := s.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if err := RequestToolApproval(s, ctx, toolName, argsMap); err != nil {
			s.writeToolError(req.ID, err.Error())
			return
		}
	}

	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	output, exitCode, err := s.infraRunner.Run(ctx, cmd.Binary, cmd.Args)
	display := cmd.Binary + " " + strings.Join(cmd.Args, " ")

	truncated := false
	if len(output) > infraOutputLimit {
		output = output[:infraOutputLimit]
		truncated = true
	}

	text := fmt.Sprintf("$ %s\n(environment=%s risk=%s apply_class=%v exit=%d)\n\n%s",
		display, cmd.Environment, cmd.RiskProfile, cmd.ApplyClass, exitCode, output)
	if truncated {
		text += fmt.Sprintf("\n[output truncated at %d bytes]", infraOutputLimit)
	}
	if err != nil {
		text += fmt.Sprintf("\n[error: %v]", err)
	}

	result := map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": text},
		},
		"isError":     err != nil,
		"environment": cmd.Environment,
		"risk":        cmd.RiskProfile,
		"apply_class": cmd.ApplyClass,
		"exit_code":   exitCode,
	}
	if destructive != nil {
		result["destructive_changes"] = destructive
	}
	s.writeResult(req.ID, result)
}

func (s *Server) handleAnsibleRunPlaybook(req Request, params toolsCallParams) {
	if !s.infraEnabled(req) {
		return
	}
	var r AnsibleRunPlaybookRequest
	if err := json.Unmarshal(params.Arguments, &r); err != nil {
		s.writeToolError(req.ID, "invalid arguments: "+err.Error())
		return
	}
	cmd, err := s.infraRegistry.ResolveAnsiblePlaybook(r.Environment, r.Playbook, r.Limit, r.Check)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.runInfraCommand(req, params.Name, params.Arguments, cmd, nil)
}

func (s *Server) handleSaltApplyState(req Request, params toolsCallParams) {
	if !s.infraEnabled(req) {
		return
	}
	var r SaltApplyStateRequest
	if err := json.Unmarshal(params.Arguments, &r); err != nil {
		s.writeToolError(req.ID, "invalid arguments: "+err.Error())
		return
	}
	cmd, err := s.infraRegistry.ResolveSaltState(r.Environment, r.State, r.Target, r.Test)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.runInfraCommand(req, params.Name, params.Arguments, cmd, nil)
}

func (s *Server) handleSSHRunQuery(req Request, params toolsCallParams) {
	if !s.infraEnabled(req) {
		return
	}
	var r SSHRunQueryRequest
	if err := json.Unmarshal(params.Arguments, &r); err != nil {
		s.writeToolError(req.ID, "invalid arguments: "+err.Error())
		return
	}
	cmd, err := s.infraRegistry.ResolveSSHQuery(r.Environment, r.Host, r.Query)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.runInfraCommand(req, params.Name, params.Arguments, cmd, nil)
}

func (s *Server) handleTerraformPlan(req Request, params toolsCallParams) {
	if !s.infraEnabled(req) {
		return
	}
	var r TerraformPlanRequest
	if err := json.Unmarshal(params.Arguments, &r); err != nil {
		s.writeToolError(req.ID, "invalid arguments: "+err.Error())
		return
	}
	cmd, err := s.infraRegistry.ResolveTerraformPlan(r.Environment, r.Vars, r.Save)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.runInfraCommand(req, params.Name, params.Arguments, cmd, nil)
}

// handleTerraformApply implements the Phase-2 saved-plan pipeline:
//
//	[1] terraform show -json <saved plan>   (read, deterministic)
//	[2] Go gate: detect delete/replace      (never an LLM judgment)
//	[3] approval with findings attached     (fail-closed without a gate)
//	[4] terraform apply <saved plan>        (TOCTOU-safe: the plan binds
//	    the exact changes; terraform refuses it if state drifted)
func (s *Server) handleTerraformApply(req Request, params toolsCallParams) {
	if !s.infraEnabled(req) {
		return
	}
	var r TerraformApplyRequest
	if err := json.Unmarshal(params.Arguments, &r); err != nil {
		s.writeToolError(req.ID, "invalid arguments: "+err.Error())
		return
	}

	showCmd, err := s.infraRegistry.ResolveTerraformShowPlan(r.Environment)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	planJSON, _, err := s.infraRunner.Run(ctx, showCmd.Binary, showCmd.Args)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf(
			"cannot inspect the saved plan for %q (run terraform_plan with save=true first): %v\n%s",
			r.Environment, err, planJSON))
		return
	}
	// An unverifiable plan must never reach apply.
	destructive, err := s.infraRegistry.DestructiveChanges([]byte(planJSON))
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf(
			"refusing to apply: the saved plan for %q could not be verified deterministically: %v", r.Environment, err))
		return
	}

	applyCmd, err := s.infraRegistry.ResolveTerraformApply(r.Environment)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	if destructive == nil {
		destructive = []string{} // non-nil: result always reports the gate ran
	}
	s.runInfraCommand(req, params.Name, params.Arguments, applyCmd, destructive)
}
